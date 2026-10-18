package code

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/simjay/plous/internal/source"
	"strings"
	"time"
)

func (s *CodeState) Start() tea.Cmd {
	kind, other := s.Pending, s.Other
	if kind == "compare" || kind == "compare-revision" {
		s.SelectFinalResource()
		s.RememberSelection()
	}
	config := s.Config
	config.Revision = s.Preference.Revision
	if kind == "revision" {
		config.Revision, kind = other, "load"
		if config.Revision == "" {
			config.Revision = "HEAD"
		}
	}
	inv, selected, renderer := s.Inventory, s.Preference.Selected, s.Renderer
	inputs := map[string]source.Inputs{}
	for id, in := range s.Inputs {
		inputs[id] = in
	}
	s.Run++
	run := s.Run
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	s.Cancel, s.Busy, s.Running, s.Pending, s.Err = cancel, true, kind, "", ""
	s.RunningEntry = selected
	s.RunningView = s.ContextID()
	if inv != nil && kind != "load" {
		s.MarkPreviewAttempt(selected)
	}
	s.LoadAttempted = true
	if kind == "load" {
		s.Notice = ""
		s.Rescan, s.ScanFailed, s.ScanCancelled = false, false, false
	}
	return func() tea.Msg {
		defer func() {
			if kind != "load" {
				cancel()
			}
		}()
		result := CodeResult{State: s, Run: run, Kind: kind, Inputs: inputs, Inventory: inv, Results: map[string]*source.Result{}, Failures: map[string]string{}}
		if kind == "load" || inv == nil {
			var snapshot *source.Snapshot
			snapshot, result.Err = source.Load(ctx, config)
			if result.Err != nil {
				cancel()
				if kind == "render" && inv != nil {
					result.Failures[selected] = result.Err.Error()
				}
				return result
			}
			if kind == "load" {
				result.Kind = "paths"
				result.Inventory = source.Browse(snapshot)
				result.Continue = func() tea.Msg {
					defer cancel()
					final := result
					final.Kind = "load"
					final.Continue = nil
					final.Inventory, final.Err = source.DiscoverContext(ctx, snapshot)
					return final
				}
				return result
			}
			inv, result.Err = source.DiscoverContext(ctx, snapshot)
			result.Inventory = inv
			if result.Err != nil {
				return result
			}
		}
		switch kind {
		case "load":
			return result
		case "preview", "render", "compare", "compare-revision":
			result.Before, result.Err = renderer.Render(ctx, inv, selected, inputs[selected])
			if result.Err != nil {
				result.Failures[selected] = result.Err.Error()
				return result
			}
			result.Results[selected] = result.Before
			if kind == "compare" {
				result.After, result.Err = renderer.Render(ctx, inv, other, inputs[other])
				if result.After != nil {
					result.Results[other] = result.After
				}
			}
			if kind == "compare-revision" {
				config.Revision = other
				var snapshot *source.Snapshot
				snapshot, result.Err = source.Load(ctx, config)
				if result.Err == nil {
					var otherInventory *source.Inventory
					otherInventory, result.Err = source.DiscoverContext(ctx, snapshot)
					if result.Err == nil {
						result.After, result.Err = renderer.Render(ctx, otherInventory, selected, inputs[selected])
					}
				}
			}
		case "check", "trace-all":
			for _, e := range inv.Entries {
				if kind == "trace-all" && !strings.HasPrefix(e.ID, "kustomize:") {
					continue
				}
				if ctx.Err() != nil {
					result.Err = ctx.Err()
					break
				}
				if e.Component {
					continue
				}
				build, err := renderer.Render(ctx, inv, e.ID, inputs[e.ID])
				if err != nil {
					result.Failures[e.ID] = err.Error()
				} else {
					result.Results[e.ID] = build
				}
			}
		}
		return result
	}
}

