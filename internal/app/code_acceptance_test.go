package app

import (
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/source"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func codeAction(t *testing.T, m *model, label string) {
	t.Helper()
	s := m.coder()
	s.Menu = "actions"
	s.Choice = slices.Index(s.MenuItems(), label)
	if s.Choice < 0 {
		for _, group := range []string{"More Tools", "Comparison Options"} {
			s.Menu = "actions"
			s.Choice = slices.Index(s.MenuItems(), group)
			if s.Choice < 0 {
				continue
			}
			m.activateCodeMenu()
			s.Choice = slices.Index(s.MenuItems(), label)
			if s.Choice >= 0 {
				break
			}
		}
	}
	if s.Choice < 0 {
		t.Fatal("missing action", label, s.MenuItems())
	}
	m.activateCodeMenu()
}

func TestCodeComparisonTraceValidateAcceptance(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	snapshot, err := source.Load(context.Background(), source.Config{Path: "../../tests/fixtures/code/inspector"})
	if err != nil {
		t.Fatal(err)
	}
	captureSourceFixture(t, snapshot)
	snapshot.Files["envs/prod/kustomization.yaml"] = append(snapshot.Files["envs/prod/kustomization.yaml"], []byte("\nsecretGenerator:\n- name: credentials\n  literals: [password=acceptance-private-secret]\n")...)
	s.Inventory = source.Discover(snapshot)
	for _, id := range []string{"kustomize:envs/dev", "kustomize:envs/prod"} {
		r, err := (source.Renderer{}).Render(context.Background(), s.Inventory, id, source.Inputs{})
		if err != nil {
			t.Fatal(err)
		}
		s.Builds[id] = r
		s.MarkPreviewAttempt(id)
	}
	s.Preference.Selected = "kustomize:envs/prod"
	s.ChooseSection(1)
	s.Comparison, s.ComparisonBase = s.Builds["kustomize:envs/dev"], s.Builds[s.Preference.Selected]
	s.ComparisonScope = s.Preference.Selected
	s.Cursor = slices.IndexFunc(s.Resources(), func(r manifest.Resource) bool { return r.Kind == "Deployment" && r.Name == "prod-api" })
	s.RememberSelection()
	view := ansi.Strip(m.codeBody(145, 35))
	if !strings.Contains(view, "Verified source identity") || !strings.Contains(view, "replicas:") {
		t.Fatal("comparison not aligned", view)
	}
	codeAction(t, m, "Changed Only")
	if strings.Contains(ansi.Strip(m.codeBody(145, 35)), "apiVersion:") {
		t.Fatal("unchanged lines survived Changed Only")
	}
	codeAction(t, m, "Swap Comparison")
	if !strings.Contains(ansi.Strip(m.codeBody(145, 35)), "Compare envs/prod → envs/dev") {
		t.Fatal("swap did not reverse labels")
	}
	if s.Preference.Selected != "kustomize:envs/prod" {
		t.Fatal("swap changed source scope")
	}
	codeAction(t, m, "All Lines")
	codeAction(t, m, "Clear Comparison")
	s.InspectorArea = 2
	s.SelectField(slices.IndexFunc(s.CurrentFields(), func(f source.Field) bool { return f.Pointer == "/spec/replicas" }))
	m.updateCode("enter")
	if s.Preference.Section != 2 || !strings.Contains(ansi.Strip(m.codeBody(145, 35)), "4 → 6") {
		t.Fatal("comparison to Trace lost field")
	}
	m.openCodeFieldSource()
	if s.Document == nil || s.Document.Cursor == 0 || s.DocumentPath != "envs/prod/kustomization.yaml" {
		t.Fatal("writer source did not position cursor")
	}
	if strings.Contains(s.Document.Text(), "acceptance-private-secret") {
		t.Fatal("source jump leaked generator input")
	}
	m.codeDocumentCommand("q")
	s.Builds[s.Preference.Selected].Observation.Truncated = true
	s.ChooseSection(1)
	s.Menu = "stages"
	if !strings.Contains(ansi.Strip(m.codeBody(70, 30)), "Partial capture") {
		t.Fatal("stage capture limit invisible")
	}
	s.Menu = ""
	s.ChooseSection(3)
	codeAction(t, m, "Images")
	if len(s.Findings()) == 0 || s.Findings()[0].Rule != "image" {
		t.Fatal("Images view has no current image rows")
	}
	m.codeFindingDestination("trace")
	if s.Preference.Section != 2 || !strings.Contains(s.Preference.Field, "image") {
		t.Fatal("image link lost field")
	}
	s.ChooseSection(3)
	codeAction(t, m, "Generators")
	if len(s.Findings()) != 1 || !strings.Contains(s.Findings()[0].Message, "password") || strings.Contains(s.Findings()[0].Message, "acceptance-private-secret") {
		t.Fatal("generator inventory missing or unsafe", s.Findings())
	}
	for _, size := range [][2]int{{80, 24}, {120, 32}, {180, 42}, {240, 50}} {
		m.width, m.height = size[0], size[1]
		for _, label := range []string{"Findings", "Images", "Generators"} {
			s.SelectValidationView(label)
			frame := m.View().Content
			lines := strings.Split(frame, "\n")
			if len(lines) != m.height {
				t.Fatal("frame height", label, size, len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != m.width {
					t.Fatal("frame width", label, size, ansi.StringWidth(line))
				}
			}
			if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("validate-%s-%d.ansi", strings.ToLower(label), m.width)), []byte(frame), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	s.SelectValidationView("Findings")
	s.ValidationCategory = "Patches"
	for _, f := range s.Findings() {
		if source.FindingCategory(f) != "Patches" {
			t.Fatal("category filter leaked", f)
		}
	}
}

func TestCodeRemovedComparisonResourceScrolls(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected, s.Pending = "plain:plain/service.yaml", "render"
	finishCode(t, m)
	before := *s.Builds[s.Preference.Selected]
	build := *before.BuildResult
	before.BuildResult = &build
	before.OperationID = "removed-fixture"
	object := map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "removed"}, "spec": map[string]any{}}
	for n := 0; n < 60; n++ {
		object["spec"].(map[string]any)[fmt.Sprintf("key%02d", n)] = fmt.Sprint(n)
	}
	before.Resources = append(slices.Clone(before.Resources), manifest.Resource{APIVersion: "v1", Kind: "ConfigMap", Name: "removed", Object: object})
	s.Comparison, s.ComparisonBase, s.ComparisonScope = &before, s.Builds[s.Preference.Selected], s.Preference.Selected
	s.Cursor = 0
	s.InspectorArea = 2
	if r, _ := s.CurrentResource(); r.Name != "removed" {
		t.Fatal("removed resource not selectable")
	}
	_ = m.codeBody(90, 22)
	m.updateCode("end")
	view := ansi.Strip(m.codeBody(90, 22))
	if !strings.Contains(view, "key59:") {
		t.Fatal("removed-only rows cannot scroll", view)
	}
	m.updateCode("up")
	if s.ComparisonOffset >= s.ComparisonRowCount()-1 {
		t.Fatal("End left scroll beyond bounded rows")
	}
	m.updateCode("enter")
	if s.Preference.Section != 1 {
		t.Fatal("absent resource incorrectly opened Trace for current scope")
	}
}
