package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"
	catalog "github.com/simjay/plous/internal/integrations/catalog"
	"github.com/simjay/plous/internal/source"
)

func TestCodeTechnologyBrowsersUseSharedInventory(t *testing.T) {
	m, s := codeModel(t)
	files := map[string]string{
		"gitops/flux.yaml": "apiVersion: helm.toolkit.fluxcd.io/v2\nkind: HelmRelease\nmetadata: {name: api}\nspec:\n  chart:\n    spec: {chart: api, version: 1.2.3, sourceRef: {kind: HelmRepository, name: charts}}\n",
		"gitops/argo.yaml": "apiVersion: argoproj.io/v1alpha1\nkind: ApplicationSet\nmetadata: {name: apps}\nspec: {}\n",
		".fluxschema.yml":  "apiVersion: schema.plugin.fluxcd.io/v1beta1\nkind: Config\n",
	}
	for file, data := range files {
		captureSourceFixture(t, s.Inventory.Source)
		s.Inventory.Source.Files[file] = []byte(data)
	}
	s.Inventory = source.Discover(s.Inventory.Source)
	if tags := s.Inventory.DirectoryTags["gitops"]; tags != "H A F" {
		t.Fatal(tags)
	}
	inv := s.Inventory
	for _, id := range []string{"helm", "kustomize", "argo", "flux"} {
		openSourceView(t, m, id)
		if m.tool != appcode.BrowserTool(id) || m.exploreActive() || m.helmActive() {
			t.Fatalf("browser leaked to Monitor: %s", m.tool)
		}
		if s.Inventory != inv || len(s.CatalogItems()) == 0 || s.Pending != "" {
			t.Fatalf("browser changed source or ran build: %s", id)
		}
		for _, size := range [][2]int{{38, 16}, {80, 24}, {145, 36}} {
			view := m.codeBody(size[0], size[1])
			lines := strings.Split(view, "\n")
			if len(lines) > size[1] {
				t.Fatalf("height %s %v", id, size)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatal("card overflow", id, size)
				}
			}
			if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
				os.MkdirAll(dir, 0700)
				os.WriteFile(filepath.Join(dir, fmt.Sprintf("browser-%s-%d.ansi", id, size[0])), []byte(view), 0600)
			}
		}
		if id == "kustomize" {
			if !s.KustomizeDirectory() {
				t.Fatal("Kustomize skipped groups")
			}
			m.updateCode("enter")
		}
		m.updateCode("enter")
		if !s.BrowserDetail || !strings.Contains(ansi.Strip(m.codeBody(100, 28)), "File:") {
			t.Fatal("card did not open details", id)
		}
		if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
			os.WriteFile(filepath.Join(dir, "details-"+id+".ansi"), []byte(m.codeBody(100, 40)), 0600)
		}
		m.updateCode("esc")
		if s.BrowserDetail {
			t.Fatal("Back did not restore cards")
		}
		item, _ := s.SelectedCatalogItem()
		m.executeCode("describe")
		if m.tool != "files" || s.Preference.File != item.File {
			t.Fatal("Open in Files lost source", id)
		}
		if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
			os.WriteFile(filepath.Join(dir, "files-"+id+".ansi"), []byte(m.codeBody(120, 38)), 0600)
		}
	}
}
func TestCodeRenderShortcutsChooseCompositionAndReturn(t *testing.T) {
	m, s := codeModel(t)
	openSourceView(t, m, "helm")
	finishCode(t, m)
	s.Cursor = slices.IndexFunc(s.CatalogItems(), func(c catalog.Item) bool { return c.EntryID == "helm:composed/charts/demo" })
	s.RememberSelection()
	originalCursor := s.Cursor
	typeText(m, "r")
	if m.shortcutPrefix != "" {
		t.Fatal("single r became a pending shortcut")
	}
	if s.Menu != "render-target" || len(s.RenderTargets) != 2 {
		t.Fatalf("missing chart/composition choice: %s %#v tool=%s view=%s cursor=%d focus=%d notice=%s error=%s", s.Menu, s.RenderTargets, m.tool, s.View, s.Cursor, m.focus, s.Notice, s.Err+" status="+m.status+" prefix="+m.shortcutPrefix)
	}
	for _, want := range []string{"Helm chart only", "Kustomize including chart"} {
		if !strings.Contains(strings.Join(s.MenuItems(), "\n"), want) {
			t.Fatal("ambiguous choice label", want)
		}
	}
	s.Choice = slices.IndexFunc(s.RenderTargets, func(e source.Entry) bool { return e.ID == "kustomize:composed" })
	m.updateCode("enter")
	if m.tool != "manifests" || s.ReturnView != "code-helm" || s.RenderRoot != "kustomize:composed" || s.Pending != "render" {
		t.Fatal("wrong render origin/target")
	}
	finishCode(t, m)
	if s.Builds["kustomize:composed"] == nil {
		t.Fatal("render failed", s.Err)
	}
	m.codeBrowserControl("graph")
	if s.Preference.Section != 0 {
		t.Fatal("Graph button did not open graph")
	}
	for _, entry := range s.Entries() {
		if entry.ID != "kustomize:composed" && entry.ID != "helm:composed/charts/demo" {
			t.Fatal("unrelated graph target", entry)
		}
	}
	m.codeBrowserControl("output")
	if s.Preference.Section != 1 || s.Pending != "" {
		t.Fatal("Output button rebuilt or did not open result")
	}
	m.updateCode("esc")
	if m.tool != "code-helm" || s.Cursor != originalCursor {
		t.Fatalf("Back lost card selection: %s %s %d", m.tool, s.Preference.File, s.Cursor)
	}
	openSourceView(t, m, "helm")
	s.Cursor = slices.IndexFunc(s.CatalogItems(), func(c catalog.Item) bool { return c.EntryID == "helm:chart" })
	s.RememberSelection()
	originalCursor = s.Cursor
	m.execute("r")
	finishCode(t, m)
	if s.ReturnView != "code-helm" || s.Preference.Selected != "helm:chart" {
		t.Fatal("colon render lost chart")
	}
	m.codeBrowserControl("back")
	if m.tool != "code-helm" || s.Cursor != originalCursor {
		t.Fatal("Back did not restore chart browser")
	}
}
func TestCodeRootAndBrowserSelectionNeverExecute(t *testing.T) {
	m, s := codeModel(t)
	s.FileRoot = "composed"
	s.Cursor = 0
	m.codeBrowserControl("root")
	if s.FileRoot != "" || s.FileRows()[0].Path != "." || s.Pending != "" {
		t.Fatal("root control failed")
	}
	m.executeCode("render")
	if s.Menu != "" || s.Pending != "" || s.Notice == "" {
		t.Fatal("Files should direct rendering to a technology view")
	}
	m.updateCode("esc")
	selectCodeFile(t, s, "README.md")
	m.executeCode("render")
	if s.Pending != "" || s.Notice == "" {
		t.Fatal("non-configuration file rendered")
	}
}

func TestCodeExternalChartDoesNotRenderItsContainingKustomization(t *testing.T) {
	m, s := codeModel(t)
	captureSourceFixture(t, s.Inventory.Source)
	s.Inventory.Source.Files["composed/release.yaml"] = []byte("apiVersion: helm.toolkit.fluxcd.io/v2\nkind: HelmRelease\nmetadata: {name: remote}\nspec:\n  chart:\n    spec: {chart: remote, version: 1.2.3}\n")
	s.Inventory = source.Discover(s.Inventory.Source)
	openSourceView(t, m, "helm")
	s.Cursor = slices.IndexFunc(s.CatalogItems(), func(c catalog.Item) bool { return c.Name == "remote" })
	if s.Cursor < 0 {
		t.Fatal("missing external chart")
	}
	if len(s.RenderCandidates()) != 0 {
		t.Fatal("external chart inherited an unrelated render target")
	}
	for _, action := range s.CatalogActions()[0].Actions {
		if action.Key == ":r" {
			t.Fatal("external chart advertises unavailable rendering")
		}
	}
	m.executeCode("render")
	if s.Pending != "" || s.Menu != "" || s.Err == "" {
		t.Fatal("unsupported render did not stay in the browser")
	}
}
