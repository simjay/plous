package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/source"
	textview "github.com/simjay/plous/internal/ui/text"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func codeInspectorFixture(t *testing.T) (*model, *appcode.CodeState) {
	t.Helper()
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	snapshot, err := source.Load(context.Background(), source.Config{Path: "../../tests/fixtures/code/inspector"})
	if err != nil {
		t.Fatal(err)
	}
	s.Inventory = source.Discover(snapshot)
	id := "kustomize:envs/prod"
	r, err := (source.Renderer{}).Render(t.Context(), s.Inventory, id, source.Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	s.Builds, s.Preference.Selected = map[string]*source.Result{id: r}, id
	s.ChooseSection(2)
	s.Preference.Resource = r.Resources[slices.IndexFunc(r.Resources, func(r manifest.Resource) bool { return r.Kind == "Deployment" && r.Name == "prod-api" })].Key()
	s.Preference.Field = "/spec/replicas"
	s.RestoreCursor()
	return m, s
}

func TestCodeTraceAcrossOverlaysKeepsSnapshotAndField(t *testing.T) {
	m, s := codeInspectorFixture(t)
	inv := s.Inventory
	codeAction(t, m, "Trace Across All Overlays")
	finishCode(t, m)
	if s.Inventory != inv || (s.ViewID() != "files" || s.Preference.Section != 2) || s.Preference.Selected != "kustomize:envs/prod" {
		t.Fatal("trace changed scope or reloaded a different repository")
	}
	if s.Builds["kustomize:apps/api/base"] == nil || s.PreviewFailure("kustomize:clusters/prod-ap-south") == "" {
		t.Fatal("missing successful trace or partial failure")
	}
	selectSource := func(id string) {
		t.Helper()
		m.selectCodeSource(slices.IndexFunc(s.NavigationEntries(), func(e source.Entry) bool { return e.ID == id }))
	}
	selectSource("kustomize:envs/dev")
	r, ok := s.CurrentResource()
	f, field := s.CurrentField()
	if !ok || !field || r.Name != "api" || f.Pointer != "/spec/replicas" || f.Value != "1" {
		t.Fatalf("lost renamed resource or field: %+v %+v unavailable=%s", r, f, s.TraceUnavailable)
	}
	selectSource("kustomize:apps/worker/base")
	if _, ok := s.CurrentResource(); ok || !strings.Contains(s.TraceUnavailable, "no unambiguous match") {
		t.Fatal("unmatched source showed another resource")
	}
	selectSource("kustomize:clusters/prod-ap-south")
	if _, ok := s.CurrentResource(); ok || !strings.Contains(s.TraceUnavailable, "failed") {
		t.Fatal("failed source showed field output")
	}
	selectSource("kustomize:clusters/prod-us-east")
	f, ok = s.CurrentField()
	if !ok || f.Pointer != "/spec/replicas" || f.Value != "6" {
		t.Fatal("could not resume the same trace after an unmatched or failed source", f)
	}
	if body := ansi.Strip(m.codeBody(120, 35)); !strings.Contains(body, "OBSERVED CHANGES") {
		t.Fatal(body)
	}
}

func TestCodeTraceAllLateResultCannotReplaceNewSnapshot(t *testing.T) {
	m, s := codeInspectorFixture(t)
	codeAction(t, m, "Trace Across All Overlays")
	cmd := m.syncCode()
	result := cmd().(appcode.CodeResult)
	old := s.Inventory
	next := *old
	s.Inventory = &next
	applyCodeResult(t, m, result)
	if s.Inventory != &next || s.Builds["kustomize:envs/dev"] != nil {
		t.Fatal("obsolete trace published into refreshed source")
	}
}

func TestCodeExportCapturesFinalOutputAndRetries(t *testing.T) {
	m, s := codeInspectorFixture(t)
	s.ChooseSection(1)
	s.Filter = "prod-api"
	s.Stage = 1
	codeAction(t, m, "Export Rendered YAML")
	captured := s.ExportResult
	if captured == nil || !strings.Contains(s.InputHelp(), "Sensitive values are masked") {
		t.Fatal("missing captured export contract")
	}
	m.width, m.height = 80, 24
	s.Input.Text = "result.yaml"
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "result.yaml") || !strings.Contains(view, "masked") {
		t.Fatal("compact export hides the destination or masking contract", view)
	}
	file := filepath.Join(t.TempDir(), "render.yaml")
	if err := os.WriteFile(file, []byte("keep original"), 0600); err != nil {
		t.Fatal(err)
	}
	e := textview.NewField(file)
	s.Input = &e
	m.submitCodeInput()
	if s.Input == nil || s.Err == "" || s.ExportResult != captured {
		t.Fatal("failed export lost retry state")
	}
	data, _ := os.ReadFile(file)
	if string(data) != "keep original" {
		t.Fatal("export replaced existing file")
	}
	file = filepath.Join(t.TempDir(), "new.yaml")
	e = textview.NewField(file)
	s.Input = &e
	m.submitCodeInput()
	data, err := os.ReadFile(file)
	resources, parseErr := manifest.Resources(string(data))
	if err != nil || parseErr != nil || len(resources) != len(captured.Resources) || s.Input != nil || s.ExportResult != nil {
		t.Fatal("export did not write full captured final output", err, parseErr)
	}
	codeAction(t, m, "Export Rendered YAML")
	m.codeKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if s.Input != nil || s.ExportResult != nil {
		t.Fatal("cancel retained export state")
	}
}