func (s *CodeState) Apply(r CodeResult) bool {
	if s == nil || s.Run != r.Run {
		return false
	}
	if r.Kind == "paths" {
		if s.Inventory == nil {
			s.Inventory = r.Inventory
		}
		return true
	}
	s.Busy, s.Cancel = false, nil
	if r.Kind == "load" {
		s.ScanFailed, s.ScanCancelled = r.Err != nil, false
		if r.Err == nil && r.Inventory != nil {
			s.Preference.Revision = r.Inventory.Source.Config.Revision
		}
	}
	if r.Inventory != nil && r.Kind != "preview" && r.Kind != "trace-all" {
		s.Inventory = r.Inventory
		if entries := s.Inventory.SourceEntries(); s.Inventory.Entry(s.Preference.Selected) == nil {
			s.Preference.Selected = ""
			for _, e := range entries {
				if s.Preference.Selected == "" || e.Renderable() {
					s.Preference.Selected = e.ID
					if e.Renderable() {
						break
					}
				}
			}
		}
		s.RestoreCursor()
	}
	if s.Inventory != nil && s.FailureSource != s.Inventory.Source.Identity {
		s.Failures = map[string]string{}
		s.FailureSource = s.Inventory.Source.Identity
	}
	// A preview uses a captured snapshot. A refresh or file save may replace it
	// while the preview is running, so obsolete output cannot publish into it.
	if (r.Kind == "preview" || r.Kind == "trace-all") && r.Inventory != s.Inventory {
		return false
	}
	for id, b := range r.Results {
		if id == s.Preference.Selected {
			s.Stage = 0
			s.FieldResource = ""
		}
		s.Builds[id] = b
		delete(s.Failures, id)
		s.RecordPreviewAttempt(id, b.SourceIdentity+"/"+b.InputIdentity)
	}
	if s.SavedComparison && s.Comparison != nil {
		if current := r.Results[s.ComparisonScope]; current != nil {
			s.ComparisonBase = current
		}
	}
	for id, f := range r.Failures {
		s.Failures[id] = f
		s.RecordPreviewAttempt(id, r.Inventory.Source.Identity+"/"+r.Inputs[id].Identity())
	}
	if r.Kind == "preview" && s.View == "manifests" && (s.Preference.Section == 1 || s.Preference.Section == 2) && r.Results[s.Preference.Selected] != nil {
		s.RestoreCursor()
		s.RestoreTraceReference()
	}
	if r.Err != nil && r.Kind != "preview" && r.Kind != "render" {
		s.Err = r.Err.Error()
	} else {
		s.Err = ""
	}
	if r.Kind == "render" && r.Before != nil {
		if s.ContextID() == s.RunningView && s.Preference.Selected == r.Before.ID && s.Document == nil {
			s.ChooseSection(1)
		}
		s.RestoreCursor()
		s.Notice = "Rendered " + CodeCountLabel(len(r.Before.Resources), "resource", "resources") + "."
	}
	if r.After != nil {
		s.ComparisonBase, s.Comparison = r.Before, r.After
		s.ComparisonScope = r.Before.ID
		s.ComparisonSwapped, s.ComparisonChanged, s.ComparisonHidden, s.SavedComparison = false, false, false, false
		s.ComparisonOffset, s.ComparisonResource = 0, ""
		if s.ContextID() == s.RunningView && s.Preference.Selected == r.Before.ID {
			s.ChooseSection(1)
		}
		s.RestoreCursor()
	}
	if previous := s.SavedBaseline; previous != nil && (r.Kind == "preview" || r.Kind == "render") {
		if current := r.Results[previous.ID]; current != nil {
			s.Comparison, s.ComparisonBase, s.ComparisonScope = previous, current, previous.ID
			s.ComparisonSwapped, s.ComparisonHidden = false, false
			s.ComparisonChanged, s.SavedComparison = true, true
			s.ComparisonOffset, s.ComparisonResource = 0, ""
			s.SavedBaseline = nil
			s.Notice = "Saved changes rendered."
			s.RestoreCursor()
		}
	}

	if r.Kind == "trace-all" {
		s.Notice = fmt.Sprintf("Traced %d Kustomize sources; %d failed. Source values use the captured snapshot.", len(r.Results)+len(r.Failures), len(r.Failures))
		if s.View == "manifests" && s.Preference.Section == 2 {
			s.RestoreTraceReference()
		}
	}
	if r.Kind == "check" {
		if s.ContextID() == s.RunningView {
			s.OpenFindings(true)
		}
		s.Notice = fmt.Sprintf("Validated %d entry points; %d failed.", len(r.Results)+len(r.Failures), len(r.Failures))
	}
	return true
}
