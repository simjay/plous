package code

import (
	manifest "github.com/simjay/plous/internal/integrations/manifest"

	"github.com/simjay/plous/internal/ui"
	"slices"
	"strings"
)

func (s *CodeState) SyncListScope() {
	s.SyncDocumentView(s.View)
	scope := s.View
	if s.View == "code-kustomize" {
		scope += "/" + s.Group
	}
	if s.View == "manifests" {
		scope = []string{"entries", "resources/" + s.Preference.Selected, "resources/" + s.Preference.Selected, "checks"}[s.Preference.Section]
	}
	if scope == s.ListScope {
		return
	}
	if s.Filters == nil {
		s.Filters, s.Cursors = map[string]string{}, map[string]int{}
	}
	if s.ListScope != "" {
		s.Filters[s.ListScope], s.Cursors[s.ListScope] = s.Filter, s.Cursor
	}
	s.ListScope = scope
	s.Filter, s.Cursor = s.Filters[scope], s.Cursors[scope]
	s.RestoreCursor()
}

func (s *CodeState) Resources() []manifest.Resource {
	build := s.Builds[s.Preference.Selected]
	if build == nil {
		return nil
	}
	if s.ComparisonActive() {
		var result []manifest.Resource
		for _, change := range s.ComparisonChanges() {
			result = append(result, change.Resource())
		}
		return result
	}
	var resources []manifest.Resource
	output := build.Resources
	if stage := s.CapturedStage(); stage != nil {
		output = nil
		for _, r := range stage.After {
			output = append(output, r)
		}
	}
	for _, resource := range output {
		if ui.SearchContains(resource.Kind+" "+resource.Namespace+" "+resource.Name, s.Filter) {
			resources = append(resources, resource)
		}
	}
	slices.SortFunc(resources, func(a, b manifest.Resource) int {
		return strings.Compare(a.Kind+"/"+a.Namespace+"/"+a.Name, b.Kind+"/"+b.Namespace+"/"+b.Name)
	})
	return resources
}

func (s *CodeState) TerraformDirectories() []string {
	if s.Inventory == nil {
		return nil
	}
	var directories []string
	for _, directory := range s.Inventory.Terraform {
		if ui.SearchContains(directory, s.Filter) {
			directories = append(directories, directory)
		}
	}
	return directories
}

func (s *CodeState) RememberSelection() {
	if s.Preference.Section == 2 && s.TraceUnavailable != "" {
		return
	}
	if s.KustomizeDirectory() {
		groups := s.KustomizeGroupItems()
		if s.Cursor >= 0 && s.Cursor < len(groups) {
			s.Preference.KustomizeGroup = groups[s.Cursor].Path
		}
		s.Preference.Browser = s.View
	} else if IsBrowser(s.View) {
		if item, ok := s.SelectedCatalogItem(); ok {
			s.Preference.BrowserItem = item.ID
		}
		s.Preference.Browser = s.View
	} else if s.View == "files" || s.View == "code" {
		if row, ok := s.SelectedFileRow(); ok {
			s.Preference.File = row.Path
		}
	} else if s.View == "manifests" && s.Preference.Section == 0 {
		if entries := s.Entries(); s.Cursor >= 0 && s.Cursor < len(entries) {
			s.Preference.Selected = entries[s.Cursor].ID
		}
	} else if s.View == "manifests" && (s.Preference.Section == 1 || s.Preference.Section == 2) {
		if resources := s.Resources(); s.Cursor >= 0 && s.Cursor < len(resources) {
			s.Preference.Resource = resources[s.Cursor].Key()
		}
	}
}