func TestCodeValidationSourceFilterComposesWithOtherFilters(t *testing.T) {
	m, s := codeInspectorFixture(t)
	s.ChooseSection(3)
	s.Inventory.Findings = append(s.Inventory.Findings,
		source.Finding{Target: "kustomize:envs/prod", Rule: "overwritten-field", Message: "replicas overwritten"},
		source.Finding{Target: "kustomize:envs/dev", Rule: "overwritten-field", Message: "replicas overwritten"})
	codeAction(t, m, "Filter Source")
	s.Choice = slices.Index(s.MenuItems(), "envs/prod")
	m.activateCodeMenu()
	s.Filter, s.ValidationCategory = "replicas overwritten", "Patches"
	if got := s.Findings(); len(got) != 1 || got[0].Target != "kustomize:envs/prod" {
		t.Fatal("source/category/text filters did not intersect", got)
	}
	s.SelectValidationView("Images")
	if got := s.Findings(); len(got) == 0 {
		t.Fatal("selected source images disappeared")
	} else {
		for _, f := range got {
			if f.Target != s.ValidationSource {
				t.Fatal("image escaped source filter")
			}
		}
	}
	s.SelectValidationView("Findings")
	if len(s.Findings()) != 1 || s.Filter != "replicas overwritten" {
		t.Fatal("view switch lost filter")
	}
	codeAction(t, m, "Filter Source")
	s.Choice = 0
	m.activateCodeMenu()
	if len(s.Findings()) != 2 {
		t.Fatal("clearing source filter damaged category or text filter")
	}
}

func TestCodeComparisonKeepsImplicitlySelectedResource(t *testing.T) {
	m, _ := codeInspectorFixture(t)
	// Dispatch uses the active view after the UI synchronizes the Code state.
	s := m.coder()
	s.ChooseSection(1)
	r, _ := s.CurrentResource()
	s.Preference.Resource = ""
	s.Pending, s.Other = "compare", "kustomize:envs/dev"
	m.startCode(s)
	s.Cancel()
	before := s.Builds[s.Preference.Selected]
	after := *before
	build := *before.BuildResult
	after.BuildResult = &build
	extra, err := manifest.Resources("apiVersion: v1\nkind: ConfigMap\nmetadata: {name: comparison-only}\n")
	if err != nil {
		t.Fatal(err)
	}
	build.Resources = append(slices.Clone(build.Resources), extra...)
	applyCodeResult(t, m, appcode.CodeResult{State: s, Run: s.Run, Kind: "compare", Before: before, After: &after})
	current, ok := s.CurrentResource()
	if !ok || current.Key() != r.Key() {
		t.Fatalf("comparison jumped from %s to %s", r.Key(), current.Key())
	}
}
