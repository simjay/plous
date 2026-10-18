package monitor

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
)

type RevisionChanges struct {
	Fields         []manifestutil.FieldChange
	FieldsComplete bool
	Before, After  string
	Diff           string
	Changes        []RevisionChange
	Added, Removed int
	Unavailable    string
}

type RevisionChange struct {
	Group, Label, Action string
}

// CompareRevisions compares recorded inputs and rendered objects, not live state.
func CompareRevisions(before, after *Snapshot) (*RevisionChanges, error) {
	if after == nil || after.Version < 1 {
		return nil, fmt.Errorf("a recorded revision is required")
	}
	if before == nil && after.Version != 1 {
		return nil, fmt.Errorf("baseline revision %d unavailable", after.Version-1)
	}
	if before != nil && (before.Name != after.Name || before.Namespace != after.Namespace || before.Version < 1 || before.Version == after.Version) {
		return nil, fmt.Errorf("the baseline must be a distinct retained revision of this release")
	}
	recorded := func(s *Snapshot) (map[string]any, error) {
		if s == nil {
			return nil, nil
		}
		objects := map[string]any{}
		resources, err := manifestutil.Resources(s.Manifest)
		if err != nil {
			return nil, fmt.Errorf("recorded resources unavailable: %w", err)
		}
		for _, r := range resources {
			objects[r.ScopedKey(s.Namespace)] = r.Object
		}
		hooks := map[string]any{}
		for _, hook := range s.Hooks {
			rs, err := manifestutil.Resources(hook.Manifest)
			if err != nil {
				return nil, fmt.Errorf("recorded hooks unavailable: %w", err)
			}
			for _, r := range rs {
				hooks[hook.Name+" / "+r.ScopedKey(s.Namespace)] = map[string]any{"events": hook.Events, "resource": r.Object}
			}
		}
		return map[string]any{"chart": s.Chart.Metadata, "suppliedValues": s.Config, "chartDefaults": s.Chart.Values, "resources": objects, "hooks": hooks}, nil
	}
	left, err := recorded(before)
	if err != nil {
		return nil, err
	}
	right, err := recorded(after)
	if err != nil {
		return nil, err
	}
	result := &RevisionChanges{}
	result.Fields, result.FieldsComplete = manifestutil.FieldChanges(left, right)
	appendChange := func(group, label string, before, after any) {
		if reflect.DeepEqual(before, after) {
			return
		}
		action := "changed"
		if before == nil {
			action = "added"
		}
		if after == nil {
			action = "removed"
		}
		result.Changes = append(result.Changes, RevisionChange{Group: group, Label: label, Action: action})
	}
	for _, group := range []string{"resources", "hooks"} {
		a, _ := left[group].(map[string]any)
		b, _ := right[group].(map[string]any)
		keys := map[string]bool{}
		for key := range a {
			keys[key] = true
		}
		for key := range b {
			keys[key] = true
		}
		var ordered []string
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		for _, key := range ordered {
			label := key
			parts := strings.Split(key, " / ")
			if len(parts) >= 4 {
				resource := parts[len(parts)-3] + " / "
				if namespace := parts[len(parts)-2]; namespace != "" {
					resource += namespace + "/"
				}
				resource += parts[len(parts)-1]
				label = resource
				if group == "hooks" {
					label = "Hook " + parts[0] + " · " + resource
				}
			}
			appendChange(group, label, a[key], b[key])
		}
	}
	for _, group := range []string{"suppliedValues", "chartDefaults", "chart"} {
		if before == nil {
			if values, _ := right[group].(map[string]any); len(values) == 0 {
				continue
			}
		}
		appendChange(group, map[string]string{"suppliedValues": "Supplied values", "chartDefaults": "Chart defaults", "chart": "Chart metadata"}[group], left[group], right[group])
	}
	if before != nil && len(result.Changes) == 0 {
		return result, nil
	}
	a, b := manifestutil.MaskedComparison(left, right)
	if before == nil {
		a = ""
	}
	// Object summaries remain useful when a full comparison exceeds the line matcher budget.
	if len(a)+len(b) > 2<<20 || strings.Count(a, "\n")+strings.Count(b, "\n") > 20000 {
		result.Unavailable = "Full diff is too large. Inspect each revision to read its configuration."
		return result, nil
	}
	result.Before, result.After = a, b
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: difflib.SplitLines(a), B: difflib.SplitLines(b), Context: 2})
	if err != nil {
		return nil, err
	}
	result.Diff = diff
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") {
			result.Added++
		}
		if strings.HasPrefix(line, "-") {
			result.Removed++
		}
	}
	return result, nil
}
