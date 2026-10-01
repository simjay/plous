package app

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	docedit "github.com/simjay/plous/internal/editor"
	"github.com/simjay/plous/internal/kubernetes"
	"github.com/simjay/plous/internal/textview"
	"k8s.io/apimachinery/pkg/types"
	"unicode/utf8"
)

func yamlViewModel(t *testing.T) *model {
	t.Helper()
	m := exploreModel(t)
	m.openExploreSelection(1)
	m.explore.detailValid = true
	return m
}

func yamlKey(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

func TestYAMLDocumentReadOnlyAndCopyExcludePresentation(t *testing.T) {
	m := yamlViewModel(t)
	d := m.syncYAMLDocument()
	original := d.text
	for _, key := range []string{"i", "a", "o", "d", "x", "p", "u", "E", "V", ":", "\t"} {
		m.yamlDocumentKey(yamlKey([]rune(key)[0], key))
	}
	m.yamlDocumentPaste("\nmetadata:\n  name: changed\n")
	if d.text != original || kubernetes.YAML(*m.explore.detail, false) != original {
		t.Fatal("reading keys or paste changed the Kubernetes YAML")
	}
	d.document.Cursor = yamlOffset(d, 1, 1)
	m.yamlDocumentKey(yamlKey('z', "z"))
	d.document.Cursor = yamlOffset(d, 3, 3)
	copied := d.document.SelectedText()
	lines := strings.Split(strings.TrimSuffix(original, "\n"), "\n")
	want := string([]rune(lines[1])[1:]) + "\n" + lines[2] + "\n" + string([]rune(lines[3])[:4])
	if copied != want || strings.Contains(copied, "\x1b") || strings.Contains(copied, "│") {
		t.Fatalf("copy contains presentation or changed source text: %q, want %q", copied, want)
	}
	handled, command := m.yamlDocumentKey(yamlKey('y', "y"))
	if !handled || command == nil || d.document.Mode == docedit.Select || d.text != original {
		t.Fatal("explicit YAML copy did not use the shared clipboard path")
	}
	for _, key := range []string{"E", "V", ":", "[", "]", "a", "b", "i", "s", "c", "h", "1", "2", "3"} {
		if handled, _ := m.yamlDocumentKey(yamlKey([]rune(key)[0], key)); handled {
			t.Fatalf("YAML reader claimed host action %q", key)
		}
	}
}

func TestYAMLDocumentSecretVisibilityClearsSensitiveReadingState(t *testing.T) {
	m := yamlViewModel(t)
	e := m.explore
	e.resource = kubernetes.Resource{Name: "secrets", Kind: "Secret", Version: "v1", Namespaced: true}
	e.detail.SetKind("Secret")
	e.detail.Object["data"] = map[string]any{"password": "cHJpdmF0ZS12YWx1ZQ=="}
	e.detail.Object["stringData"] = map[string]any{"token": "private-token"}
	e.detail.SetAnnotations(map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "private-old-value"})
	original, _ := json.Marshal(e.detail.Object)
	view := ansi.Strip(m.yamlDocumentBody(100, 40))
	for _, value := range []string{"cHJpdmF0ZS12YWx1ZQ==", "private-token", "private-old-value"} {
		if strings.Contains(view, value) || strings.Contains(e.yamlDocument.text, value) {
			t.Fatalf("hidden YAML retained or displayed %q", value)
		}
	}
	if !strings.Contains(view, "Values hidden") || !strings.Contains(view, "v Reveal values") {
		t.Fatalf("hidden value controls unclear: %s", view)
	}
	m.yamlDocumentKey(yamlKey('/', "/"))
	m.yamlDocumentKey(yamlKey('v', "v"))
	if e.reveal || e.yamlDocument.document.Query != "v" {
		t.Fatal("typing v in search revealed Secret values")
	}
	m.yamlDocumentKey(yamlKey(tea.KeyEscape, ""))
	m.yamlDocumentKey(yamlKey('v', "v"))
	if !e.reveal || !strings.Contains(e.yamlDocument.text, "private-token") {
		t.Fatal("explicit reveal failed")
	}
	d := e.yamlDocument
	for row, line := range strings.Split(d.document.Text(), "\n") {
		if strings.Contains(line, "private-token") {
			d.document.Cursor = yamlOffset(d, row, 0)
			break
		}
	}
	m.yamlDocumentKey(yamlKey('z', "z"))
	d.document.Query = "private-token"
	m.yamlDocumentKey(yamlKey('v', "v"))
	if e.reveal || d.document.Mode == docedit.Select || d.document.Query != "" || d.document.Searching {
		t.Fatal("hide retained revealed search or selection")
	}
	if strings.Contains(d.document.SelectedText(), "private-token") || strings.Contains(d.text, "private-token") {
		t.Fatal("copy could expose old revealed values after hiding")
	}
	for _, line := range strings.Split(d.document.Text(), "\n") {
		if strings.Contains(line, "private-token") {
			t.Fatal("reader retained revealed text after hiding")
		}
	}
	after, _ := json.Marshal(e.detail.Object)
	if string(original) != string(after) {
		t.Fatal("read-only Secret presentation changed the source object")
	}
}

