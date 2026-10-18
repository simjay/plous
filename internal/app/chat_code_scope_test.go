package app

import (
	"github.com/simjay/plous/internal/workspace"
	"testing"
)

func TestChatCapturesSelectedCodeFileAndRenderedIdentity(t *testing.T) {
	m, s := codeModel(t)
	rows := s.FileRows()
	if len(rows) < 2 {
		t.Fatal("fixture lacks selections")
	}
	s.Cursor = 0
	first := m.assistantPageContext()
	s.Cursor = 1
	second := m.assistantPageContext()
	if first.Source == nil || second.Source == nil || first.Source.Path == second.Source.Path {
		t.Fatal("Code selections collapsed", first, second)
	}
	m.tool = "manifests"
	s.Preference.Selected = "plain:plain/service.yaml"
	s.Pending = "render"
	finishCode(t, m)
	page := m.assistantPageContext()
	if page.Source.Result == "" || page.Source.Resource == "" || page.Source.SourceIdentity != s.Inventory.Source.Identity {
		t.Fatal(page)
	}
	connections := m.assistantConnections()
	m.captureSourceReads(connections)
	if connections[0].SourceView == nil {
		t.Fatal("render context has no matching read capability")
	}
}

func TestGitSourceGetsScopedReadCapabilityWithoutAcquisition(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Sources")
	m.current().Connections = []workspace.Connection{{ID: "remote", Tool: "code", Name: "Git source", Settings: map[string]string{"source_type": "Git repository", "path": "https://example.invalid/repo.git"}}}
	m.connection, m.tool = "remote", "files"
	captured := m.assistantConnections()
	m.captureSourceReads(captured)
	if len(captured) != 1 || captured[0].Tool != "source" || captured[0].Path != "" || captured[0].SourceView != nil {
		t.Fatal(captured)
	}
}
