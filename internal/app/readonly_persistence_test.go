package app

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"testing"
)

func readOnlyPersistenceModel(t *testing.T, view string) *model {
	t.Helper()
	if view == "YAML" {
		return yamlViewModel(t)
	}
	m, _ := testModel(t)
	createWorkspace(t, m, "Commerce")
	mainCommand(t, m, "3")
	press(t, m, "enter")
	typeText(m, "d")
	if !m.activityReviewVisible() {
		t.Fatal("Activity comparison did not open")
	}
	return m
}

func TestReadOnlyGlobalShortcutsAutosaveSession(t *testing.T) {
	for _, view := range []string{"Activity", "YAML"} {
		for _, shortcut := range []string{"bb", "ii", "11", "22", "33"} {
			t.Run(view+"/"+shortcut, func(t *testing.T) {
				m := readOnlyPersistenceModel(t, view)
				if err := m.saveSession(); err != nil {
					t.Fatal(err)
				}
				initialMode := m.mode
				typeText(m, shortcut[:1])
				m.Update(autosaveMsg{})
				m.drainPersistence()
				if m.dirty {
					t.Fatal("autosave between prefix keys should clear dirty state")
				}
				typeText(m, shortcut[1:])
				if !m.dirty {
					t.Fatal("completed global shortcut must mark the session for autosave")
				}
				m.Update(autosaveMsg{})
				m.drainPersistence()
				data, err := m.store.ReadSession()
				if err != nil {
					t.Fatal(err)
				}
				var saved session
				if err := json.Unmarshal(data, &saved); err != nil {
					t.Fatal(err)
				}
				wantMode := initialMode
				if shortcut[0] >= '1' && shortcut[0] <= '3' {
					wantMode = int(shortcut[0] - '1')
				}
				if saved.Mode != wantMode || saved.LeftHidden != (shortcut == "bb") || saved.RightHidden || (saved.AssistantSize == assistantDouble) != (shortcut == "ii") {
					t.Fatalf("autosave lost completed shortcut: mode=%d, sidebar hidden=%t, AI hidden=%t", saved.Mode, saved.LeftHidden, saved.RightHidden)
				}
			})
		}
	}
}

func TestReadOnlySearchDoesNotRunOrPersistGlobalShortcuts(t *testing.T) {
	for _, view := range []string{"Activity", "YAML"} {
		t.Run(view, func(t *testing.T) {
			m := readOnlyPersistenceModel(t, view)
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			before, err := m.store.ReadSession()
			if err != nil {
				t.Fatal(err)
			}
			mode := m.mode
			typeText(m, "/bbii112233:settings")
			m.Update(tea.PasteMsg{Content: "\n:quit"})
			query := ""
			if view == "Activity" {
				query = m.activityReview.Comparison.Reader.Query
			} else {
				query = m.explore.YamlDocument.Document.Query
			}
			if query != "bbii112233:settings:quit" || m.mode != mode || m.leftHidden || m.rightHidden || m.command != nil || m.settingsOverlay != nil || m.dirty {
				t.Fatal("literal search input reached global shortcuts or session persistence")
			}
			m.Update(autosaveMsg{})
			m.drainPersistence()
			after, err := m.store.ReadSession()
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("transient reader search changed the persisted session")
			}
		})
	}
}
