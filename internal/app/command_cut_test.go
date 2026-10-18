package app

import (
	"strings"
	"testing"
)

func TestRetiredFirstCutCommandsDoNotRouteOrChangeScope(t *testing.T) {
	for _, command := range []string{"k", "kinds", "t", "build", "T", "activitytype", "M", "recovercontext"} {
		t.Run(command, func(t *testing.T) {
			m := exploreModel(t)
			before, connection := m.explore.Preference, m.connection
			for _, focus := range []int{0, 1, 2} {
				m.focus = focus
				m.execute(command)
				if !strings.Contains(m.status, "Unknown command") || m.focus != focus || m.connection != connection || m.explore.Preference != before || m.explore.Picker != "" {
					t.Fatal("retired command changed navigation or scope", command, m.status)
				}
			}
		})
	}
}

func TestCodeRenderAndNamedValidationReplaceBuildShortcut(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected = "plain:plain/service.yaml"
	s.OpenFindings(true)
	s.Pending = ""
	if m.executeCode("build") {
		t.Fatal("retired build command remains active")
	}
	m.updateCode("t")
	if s.Pending != "" {
		t.Fatal("retired local key started work")
	}
	m.execute("r")
	if s.Pending != "render" {
		t.Fatal("Render in Findings must render the selection, not validate every entry")
	}
	s.Pending = ""
	m.width, m.height = 160, 40
	clickMouse(t, m, "main", "code-validate-all/")
	if s.Pending != "check" {
		t.Fatal("named Validate All control is unreachable")
	}
	s.Pending = ""
	s.Busy = true
	m.mouseItem("code-validate-all", "", false)
	if s.Pending != "" {
		t.Fatal("validation control bypassed the running-operation guard")
	}
}

func TestActivityTypeUsesArrowsInsteadOfLetterShortcut(t *testing.T) {
	m := activityTimelineModel(t)
	before := m.activityUI.Filter
	typeText(m, "T")
	if m.activityUI.Filter != before {
		t.Fatal("retired Activity shortcut still cycles types")
	}
	press(t, m, "right")
	if m.activityUI.Filter == before {
		t.Fatal("Activity arrows lost type navigation")
	}
}

func TestConnectionPickerRemainsVisibleAfterResourcePickerRemoval(t *testing.T) {
	m := exploreModel(t)
	m.execute("K")
	choices := m.exploreConnections()
	view := m.explorePickerBody(100, 20)
	if m.explore.Picker != "connections" || len(choices) == 0 || !strings.Contains(view, "Choose Kubernetes connection") || !strings.Contains(view, choices[0].Label) {
		t.Fatal("connection picker lost its visible choices", view)
	}
	press(t, m, "enter")
	if m.explore.Picker != "" || m.connection != choices[0].Connection.ID {
		t.Fatal("connection picker did not open its selected connection")
	}
}