func TestYAMLDocumentSearchSelectionAndEscapeLayers(t *testing.T) {
	m := yamlViewModel(t)
	m.yamlDocumentKey(yamlKey('z', "z"))
	m.yamlDocumentKey(yamlKey('/', "/"))
	m.yamlDocumentPaste("kind:\n:quit\rbb")
	d := m.explore.yamlDocument
	if !m.yamlDocumentSearching() || d.document.Query != "kind::quitbb" || d.document.Mode != docedit.Select {
		t.Fatal("search paste was not literal or changed selection")
	}
	if handled, _ := m.yamlDocumentKey(yamlKey(tea.KeyEscape, "")); !handled || d.document.Searching || d.document.Mode != docedit.Select {
		t.Fatal("first Escape did not close only search")
	}
	if handled, _ := m.yamlDocumentKey(yamlKey(tea.KeyEscape, "")); !handled || d.document.Mode == docedit.Select {
		t.Fatal("second Escape did not close only selection")
	}
	if handled, _ := m.yamlDocumentKey(yamlKey(tea.KeyEscape, "")); handled {
		t.Fatal("reader trapped resource-level Escape")
	}
	if m.explore.detail == nil || m.mode != 1 {
		t.Fatal("reader changed the host view")
	}
}

func TestYAMLDocumentPreservesReadingPositionUntilIdentityChanges(t *testing.T) {
	m := yamlViewModel(t)
	d := m.syncYAMLDocument()
	d.document.Cursor = yamlOffset(d, 8, 2)
	d.document.Anchor = yamlOffset(d, 6, 1)
	d.document.Mode = docedit.Select
	d.document.Query = "metadata"

	for _, size := range [][2]int{{80, 20}, {24, 12}, {180, 40}} {
		m.focus = 2
		m.yamlDocumentBody(size[0], size[1])
		m.focus = 1
		if m.explore.yamlDocument != d || d.document.Cursor != yamlOffset(d, 8, 2) || d.document.Mode != docedit.Select || d.document.Query != "metadata" {
			t.Fatal("resize or temporary focus lost reading state")
		}
	}
	m.explore.detail = m.explore.detail.DeepCopy()
	m.explore.detail.SetLabels(map[string]string{"refreshed": "yes"})
	m.syncYAMLDocument()
	if m.explore.yamlDocument != d || strings.Count(string([]rune(d.document.Text())[:d.document.Cursor]), "\n") != 8 || d.document.Mode == docedit.Select || d.document.Query != "" || !strings.Contains(d.document.Notice, "refreshed") {
		t.Fatal("refresh failed to retain position while clearing stale selection/search")
	}
	m.explore.detail.SetUID(types.UID("replacement"))
	fresh := m.syncYAMLDocument()
	if fresh == d || fresh.document.Cursor != 0 || fresh.document.Mode == docedit.Select || fresh.document.Query != "" {
		t.Fatal("new object inherited the previous object's reading state")
	}
	m.currentConnection().Settings["context"] = "different-context"
	if m.syncYAMLDocument() == fresh {
		t.Fatal("a different context reused the previous document identity")
	}
}

