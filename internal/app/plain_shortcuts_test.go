package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/workspace"
)

func TestResourceRefreshUsesPlainKeyOrCommand(t *testing.T) {
	for _, command := range []string{"key g", "g", "refresh"} {
		t.Run(command, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			watch := e.stream.ctx
			m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
			if watch.Err() != nil {
				t.Fatal("retired Ctrl+R refreshed the resource table")
			}
			if command == "key g" {
				typeText(m, "g")
			} else {
				mainCommand(t, m, command)
			}
			if watch.Err() == nil || m.explore.preference.Namespace != "shop" || m.explore.preference.Resource != "pods" {
				t.Fatal("refresh did not replace the request while retaining scope")
			}
		})
	}
}

func TestRestorePromptCommandRetainsDraftAndDocument(t *testing.T) {
	for _, command := range []string{"p", "restoreprompt"} {
		t.Run(command, func(t *testing.T) {
			m, a := documentModel(t)
			a.document.Paste("# unsaved\n")
			document := a.document.Text()
			m.chat().turns = []workspace.Turn{{Request: workspace.Request{Prompt: "/model"}}}
			m.draft().insert("unfinished", true)
			mainCommand(t, m, command)
			if m.draft().text != "unfinished" || !strings.Contains(m.status, "Clear the draft") {
				t.Fatal("restore replaced an unfinished draft")
			}
			*m.draft() = newEditor("")
			m.focusChat(true)
			m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
			if m.draft().text != "" {
				t.Fatal("retired Ctrl+R restored a prompt")
			}
			press(t, m, "esc")
			m.rightHidden = true
			mainCommand(t, m, command)
			if m.focus != 2 || !m.chatEditing || m.rightHidden || m.draft().text != "/model" || !m.draft().literal {
				t.Fatal("restore should open a literal draft without sending")
			}
			if len(m.chat().turns) != 1 || m.run != nil || m.explore.action != a || a.document.Text() != document || a.pending != "" {
				t.Fatal("restoring a prompt sent a request or changed the document")
			}
		})
	}
	t.Run("empty history", func(t *testing.T) {
		m, _ := testModel(t)
		m.execute("p")
		if m.draft().text != "" || !strings.Contains(m.status, "No previous message") {
			t.Fatal("empty history should explain why there is nothing to restore")
		}
	})
}

func TestEditorValidationUsesSaveCommand(t *testing.T) {
	m, a := documentModel(t)
	before := a.document.Text()
	for _, insert := range []bool{false, true} {
		if insert {
			typeText(m, "i")
		}
		m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		if a.pending != "" || a.busy || a.document.Text() != before {
			t.Fatal("retired Ctrl+S changed or validated the draft")
		}
	}
	press(t, m, "esc")
	a.document.Paste("# changed\n")
	before = a.document.Text()
	m.execute("w")
	if a.pending != "validate" || a.document.Text() != before {
		t.Fatal(":w should request validation without changing the document")
	}
}
