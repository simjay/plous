package app

import (
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/source"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCodeSourcesUsesNodesAndAnInspector(t *testing.T) {
	m, s := codeModel(t)
	snapshot, err := source.Load(context.Background(), source.Config{Path: "../../tests/fixtures/code/transformers"})
	if err != nil {
		t.Fatal(err)
	}
	m.selectScope(m.active, "manifests", m.connection)
	s.Inventory = source.Discover(snapshot)
	s.Preference.Selected = "kustomize:overlays/custom-metadata-labels"
	s.ChooseSection(0)
	s.RestoreCursor()
	graph := ansi.Strip(s.SourceGraph(95, 30))
	if strings.Count(graph, "╭") != 2 || !strings.Contains(graph, "◀") || strings.Contains(graph, "metadataLabelTransformer.yaml") {
		t.Fatal("graph must contain two linked kustomizations, with input files in the inspector", graph)
	}
	view := ansi.Strip(m.codeBody(145, 45))
	for _, want := range []string{"Reference graph", "INCLUDES", "THIS NODE ADDS", "transformers: 1", "USED BY", "FINDINGS"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in source inspector: %s", want, view)
		}
	}
	if view := ansi.Strip(m.codeBody(90, 30)); !strings.Contains(view, "Reference graph") || !strings.Contains(view, "base") {
		t.Fatal("tree did not preserve the dependency edge", view)
	}
	m.updateCode("right")
	m.updateCode("pgdown")
	if s.InspectorArea != 1 || s.DetailOffset == 0 {
		t.Fatal("keyboard cannot reach the source inspector")
	}
}

func TestCodeRenderFieldOpensTraceWithCapturedSource(t *testing.T) {
	m, s := codeModel(t)
	m.width, m.height = 240, 50
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected, s.Pending = "plain:plain/service.yaml", "render"
	finishCode(t, m)
	resource, _ := s.CurrentResource()
	fields := s.CurrentFields()
	index := slices.IndexFunc(fields, func(f source.Field) bool { return f.Pointer == "/metadata/name" })
	if index < 0 {
		t.Fatal("name field unavailable")
	}
	view := ansi.Strip(m.codeBody(140, 38))
	for _, want := range []string{"Entry: plain/service.yaml", "RESOURCES", "apiVersion: v1", "Service / api"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in inline Render", want)
		}
	}
	clickMouse(t, m, "main", fmt.Sprintf("code-field/%d", index))
	if s.Preference.Section != 2 || s.Preference.Field != "/metadata/name" || s.Preference.Resource != resource.Key() || s.Document != nil {
		t.Fatal("field click lost scope or opened the old document dump")
	}
	trace := ansi.Strip(m.codeBody(150, 38))
	for _, want := range []string{"FIELD HISTORY", "Final value: api", "Exact source field", "plain/service.yaml:4"} {
		if !strings.Contains(trace, want) {
			t.Fatalf("missing trace evidence %q: %s", want, trace)
		}
	}
	m.updateCode("enter")
	if s.Document == nil || s.DocumentPath != "plain/service.yaml" || !strings.Contains(s.Document.Notice, ":4") {
		t.Fatal("Trace did not open the captured source with its original location")
	}
}

func TestCodeValidationFailureLinksAndScrolling(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	id := "kustomize:prod"
	s.MarkPreviewAttempt(id)
	s.Failures[id] = "Cannot render prod: unmatched patch target. " + strings.Repeat("diagnostic ", 60) + "last diagnostic"
	s.ChooseSection(3)
	s.Filter = "unmatched"
	s.Cursor = 0
	view := ansi.Strip(m.codeBody(120, 28))
	for _, want := range []string{"SEVERITY", "FINDING", "WHERE", "1 failed", "unmatched"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Validate missing %q: %s", want, view)
		}
	}
	m.updateCode("right")
	for range 10 {
		m.updateCode("pgdown")
	}
	if view := ansi.Strip(m.codeBody(90, 28)); !strings.Contains(view, "last diagnostic") {
		t.Fatal("long finding details cannot be scrolled", view)
	}
	m.codeFindingDestination("sources")
	if s.Preference.Section != 0 || s.Preference.Selected != id {
		t.Fatal("finding did not retain its source when navigating")
	}
}

