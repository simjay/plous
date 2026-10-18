package source

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	argocode "github.com/simjay/plous/internal/integrations/argo/code"
	catalog "github.com/simjay/plous/internal/integrations/catalog"
	fluxcode "github.com/simjay/plous/internal/integrations/flux/code"
)

func (i *Inventory) discoverCatalog(ctx context.Context) {
	groups := i.kustomizeGroups()
	patches := map[string]bool{}
	for _, entry := range i.Entries {
		for _, ref := range entry.References {
			if strings.HasPrefix(ref.Relation, "patches") {
				patches[ref.Path] = true
			}
		}
	}
	add := func(c catalog.Item) {
		if i.discoveryErr != nil {
			return
		}
		if !i.keep(c) {
			return
		}
		c.Name = DisplayText(c.Name)
		c.Detail = DisplayText(c.Detail)
		c.Location = DisplayText(c.Location)
		c.Version = DisplayText(c.Version)
		i.Catalog = append(i.Catalog, c)
	}
	for _, e := range i.Entries {
		switch e.Kind {
		case "helm":
			c := i.capturedHelmChart(e)
			add(catalog.Item{ID: e.ID, Technology: "helm", Group: e.Path, Name: c.Name, Kind: "Helm chart", File: e.File, EntryID: e.ID, Version: c.Version, Detail: "Local chart"})
		case "kustomize", "kustomize+helm":
			roles := i.EntryRoles(e.ID)
			role := "Kustomization"
			if e.Component {
				role = "Component"
			} else if slices.Contains(roles, "overlay") && slices.Contains(roles, "base") {
				role = "Base + Overlay"
			} else if slices.Contains(roles, "overlay") {
				role = "Overlay"
			} else if slices.Contains(roles, "base") {
				role = "Base"
			}
			add(catalog.Item{ID: e.ID, Technology: "kustomize", Group: groups[e.ID], Name: e.Path, Kind: role, File: e.File, EntryID: e.ID})
			for n, c := range i.declaredHelmCharts(e.ID) {
				if c.EntryID != "" {
					continue
				}
				add(catalog.Item{ID: fmt.Sprintf("chart:%s:%d", e.ID, n), Technology: "helm", Group: e.Path, Name: c.Name, Kind: "Helm chart", File: e.File, EntryID: e.ID, Version: c.Version, Location: c.Repository, Detail: "Declared in Kustomize", External: true})
			}
		}
	}
	for _, file := range i.Source.Paths() {
		if ctx.Err() != nil || i.discoveryErr != nil {
			return
		}
		if ext := strings.ToLower(path.Ext(file)); ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		// Templates are interpreted only by an explicit Helm render.
		inChart := false
		for _, entry := range i.Entries {
			if entry.Kind == "helm" && underPath(file, entry.Path) {
				inChart = true
				break
			}
		}
		if inChart {
			continue
		}
		docs := catalog.Documents(map[string][]byte{file: i.discoveryFile(file)})
		for _, items := range [][]catalog.Item{argocode.Discover(docs), fluxcode.Discover(docs)} {
			for _, item := range items {
				add(item)
			}
		}
	}
	slices.SortStableFunc(i.Catalog, func(a, b catalog.Item) int {
		return strings.Compare(a.Technology+"\x00"+a.Group+"\x00"+a.File+"\x00"+a.ID, b.Technology+"\x00"+b.Group+"\x00"+b.File+"\x00"+b.ID)
	})
	i.DirectoryTags = map[string]string{}
	tags := map[string]map[string]bool{}
	letters := map[string]string{"helm": "H", "kustomize": "K", "argo": "A", "flux": "F"}
	for _, c := range i.Catalog {
		for dir := path.Dir(c.File); ; dir = path.Dir(dir) {
			if tags[dir] == nil {
				tags[dir] = map[string]bool{}
			}
			tags[dir][letters[c.Technology]] = true
			if dir == "." {
				break
			}
		}
	}
	for dir, set := range tags {
		var letters []string
		for _, letter := range []string{"H", "K", "A", "F"} {
			if set[letter] {
				letters = append(letters, letter)
			}
		}
		tags := strings.Join(letters, " ")
		if !i.keep([]string{dir, tags}) {
			return
		}
		i.DirectoryTags[dir] = tags
	}
	i.Catalog = slices.DeleteFunc(i.Catalog, func(c catalog.Item) bool { return patches[c.File] })
}

// Compute connected groups once so shared bases do not repeat the repository traversal.
func (i *Inventory) kustomizeGroups() map[string]string {
	neighbors := map[string][]string{}
	paths := map[string]string{}
	for _, e := range i.Entries {
		if !strings.HasPrefix(e.ID, "kustomize:") {
			continue
		}
		paths[e.ID] = e.Path
		for _, id := range i.EntryDependencies(e.ID) {
			if strings.HasPrefix(id, "kustomize:") {
				neighbors[e.ID] = append(neighbors[e.ID], id)
				neighbors[id] = append(neighbors[id], e.ID)
			}
		}
	}
	groups := map[string]string{}
	for root, common := range paths {
		if _, done := groups[root]; done {
			continue
		}
		members := []string{root}
		groups[root] = common
		for n := 0; n < len(members); n++ {
			id := members[n]
			for common != "." && !underPath(paths[id], common) {
				common = path.Dir(common)
			}
			for _, next := range neighbors[id] {
				if _, done := groups[next]; !done {
					groups[next] = common
					members = append(members, next)
				}
			}
		}
		for _, id := range members {
			groups[id] = common
		}
	}
	return groups
}
func underPath(file, dir string) bool {
	return dir == "." || dir == "" || file == dir || strings.HasPrefix(file, dir+"/")
}

// RenderTargets prefers explicit definitions and direct file ownership. Helm consumers
// are included only when the Kustomization actually references that chart.
func (i *Inventory) RenderTargets(file string, directory bool) []Entry {
	var result []Entry
	add := func(e Entry) {
		if e.Renderable() && !slices.ContainsFunc(result, func(v Entry) bool { return v.ID == e.ID }) {
			result = append(result, e)
		}
	}
	for _, e := range i.Entries {
		if e.Kind == "plain" {
			continue
		}
		if e.Path == file && directory || e.File == file && !directory {
			add(e)
		}
	}
	if len(result) == 0 && !directory {
		for _, e := range i.Entries {
			if e.Kind == "plain" {
				continue
			}
			for _, ref := range e.References {
				if ref.Path == file && ref.Error == "" {
					add(e)
				}
			}
		}
		nearest := ""
		for _, e := range i.Entries {
			if e.Kind != "plain" && underPath(file, e.Path) && (nearest == "" || len(e.Path) > len(nearest)) {
				nearest = e.Path
			}
		}
		if nearest != "" {
			for _, e := range i.Entries {
				if e.Kind != "plain" && e.Path == nearest {
					add(e)
				}
			}
		}
	}
	if len(result) == 0 && directory {
		for _, e := range i.Entries {
			if e.Kind != "plain" && underPath(e.Path, file) {
				add(e)
			}
		}
	}
	selected := slices.Clone(result)
	for _, e := range selected {
		if e.Kind == "helm" {
			for _, id := range i.EntryConsumers(e.ID) {
				if parent := i.Entry(id); parent != nil {
					add(*parent)
				}
			}
		}
	}
	if len(result) == 0 && !directory {
		if e := i.Entry("plain:" + file); e != nil {
			add(*e)
		}
	}
	return result
}
