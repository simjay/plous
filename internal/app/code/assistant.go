package code

import (
	"fmt"
	"slices"
	"strings"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/source"
)

func (s *CodeState) AssistantSelection() source.ReadSelection {
	selection := source.ReadSelection{Path: s.Preference.File, Target: s.Preference.Selected}
	if s.Inventory != nil {
		selection.SourceIdentity, selection.Revision = s.Inventory.Source.Identity, s.Inventory.Source.Revision
	}
	if s.Document != nil {
		selection.Path, selection.Unsaved = s.DocumentPath, s.Document.Dirty()
	}
	if s.View == "files" || s.View == "code" {
		if s.Document == nil {
			if row, ok := s.SelectedFileRow(); ok {
				selection.Path = row.Path
			}
		}
	} else if IsBrowser(s.View) {
		if item, ok := s.SelectedCatalogItem(); ok {
			selection.Path, selection.Target = item.File, item.EntryID
		}
	}
	if s.View == "manifests" {
		selection.Section = CodeSections[max(0, min(s.Preference.Section, len(CodeSections)-1))]
		if b := s.Builds[s.Preference.Selected]; b != nil {
			selection.Result = b.OperationID
		}
		if s.Preference.Section == 1 || s.Preference.Section == 2 {
			if r, ok := s.CurrentResource(); ok {
				selection.Resource = r.Key()
			}
		}
		if s.Preference.Section == 2 {
			if field, ok := s.CurrentField(); ok {
				selection.Field = field.Pointer
			}
		}
		if s.ComparisonActive() {
			selection.Result, selection.Other = s.ComparisonBase.OperationID, s.Comparison.OperationID
			if s.ComparisonSwapped {
				selection.Result, selection.Other = selection.Other, selection.Result
			}
		}
		if stage := s.assistantStage(); stage != nil {
			selection.Result = stage.OperationID
		}
	}
	return selection
}

func (s *CodeState) AssistantReadView() *source.ReadView {
	failures := map[string]string{}
	if s.Inventory != nil {
		for _, e := range s.Inventory.Entries {
			if failure := s.PreviewFailure(e.ID); failure != "" {
				failures[e.ID] = failure
			}
		}
	}
	return source.CaptureReadView(s.Inventory, s.Builds, s.Inputs, failures, s.Comparison, s.ComparisonBase, s.SavedBaseline, s.assistantStage())
}

// A captured intermediate stage is an independent result without final-output
// field attribution, matching the stage inspector's existing behavior.
func (s *CodeState) assistantStage() *source.Result {
	stage := s.CapturedStage()
	if stage == nil {
		return nil
	}
	original := s.Builds[s.Preference.Selected]
	result := *original
	build := *original.BuildResult
	result.BuildResult = &build
	build.Resources = nil
	for _, r := range stage.After {
		build.Resources = append(build.Resources, r)
	}
	slices.SortFunc(build.Resources, func(a, b manifest.Resource) int { return strings.Compare(a.Key(), b.Key()) })
	build.Observation = nil
	result.Origins, result.Lineages = nil, nil
	result.OperationID = fmt.Sprintf("%s/stage/%d", original.OperationID, s.Stage)
	return &result
}