func TestCodeNativeTransformerViews(t *testing.T) {
	m, s := codeModel(t)
	snapshot, err := source.Load(context.Background(), source.Config{Path: "../../tests/fixtures/code/transformers"})
	if err != nil {
		t.Fatal(err)
	}
	m.selectScope(m.active, "manifests", m.connection)
	s.Inventory = source.Discover(snapshot)
	s.Preference.Selected = "kustomize:overlays/custom-metadata-labels"
	for _, entry := range s.Inventory.Entries {
		result, err := (source.Renderer{}).Render(context.Background(), s.Inventory, entry.ID, source.Inputs{})
		if err != nil {
			t.Fatal(err)
		}
		s.Builds[entry.ID] = result
		s.MarkPreviewAttempt(entry.ID)
	}
	m.expanded[m.active], m.expanded[m.connection] = true, true
	for _, size := range [][2]int{{80, 24}, {120, 32}, {180, 42}, {240, 50}} {
		m.width, m.height = size[0], size[1]
		for section, scene := range appcode.CodeSections {
			s.ChooseSection(section)
			s.RestoreCursor()
			if section == 2 {
				index := slices.IndexFunc(s.CurrentFields(), func(f source.Field) bool { return f.Pointer == "/metadata/labels/owner" })
				if !s.SelectField(index) {
					t.Fatal("rendered transformer field is unavailable")
				}
			}
			view := m.View().Content
			if strings.Contains(ansi.Strip(view), "not supported by Plous builds") {
				t.Fatal("valid builtin transformer still fails in the UI")
			}
			if lines := strings.Split(view, "\n"); len(lines) != m.height {
				t.Fatalf("%s height %d, want %d", scene, len(lines), m.height)
			} else {
				for _, line := range lines {
					if ansi.StringWidth(line) != m.width {
						t.Fatalf("%s width %d, want %d", scene, ansi.StringWidth(line), m.width)
					}
				}
			}
			if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				for ext, text := range map[string]string{"txt": ansi.Strip(view), "ansi": view} {
					name := fmt.Sprintf("native-%s-%d.%s", strings.ToLower(scene), m.width, ext)
					if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}

func TestCodeCompactRenderRecoveryAndPreciseFinding(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected, s.Pending = "plain:plain/service.yaml", "render"
	finishCode(t, m)
	view := ansi.Strip(m.codeBody(50, 26))
	if !strings.Contains(view, "apiVersion: v1") || !strings.Contains(view, "[YAML]") {
		t.Fatal("compact Render must start on YAML", view)
	}
	s.ChooseSection(3)
	b := s.Builds[s.Preference.Selected]
	resource := b.Resources[0]
	b.Findings = []source.Finding{{Severity: "warning", Rule: "fixture", Target: b.ID, Resource: resource.Key(), Field: "/metadata/name", Summary: "Selected field needs review", Message: "Fixture diagnostic"}}
	s.Filter = "Fixture diagnostic"
	s.Cursor = 0
	m.codeFindingDestination("trace")
	field, ok := s.CurrentField()
	if !ok || field.Pointer != "/metadata/name" || s.Preference.Resource != resource.Key() || s.Filter != "" || s.Preference.Section != 2 {
		t.Fatal("finding lost exact destination", s.Preference, field)
	}
	s.ChooseSection(1)
	s.Preference.Selected = "kustomize:prod"
	s.MarkPreviewAttempt(s.Preference.Selected)
	s.Failures[s.Preference.Selected] = "unmatched target"
	view = ansi.Strip(m.codeBody(90, 26))
	if !strings.Contains(view, "Graph") || !strings.Contains(view, "Couldn't render prod") || !strings.Contains(view, "Technical details") {
		t.Fatal("failure removed source navigation", view)
	}
	m.activateCodeDiagnostics()
	if s.Document == nil || !strings.Contains(s.Document.Text(), "unmatched target") {
		t.Fatal("technical details lost original failure")
	}
	s.CloseDocument()
	s.Preference.Selected = b.ID
	s.SyncListScope()
	s.Filter = "matches-nothing"
	view = ansi.Strip(m.codeBody(90, 26))
	if !strings.Contains(view, "Entry:") || !strings.Contains(view, "Backspace Clear filter") {
		t.Fatal("empty filter removed source navigation", view)
	}
}

func TestCodeRealisticInspectorStagesAndFrames(t *testing.T) {
	m, s := codeModel(t)
	snapshot, err := source.Load(context.Background(), source.Config{Path: "../../tests/fixtures/code/inspector"})
	if err != nil {
		t.Fatal(err)
	}
	m.selectScope(m.active, "manifests", m.connection)
	s.Inventory = source.Discover(snapshot)
	for _, entry := range s.Inventory.Entries {
		if entry.Component {
			continue
		}
		b, err := (source.Renderer{}).Render(context.Background(), s.Inventory, entry.ID, source.Inputs{})
		s.MarkPreviewAttempt(entry.ID)
		if err != nil {
			s.Failures[entry.ID] = err.Error()
		} else {
			s.Builds[entry.ID] = b
		}
	}
	s.Preference.Selected = "kustomize:clusters/prod-us-east"
	s.ChooseSection(1)
	resources := s.Resources()
	s.Cursor = slices.IndexFunc(resources, func(r manifest.Resource) bool { return r.Kind == "Deployment" && r.Name == "prod-api" })
	s.RememberSelection()
	s.ChooseSection(2)
	s.CurrentFields()
	s.SelectField(slices.IndexFunc(s.CurrentFields(), func(f source.Field) bool { return f.Pointer == "/spec/replicas" }))
	view := ansi.Strip(m.codeBody(145, 40))
	for _, want := range []string{"OBSERVED CHANGES", "1 → 4", "4 → 6", "replicas-patch.yaml"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Trace missing %q: %s", want, view)
		}
	}
	m.openCodeFieldSource()
	if s.DocumentPath != "envs/prod/kustomization.yaml" {
		t.Fatal("Trace opened base instead of last writer", s.DocumentPath)
	}
	m.codeDocumentCommand("q")
	s.ChooseSection(1)
	if len(s.StageItems()) < 3 {
		t.Fatal("no captured stages")
	}
	s.Menu, s.Choice = "stages", 1
	m.activateCodeMenu()
	if s.CapturedStage() == nil || !strings.Contains(ansi.Strip(m.codeBody(80, 25)), "Stage 1") {
		t.Fatal("stage picker did not select observed output")
	}
	s.Menu, s.Choice = "stages", 0
	m.activateCodeMenu()
	if s.Stage != 0 {
		t.Fatal("Final output did not restore")
	}
	m.expanded[m.active], m.expanded[m.connection] = true, true
	for _, size := range [][2]int{{80, 24}, {120, 32}, {180, 42}, {240, 50}} {
		m.width, m.height = size[0], size[1]
		for section, scene := range appcode.CodeSections {
			s.ChooseSection(section)
			s.RestoreCursor()
			frame := m.View().Content
			lines := strings.Split(frame, "\n")
			if len(lines) != m.height {
				t.Fatalf("%s exceeds terminal height", scene)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != m.width {
					t.Fatalf("%s exceeds terminal width", scene)
				}
			}
			if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("inspector-%s-%d.txt", strings.ToLower(scene), m.width)), []byte(ansi.Strip(frame)), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
