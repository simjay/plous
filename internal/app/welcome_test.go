package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestStartupUsesCurrentContextAndPreservesSavedToolState(t *testing.T) {
	fixtures := map[string]func(*testing.T) *model{
		"Kubernetes": exploreModel,
		"Helm":       func(t *testing.T) *model { m, _ := helmModel(t); return m },
		"Code":       func(t *testing.T) *model { m, _ := codeModel(t); return m },
	}
	for tool, fixture := range fixtures {
		for mode, name := range modes {
			t.Run(tool+"/"+name, func(t *testing.T) {
				m := fixture(t)
				m.mode, m.focus = mode, 2
				m.draft().Text = "Keep this workspace draft"
				if err := m.saveSession(); err != nil {
					t.Fatal(err)
				}
				restored, err := newModel(m.store, m.history, "test")
				if err != nil {
					t.Fatal(err)
				}
				if restored.mode != 0 || restored.focus != 1 || restored.currentConnection() != nil || restored.tool != "" {
					t.Fatal("startup reopened the saved tool or tab")
				}
				if restored.active != "" || !restored.currentRoot || restored.drafts[m.conversationScope()].Text != "Keep this workspace draft" {
					t.Fatal("startup lost the workspace or its draft")
				}
				body := ansi.Strip(restored.mainBody(90, 32))
				if !strings.Contains(body, "Current Workspace") {
					t.Fatal("startup did not show Current Workspace", body)
				}
				if restored.syncExplore() != nil || restored.syncHelm() != nil || restored.syncActivity() != nil {
					t.Fatal("the welcome page started the saved connection or Activity")
				}
				restored.selectScope(m.active, m.tool, m.connection)
				if restored.currentConnection() == nil || restored.connection != m.connection || restored.draft().Text != "Keep this workspace draft" {
					t.Fatal("the saved connection could not be selected from the welcome page")
				}
			})
		}
	}
}

func TestFirstLaunchOffersCurrentContextAndOptionalWorkspace(t *testing.T) {
	m, _ := testModel(t)
	body := ansi.Strip(m.mainBody(90, 32))
	if !strings.Contains(body, "Current Workspace") {
		t.Fatal("first launch did not show Current Workspace", body)
	}
	press(t, m, "enter")
	if m.explore == nil || m.explore.InputKind != "connections" {
		t.Fatal("Enter did not open context selection")
	}
	press(t, m, "esc")
	mainCommand(t, m, "n")
	if m.form == nil || m.form.Kind != "workspace" {
		t.Fatal(":n did not open optional workspace setup")
	}
}
