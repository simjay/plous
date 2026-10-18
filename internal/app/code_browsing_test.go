package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/source"

	docedit "github.com/simjay/plous/internal/ui/editor"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func selectCodeFile(t *testing.T, s *appcode.CodeState, name string) {
	t.Helper()
	s.FileRows()
	s.RevealFile(name)
	s.Preference.File = name
	s.RestoreCursor()
	if row, ok := s.SelectedFileRow(); !ok || row.Path != name {
		t.Fatal("file tree did not select", name)
	}
}

func TestCodeFilesTreeOpensEditorAndPreservesFolders(t *testing.T) {
	m, s := codeModel(t)
	rows := s.FileRows()
	seenFile := false
	for _, row := range rows {
		if row.Path == "." {
			continue
		}
		if strings.Contains(row.Path, "/") || row.Open {
			t.Fatal("initial tree must show closed root folders", rows)
		}
		if !row.Directory {
			seenFile = true
		} else if seenFile {
			t.Fatal("folders must precede files")
		}
	}
	s.Cursor = 1
	m.updateCode("right")
	if !s.FileRows()[1].Open {
		t.Fatal("Right did not expand folder")
	}
	m.updateCode("right")
	m.updateCode("left")
	if s.Cursor != 1 || !s.FileRows()[1].Open {
		t.Fatal("Left on a child did not return to its parent")
	}
	m.updateCode("left")
	if s.FileRows()[1].Open {
		t.Fatal("Left did not collapse folder")
	}
	selectCodeFile(t, s, "plain/service.yaml")
	m.updateCode("enter")
	if s.Document == nil || s.Document.ReadOnly || s.Document.Mode != docedit.Normal || s.File.Path != "plain/service.yaml" {
		t.Fatal("Enter must directly open the local editor")
	}
	original := string(s.File.Baseline)
	if s.Document.Text() != original {
		t.Fatal("editor changed the original bytes")
	}
	m.codeDocumentCommand("q")
	row, _ := s.SelectedFileRow()
	if row.Path != "plain/service.yaml" || !s.FileExpanded["plain"] {
		t.Fatal("closing the editor lost the tree position")
	}
	s.Pending = "load"
	finishCode(t, m)
	if !s.FileExpanded["plain"] {
		t.Fatal("refresh discarded open folders")
	}
	s.FileExpanded = nil
	s.RestoreCursor()
	row, _ = s.SelectedFileRow()
	if row.Path != "plain/service.yaml" || !s.FileExpanded["plain"] {
		t.Fatal("restored selection is hidden inside a closed folder")
	}
}

func TestCodeFileSearchRevealsAncestorsWithoutChangingFolds(t *testing.T) {
	m, s := codeModel(t)
	m.updateCode("/")
	s.Input.Text = "service.yaml"
	m.submitCodeInput()
	row, _ := s.SelectedFileRow()
	if row.Directory || row.Path != "plain/service.yaml" || s.FileExpanded["plain"] {
		t.Fatal("search did not select its file or changed manual folds", row)
	}
	m.updateCode("enter")
	if s.Document == nil || s.Document.ReadOnly {
		t.Fatal("filtered Enter did not open the editor")
	}
	m.codeDocumentCommand("q")
	m.updateCode("backspace")
	if slices.ContainsFunc(s.FileRows(), func(row appcode.CodeFileRow) bool { return row.Path == "plain/service.yaml" }) {
		t.Fatal("clearing search retained automatic folder expansion")
	}
	for _, width := range []int{32, 80, 120} {
		body := ansi.Strip(m.codeBody(width, 24))
		if strings.Contains(body, "Kubernetes:") || strings.Contains(body, "Terraform:") {
			t.Fatal("Files contains unrelated capability metadata", body)
		}
	}
}

func TestCodeFilesRemoteEnterUsesReadOnlyEditor(t *testing.T) {
	m, s := codeModel(t)
	selectCodeFile(t, s, "plain/service.yaml")
	// Keep the same captured inventory and simulate a remote source adapter.
	s.Config.Repository = "https://example.invalid/platform.git"
	m.updateCode("enter")
	if s.Document == nil || !s.Document.ReadOnly || s.File != nil || s.Document.Name != "plain/service.yaml" {
		t.Fatal("remote Enter did not open the read-only editor")
	}
}

func TestCodeFileTreeMouseOpensFoldersAndEditor(t *testing.T) {
	m, s := codeModel(t)
	m.width, m.height = 180, 42
	now := time.Unix(1000, 0)
	m.mouse.now = func() time.Time { return now }
	clickMouse(t, m, "main", "code-row/1")
	if s.FileRows()[1].Open {
		t.Fatal("single click opened folder")
	}
	now = now.Add(100 * time.Millisecond)
	clickMouse(t, m, "main", "code-row/1")
	if !s.FileRows()[1].Open {
		t.Fatal("double click did not expand folder")
	}
	selectCodeFile(t, s, "plain/service.yaml")
	target := fmt.Sprintf("code-row/%d", s.Cursor)
	now = now.Add(time.Second)
	clickMouse(t, m, "main", target)
	now = now.Add(100 * time.Millisecond)
	clickMouse(t, m, "main", target)
	if s.Document == nil || s.Document.ReadOnly || s.File.Path != "plain/service.yaml" {
		t.Fatal("double click did not open the selected local file for editing")
	}
}

