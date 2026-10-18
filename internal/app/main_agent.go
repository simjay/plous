package app

import (
	"github.com/simjay/plous/internal/ai/agent"
	appchat "github.com/simjay/plous/internal/app/chat"
	appsettings "github.com/simjay/plous/internal/app/settings"
)

type nativeAgentChange struct {
	session *appchat.Native
	form    *appsettings.Form
	kind    agent.Kind
}

func (m *model) beginAgentSettings() {
	m.beginSettingsOverlay()
	m.toolSetup = nil
	m.command = nil
	m.detail = ""
	m.form = appsettings.AgentForm(m.settings.MainAgent)
	m.form.RefreshAgent(m.settings.Configs())
	m.page, m.status = "", ""
}

func (m *model) returnToAgent(parent *appsettings.Form) {
	if parent == nil {
		parent = appsettings.AgentForm(agent.Plous)
	}
	parent.RefreshAgent(m.settings.Configs())
	m.form = parent
	m.page, m.status = "", ""
}

func (m *model) activateAgentRow(row appsettings.Row) {
	switch row.Kind {
	case appsettings.ProviderRow:
		for i, config := range m.settings.Configs() {
			if "provider/"+config.Name == row.ID {
				m.beginAISettings(i)
				return
			}
		}
	case appsettings.AddProviderRow:
		m.beginAppSettings()
	case appsettings.SaveAgentRow:
		m.submitForm()
	case appsettings.CancelAgentRow:
		m.cancelForm()
	}
}

func (m *model) saveMainAgent(kind agent.Kind) {
	if !kind.Valid() {
		m.status = appsettings.AgentAvailability(kind)
		return
	}
	if kind == m.settings.MainAgent.Effective() {
		m.finishAgentForm()
		return
	}
	if m.run != nil {
		m.status = "Wait for the current response to finish or stop it with :X before changing agents."
		return
	}
	if n := m.nativeAgent; n != nil {
		if n.SaveErr != nil {
			m.status = "Retry the evidence save with :X, then Save the agent choice."
			return
		}
		if !n.Finished {
			m.nativeChange = &nativeAgentChange{session: n, form: m.form, kind: kind}
			m.nativeRequested = false
			n.Captured = false
			n.Close()
			m.status = "Closing " + n.Kind.Label() + " and saving tool evidence before switching to " + kind.Label() + "."
			return
		}
	}
	m.applyMainAgent(kind)
}

func (m *model) finishNativeAgentChange(n *appchat.Native) {
	change := m.nativeChange
	if change == nil || change.session != n {
		return
	}
	m.nativeChange = nil
	// A completed close must not apply a canceled or subsequently edited choice.
	if m.quitting || m.nativeAgent != n || m.form != change.form || appsettings.AgentKind(m.form.Fields[0].Value.Text) != change.kind {
		return
	}
	if n.SaveErr != nil {
		m.status = "Evidence save failed. :X retries the save; then Save the agent choice."
		return
	}
	m.applyMainAgent(change.kind)
}

func (m *model) applyMainAgent(kind agent.Kind) {
	next := m.settings
	next.MainAgent = kind
	if !m.saveAISettings(next) {
		return
	}
	m.nativeAgent, m.nativeChange = nil, nil
	m.nativeError = ""
	m.nativeFresh = false
	m.nativeRequested = m.externalAgentSelected()
	m.finishAgentForm()
}

func (m *model) finishAgentForm() {
	m.nativeChange = nil
	m.form = nil
	m.page = "settings"
	m.selectSettingsRow("agent")
	m.status = "Agent: " + m.settings.MainAgent.Label() + ". " + appsettings.AgentAvailability(m.settings.MainAgent)
}
