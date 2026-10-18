package source

import (
	"slices"
	"strings"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
)

type ResourceChange struct {
	Before, After manifest.Resource
	Status, Match string
}

func (c ResourceChange) Resource() manifest.Resource {
	if c.After.Name != "" {
		return c.After
	}
	return c.Before
}

// A file alone is not a resource identity: it can contain multiple documents or
// be included repeatedly. Match only unique, verified source document identities.
func Compare(before, after *Result) []ResourceChange {
	old, next := before.Resources, after.Resources
	used := map[int]bool{}
	exact := map[string]int{}
	reserved := map[int]string{}
	left, right := map[string][]int{}, map[string][]int{}
	for n, r := range old {
		exact[r.Key()] = n
		if key := before.Lineages[r.Key()]; key != "" {
			left[key] = append(left[key], n)
		}
	}
	for n, r := range next {
		if key := after.Lineages[r.Key()]; key != "" {
			right[key] = append(right[key], n)
		}
	}
	for key, indices := range left {
		if len(indices) == 1 && len(right[key]) == 1 {
			reserved[indices[0]] = key
		}
	}
	var changes []ResourceChange
	for _, r := range next {
		index, match := -1, "No verified match"
		lineage := after.Lineages[r.Key()]
		if lineage != "" && len(left[lineage]) == 1 && len(right[lineage]) == 1 {
			index, match = left[lineage][0], "Verified source identity"
		} else if lineage != "" && (len(left[lineage]) > 1 || len(right[lineage]) > 1) {
			match = "Ambiguous source identity"
		}
		if index < 0 && match != "Ambiguous source identity" {
			if n, exists := exact[r.Key()]; exists && !used[n] {
				other := before.Lineages[old[n].Key()]
				if (lineage == "" || other == "" || lineage == other) && (reserved[n] == "" || reserved[n] == lineage) {
					index, match = n, "Exact resource identity"
				}
			}
		}
		change := ResourceChange{After: r, Status: "Added", Match: match}
		if index >= 0 && !used[index] {
			used[index] = true
			change.Before, change.Status = old[index], "Changed"
			a, b := manifest.MaskedComparison(old[index].Object, r.Object)
			if a == b {
				change.Status = "Unchanged"
			}
		}
		changes = append(changes, change)
	}
	for n, r := range old {
		if !used[n] {
			match := "No verified match"
			key := before.Lineages[r.Key()]
			if key != "" && (len(left[key]) > 1 || len(right[key]) > 1) {
				match = "Ambiguous source identity"
			}
			changes = append(changes, ResourceChange{Before: r, Status: "Removed", Match: match})
		}
	}
	slices.SortStableFunc(changes, func(a, b ResourceChange) int { return strings.Compare(a.Resource().Key(), b.Resource().Key()) })
	return changes
}

func (r *Result) identifyLineages(i *Inventory) {
	r.Lineages = map[string]string{}
	initial := map[int]manifest.Resource{}
	if r.Observation != nil {
		for _, stage := range r.Observation.Stages {
			for _, snapshot := range []map[int]manifest.Resource{stage.Before, stage.After} {
				for id, resource := range snapshot {
					if _, exists := initial[id]; !exists {
						initial[id] = resource
					}
				}
			}
		}
	}
	files := map[string][]manifest.Resource{}
	for _, resource := range r.Resources {
		file := r.Origins[resource.Key()]
		if file == "" || strings.HasPrefix(r.ID, "helm:") {
			continue
		}
		candidate := resource
		if r.Observation != nil {
			if original, ok := initial[r.Observation.FinalIDs[resource.Key()]]; ok {
				candidate = original
			}
		}
		docs, loaded := files[file]
		if !loaded {
			docs, _ = manifest.Resources(string(i.Source.Files[file]))
			files[file] = docs
		}
		matches := 0
		for _, doc := range docs {
			if doc.Key() == candidate.Key() {
				matches++
			}
		}
		if matches == 1 {
			r.Lineages[resource.Key()] = file + "\x00" + candidate.Key()
		}
	}
}
