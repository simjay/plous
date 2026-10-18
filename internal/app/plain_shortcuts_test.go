package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/ai/history"
	textview "github.com/simjay/plous/internal/ui/text"

	"strings"
	"testing"
)

func TestResourceRefreshUsesPlainKeyOrCommand(t *testing.T) {
	for _, command := range []string{"key g", "g", "refresh"} {
		t.Run(command, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			watch := e.Stream.Ctx
			m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
			if watch.Err() != nil {
				t.Fatal("retired Ctrl+R refreshed the resource table")
			}
			if command == "key g" {
				typeText(m, "g")
			} else {
				mainCommand(t, m, command)
			}
			if watch.Err() == nil || m.explore.Preference.Namespace != "shop" || m.explore.Preference.Resource != "pods" {
				t.Fatal("refresh did not replace the request while retaining scope")
			}
		})
	}
}

func TestRestorePromptCommandRetainsDraftAndDocument(t *testing.T) {
	for _, command := range []string{"p", "restoreprompt"} {
		t.Run(command, func(t *testing.T) {
			m, a := documentModel(t)
			a.Document.Paste("# unsaved\n")
			document := a.Document.Text()
			m.chat().Turns = []history.Turn{{Request: history.Request{Prompt: "/model"}}}
			m.draft().Insert("unfinished", true)
			mainCommand(t, m, command)
			if m.draft().Text != "unfinished" || !strings.Contains(m.status, "Clear the draft") {
				t.Fatal("restore replaced an unfinished draft")
			}
			*m.draft() = textview.NewField("")
			m.focusChat(true)
			m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
			if m.draft().Text != "" {
				t.Fatal("retired Ctrl+R restored a prompt")
			}
			press(t, m, "esc")
			m.rightHidden = true
			mainCommand(t, m, command)
			if m.focus != 2 || !m.chatEditing || m.rightHidden || m.draft().Text != "/model" || !m.draft().Literal {
				t.Fatal("restore should open a literal draft without sending")
			}
			if len(m.chat().Turns) != 1 || m.run != nil || m.explore.Action != a || a.Document.Text() != document || a.Pending != "" {
				t.Fatal("restoring a prompt sent a request or changed the document")
			}
		})
	}
	t.Run("empty history", func(t *testing.T) {
		m, _ := testModel(t)
		m.execute("p")
		if m.draft().Text != "" || !strings.Contains(m.status, "No previous message") {
			t.Fatal("empty history should explain why there is nothing to restore")
		}
	})
}

func TestEditorValidationUsesSaveCommand(t *testing.T) {
	m, a := documentModel(t)
	before := a.Document.Text()
	for _, insert := range []bool{false, true} {
		if insert {
			typeText(m, "i")
		}
		m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		if a.Pending != "" || a.Busy || a.Document.Text() != before {
			t.Fatal("retired Ctrl+S changed or validated the draft")
		}
	}
	press(t, m, "esc")
	a.Document.Paste("# changed\n")
	before = a.Document.Text()
	m.execute("w")
	if a.Pending != "validate" || a.Document.Text() != before {
		t.Fatal(":w should request validation without changing the document")
	}
}
