package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/ai/agent"
	appchat "github.com/simjay/plous/internal/app/chat"
	appsettings "github.com/simjay/plous/internal/app/settings"
	textview "github.com/simjay/plous/internal/ui/text"
)

func TestAgentPickerExcludesUnsupportedAgents(t *testing.T) {
	m, n := nativeModel(t)
	m.openSettings()
	m.selectSettingsItem("Agent: Claude Code")
	m.updateSettings("enter")
	if !slices.Equal(m.form.Fields[0].Options, []string{"Plous", "Claude Code", "Codex"}) {
		t.Fatal("agent picker offers unsupported choices", m.form.Fields[0].Options)
	}
	m.form.Fields[0].Value = textview.NewField("OpenCode")
	m.form.Index = len(m.form.Fields)
	m.submitForm()
	if m.settings.MainAgent != agent.ClaudeCode || m.form == nil || m.nativeChange != nil || n.Input.Ctx.Err() != nil {
		t.Fatal("unsupported choice replaced or closed the active agent")
	}
	if !strings.Contains(m.status, "not supported") {
		t.Fatal("unsupported choice was reported as available", m.status)
	}
}

func TestNativeAgentChangeWaitsForEvidenceAndPreservesFocus(t *testing.T) {
	m, n := nativeModel(t)
	t.Setenv("PATH", t.TempDir())
	m.openSettings()
	m.selectSettingsItem("Agent: Claude Code")
	m.updateSettings("enter")
	press(t, m, "right")
	m.form.Index = len(m.form.Fields)
	m.submitForm()
	if m.settings.MainAgent != agent.ClaudeCode || n.Input.Ctx.Err() != context.Canceled || m.nativeChange == nil || n.Captured {
		t.Fatal("Save did not close the native session before applying the choice")
	}
	m.focus = 0
	m.openHelp()
	help := m.help
	m.draft().Insert("retained Plous draft", true)
	m.Update(nativeAgentResult{session: n})
	saved, err := m.store.ReadSettings()
	if err != nil || saved.MainAgent != agent.Codex || m.form != nil || m.nativeChange != nil || m.page != "settings" {
		t.Fatal("successful closure did not save the requested agent", saved, err)
	}
	if m.focus != 0 || m.help != help || m.draft().Text != "retained Plous draft" || m.nativeAgent != nil {
		t.Fatal("switch stole focus, lost the draft, or retained the old terminal")
	}
	if !strings.Contains(m.status, "unavailable") {
		t.Fatal("missing new executable was not reported", m.status)
	}
}

func TestNativeAgentChangeDoesNotApplyCanceledOrFailedSave(t *testing.T) {
	for _, action := range []string{"cancel", "edit", "quit", "evidence failure", "settings conflict"} {
		t.Run(action, func(t *testing.T) {
			m, n := nativeModel(t)
			t.Setenv("PATH", t.TempDir())
			if !m.saveAISettings(m.settings) {
				t.Fatal(m.status)
			}
			m.openSettings()
			m.selectSettingsItem("Agent: Claude Code")
			m.updateSettings("enter")
			press(t, m, "right")
			form := m.form
			m.submitForm()
			result := nativeAgentResult{session: n}
			switch action {
			case "cancel":
				m.cancelForm()
			case "edit":
				press(t, m, "right")
			case "quit":
				m.quitting = true
			case "evidence failure":
				result.saveErr = errors.New("fixture storage unavailable")
			case "settings conflict":
				next := m.settings
				next.MouseDisabled = !next.MouseDisabled
				if _, err := m.store.SaveSettings(next); err != nil {
					t.Fatal(err)
				}
			}
			m.Update(result)
			if m.settings.MainAgent != agent.ClaudeCode || m.nativeChange != nil {
				t.Fatal("late closure applied an unconfirmed choice")
			}
			if action == "evidence failure" || action == "settings conflict" {
				if m.form != form || appsettings.AgentKind(form.Fields[0].Value.Text) != agent.Codex {
					t.Fatal("failed save lost the pending form choice")
				}
				status := m.status
				m.View()
				if m.status != status {
					t.Fatal("rendering hid the save error", m.status)
				}
			}
		})
	}
}

func TestSavingCurrentAgentKeepsNativeSession(t *testing.T) {
	m, n := nativeModel(t)
	m.openSettings()
	m.selectSettingsItem("Agent: Claude Code")
	m.updateSettings("enter")
	m.submitForm()
	if n.Input.Ctx.Err() != nil || m.nativeAgent != n || m.nativeChange != nil || m.form != nil {
		t.Fatal("saving an unchanged choice restarted the native session")
	}
}

func TestMainAgentSettingsAndMissingExecutable(t *testing.T) {
	m, _ := testModel(t)
	if m.settings.MainAgent.Effective() != agent.Plous {
		t.Fatal("fresh profile did not choose Plous")
	}
	t.Setenv("PATH", t.TempDir())
	m.openSettings()
	m.selectSettingsItem("Agent: Plous")
	m.updateSettings("enter")
	if m.form == nil || m.form.Kind != "mainagent" {
		t.Fatal("missing Settings agent picker")
	}
	press(t, m, "right")
	if m.form.Fields[0].Value.Text != "Claude Code" {
		t.Fatal("wrong first external choice")
	}
	m.form.Index = len(m.form.Fields)
	m.submitForm()
	saved, err := m.store.ReadSettings()
	if err != nil || saved.MainAgent != agent.ClaudeCode {
		t.Fatal(saved, err)
	}
	if !strings.Contains(m.status, "unavailable") {
		t.Fatal("missing executable not explained", m.status)
	}
	m.closeSettings()
	m.loadChat()
	m.focusChat(true)
	*m.draft() = textview.NewField("Keep this question")
	if cmd := m.sendChat(); cmd != nil || m.run != nil || m.draft().Text != "Keep this question" || !strings.Contains(m.status, "unavailable") {
		t.Fatal("unavailable runtime lost draft or dispatched", m.status)
	}
	if _, exists := helpEntries(m.chatHelp())["model"]; exists {
		t.Fatal("external-agent Help advertised the Plous model picker")
	}
	if strings.Contains(m.chatBody(60, 24), "/model") {
		t.Fatal("external agent offered Plous model selection")
	}
	*m.draft() = textview.NewField("/model")
	press(t, m, "enter")
	if m.chat().Models || m.draft().Text != "/model" {
		t.Fatal("external CLI input opened Plous model picker")
	}
}

func TestMainAgentCannotReplaceRunningTurn(t *testing.T) {
	m, _ := testModel(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	run := &appchat.ChatRun{Agent: agent.Plous, Chat: m.chat(), Cancel: cancel}
	m.run = run
	m.openSettings()
	m.selectSettingsItem("Agent: Plous")
	m.updateSettings("enter")
	press(t, m, "right")
	m.form.Index = len(m.form.Fields)
	m.submitForm()
	if m.settings.MainAgent.Effective() != agent.Plous || m.run != run || ctx.Err() != nil || m.form == nil {
		t.Fatal("agent choice changed an active turn")
	}
	m.View()
	if !strings.Contains(m.status, "current response") {
		t.Fatal("rendering hid the reason switching is blocked", m.status)
	}
	m.run = nil
}