func TestCodeSourceClicksKeepSelectionStableWithoutRendering(t *testing.T) {
	m, s := codeModel(t)
	m.width, m.height = 180, 60
	m.selectScope(m.active, "manifests", m.connection)
	s.OpenDetail(0)
	now := time.Unix(1000, 0)
	m.mouse.now = func() time.Time { return now }
	index := slices.IndexFunc(s.Entries(), func(e source.Entry) bool { return e.ID == "plain:plain/service.yaml" })
	target := fmt.Sprintf("code-row/%d", index)
	before := mousePoint(t, m, "main", target)
	clickMouse(t, m, "main", target)
	if m.syncCode() != nil || len(s.Builds) != 0 {
		t.Fatal("selecting a configuration started a render")
	}
	after := mousePoint(t, m, "main", target)
	if before.Y != after.Y {
		t.Fatal("selecting a configuration moved its node under the pointer")
	}
	now = now.Add(100 * time.Millisecond)
	clickMouse(t, m, "main", target)
	if s.Preference.Section != 0 || s.Preference.Selected != "plain:plain/service.yaml" || m.syncCode() != nil {
		t.Fatal("double clicking a configuration left the map or started a render")
	}
}

func TestCodeOutputRestoresPreviewForCapturedSelection(t *testing.T) {
	m, s := codeModel(t)
	if m.syncCode() != nil || len(s.Builds) != 0 {
		t.Fatal("Files must not render")
	}
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected = "plain:plain/service.yaml"
	s.ChooseSection(1)
	s.RestoreCursor()
	captured := s.Inventory
	cmd := m.syncCode()
	if cmd == nil || s.Running != "preview" || m.syncCode() != nil {
		t.Fatal("restoring missing output must schedule one automatic preview")
	}
	if view := ansi.Strip(m.codeBody(120, 30)); !strings.Contains(view, "OUTPUT") || !strings.Contains(view, "Entry: plain/service.yaml") || !strings.Contains(view, "Rendering") {
		t.Fatal("Render hides selection or progress", view)
	}
	m.updateCode("enter")
	if s.Preference.Section != 1 || s.Menu != "" {
		t.Fatal("Enter must open resources while preview is running")
	}
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if s.Builds[s.Preference.Selected] == nil || s.Inventory != captured || m.syncCode() != nil {
		t.Fatal("preview did not reuse captured source or repeated itself")
	}
	m.updateCode("y")
	if s.Document == nil || !s.Document.ReadOnly || !strings.Contains(s.Document.Text(), "kind: Service") {
		t.Fatal("rendered resources did not open in the shared editor")
	}
}

func TestCodePreviewFailureCancellationAndInputChanges(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected = "kustomize:prod"
	s.ChooseSection(1)
	s.RestoreCursor()
	calls := 0
	s.Renderer.Kustomize = func(ctx context.Context, dir string) ([]byte, string, error) {
		calls++
		if strings.HasPrefix(dir, s.Config.Path+string(filepath.Separator)) {
			t.Fatal("preview ran in the original source")
		}
		return nil, "fixture", errors.New("fixture render failed")
	}
	cmd := m.syncCode()
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if s.Failures[s.Preference.Selected] == "" || calls != 1 || m.syncCode() != nil {
		t.Fatal("failed preview must wait for explicit retry")
	}
	s.ChooseSection(1)
	if view := ansi.Strip(m.codeBody(80, 24)); !strings.Contains(view, "Render failed") || !strings.Contains(view, "r Retry") {
		t.Fatal("failure view does not explain recovery", view)
	}
	m.executeCode("render")
	cmd = m.syncCode()
	m.updateCode("esc")
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if m.syncCode() != nil || !strings.Contains(s.Failures[s.Preference.Selected], "cancelled") {
		t.Fatal("cancelled render restarted or published a late result")
	}
	s.Inputs[s.Preference.Selected] = source.Inputs{Namespace: "new-input"}
	if m.syncCode() == nil {
		t.Fatal("new inputs must allow a new preview")
	}
	s.Stop()
}

func TestCodePreviewRestoresResourceSelection(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	id := "plain:plain/service.yaml"
	s.Preference.Selected = id
	text := "apiVersion: v1\nkind: Service\nmetadata: {name: first}\n---\napiVersion: v1\nkind: Service\nmetadata: {name: second}\n"
	s.Inventory = source.Discover(changedSourceFixture(t, s.Inventory.Source, "plain/service.yaml", []byte(text)))
	s.ChooseSection(1)
	cmd := m.syncCode()
	result := cmd().(appcode.CodeResult)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	for _, resource := range result.Results[id].Resources {
		if resource.Name == "second" {
			s.Preference.Resource = resource.Key()
		}
	}
	applyCodeResult(t, m, result)
	if s.Resources()[s.Cursor].Name != "second" {
		t.Fatal("automatic preview lost the saved resource selection")
	}
}

func TestCodePreviewLateResultsDoNotMoveSelectionOrReplaceSnapshot(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected = "plain:plain/service.yaml"
	s.ChooseSection(1)
	s.RestoreCursor()
	cmd := m.syncCode()
	m.updateCode("home")
	selected := s.Preference.Selected
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if s.Preference.Selected != selected || s.Preference.Section != 1 || s.Builds["plain:plain/service.yaml"] == nil {
		t.Fatal("background result changed navigation or lost captured entry")
	}
	s.Preference.Selected = "plain:plain/service.yaml"
	s.Inputs[s.Preference.Selected] = source.Inputs{Namespace: "changed"}
	cmd = m.syncCode()
	oldBuild := s.Builds[s.Preference.Selected]
	s.Inventory = source.Discover(changedSourceFixture(t, s.Inventory.Source, "README.md", []byte("saved during preview")))
	saved := s.Inventory
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if s.Inventory != saved || s.Builds[s.Preference.Selected] != oldBuild {
		t.Fatal("old preview replaced a saved source or published obsolete output")
	}
}