func TestYAMLDocumentFrameFitsAndUsesSharedSyntax(t *testing.T) {
	m := yamlViewModel(t)
	for _, size := range [][2]int{{18, 5}, {36, 12}, {80, 24}, {180, 40}} {
		view := m.yamlDocumentBody(size[0], size[1])
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatalf("%v frame height overflow", size)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("%v frame width overflow", size)
			}
		}
		if !strings.Contains(ansi.Strip(view), "Read only") || !strings.Contains(view, "│") {
			t.Fatalf("%v read-only state or numbered document missing: %s", size, view)
		}
	}
	view := ansi.Strip(m.yamlDocumentBody(100, 30))
	for _, want := range []string{"/ Find", "z Select", "y Copy", "apiVersion: v1", "kind: Pod"} {
		if !strings.Contains(view, want) {
			t.Fatalf("document did not expose %q", want)
		}
	}
	for _, line := range strings.Split(m.explore.yamlDocument.document.Text(), "\n") {
		if ansi.Strip(docedit.YAMLLine(line, textview.DefaultTheme())) != line {
			t.Fatal("syntax rendering altered document text")
		}
	}
}

func TestYAMLDocumentDoesNotCopyWhenDetailFailureReplacesDocument(t *testing.T) {
	for _, field := range []string{"error", "text"} {
		t.Run(field, func(t *testing.T) {
			m := yamlViewModel(t)
			m.yamlDocumentBody(80, 24)
			m.explore.detailValid = false
			if field == "error" {
				m.explore.detailError = "Resource no longer exists."
			} else {
				m.explore.detailText = "Resource no longer exists."
			}
			if m.yamlDocumentActive() {
				t.Fatal("YAML reader remained active behind the failed detail view")
			}
			if handled, copy := m.yamlDocumentKey(yamlKey('y', "y")); handled || copy != nil {
				t.Fatal("failed detail copied the stale document")
			}
			if m.yamlDocumentPaste("anything") {
				t.Fatal("failed detail accepted document input")
			}
		})
	}
}

func yamlOffset(d *yamlDocument, row, col int) int {
	lines := strings.Split(d.document.Text(), "\n")
	row = min(row, len(lines)-1)
	offset := 0
	for _, line := range lines[:row] {
		offset += utf8.RuneCountInString(line) + 1
	}
	return offset + min(col, utf8.RuneCountInString(lines[row]))
}

func TestYAMLCacheInvalidatesSourceRevisionAndReplacement(t *testing.T) {
	m := yamlViewModel(t)
	d := m.syncYAMLDocument()
	d.document.Query = "metadata"
	for range 3 {
		m.syncYAMLDocument()
	}
	if d.document.Query != "metadata" || d.document.Revision != 1 {
		t.Fatal("unchanged source reset document state")
	}
	m.explore.detail.SetLabels(map[string]string{"revision": "new"})
	m.explore.detail.SetResourceVersion("new-version")
	m.syncYAMLDocument()
	if !strings.Contains(d.document.Text(), "revision: new") || d.document.Query != "" {
		t.Fatal("updated revision reused cached YAML")
	}
	m.explore.detail = m.explore.detail.DeepCopy()
	m.explore.detail.SetLabels(map[string]string{"replacement": "new"})
	m.syncYAMLDocument()
	if !strings.Contains(d.document.Text(), "replacement: new") {
		t.Fatal("replacement object reused cached YAML")
	}
}
