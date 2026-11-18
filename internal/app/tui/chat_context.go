package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/simjay/plous/internal/ai"
	backend "github.com/simjay/plous/internal/ai/agent"
	aimodel "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/app/monitor"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	"github.com/simjay/plous/internal/ui"
)

func (m *model) contextText(connections []ai.Connection) string {
	if m.savedConnection() == nil {
		return ""
	}
	// Capture identities and filters only; resource bodies are retrieved through tools.
	value := struct {
		Workspace   string                `json:"workspace"`
		Revision    uint64                `json:"revision"`
		Connections []ai.Connection       `json:"connections"`
		Viewing     appchat.AssistantPage `json:"viewing"`
	}{m.current().Name, m.current().Revision, connections, m.assistantPageContext()}
	data, _ := json.MarshalIndent(value, "", "  ")
	return string(data)
}

func (m *model) chatMessages(options ai.Options, draft string, preview bool) []aimodel.Message {
	c := m.chat()
	response := ""
	if preview && m.run != nil && m.run.Chat == c && len(c.Turns) > 0 {
		t := c.Turns[len(c.Turns)-1]
		if c.ActiveTurn != nil {
			t = *c.ActiveTurn
		}
		if t.Response.Status == "streaming" {
			draft, response = t.Request.Prompt, t.Response.Text
		}
	}
	messages := ai.PreviewMessages(options.System, options.ScopeContext, draft)
	if len(c.Messages) > 0 {
		recent, err := aimodel.Prepare(c.Messages, nil)
		if err == nil {
			previewMessages := messages[:1:1]
			for _, message := range recent.Messages {
				data, _ := json.Marshal(message)
				previewMessages = append(previewMessages, aimodel.Message{Role: string(message.Role), Text: string(data)})
			}
			messages = append(previewMessages, messages[len(messages)-1])
		}
	}
	if response != "" {
		messages = append(messages, aimodel.Message{Role: "assistant", Text: response})
	}
	return messages
}

func (m *model) assistantOptions() ai.Options {
	connections := m.assistantConnections()
	options := ai.Options{
		Debug: m.debugPolicy, UI: m.uiTools(connections), Connections: connections,
		Store: m.store, History: m.history, WorkspaceID: m.conversationScope(),
		Agent: m.settings.MainAgent.Effective(), System: ai.Instructions, ScopeContext: m.contextText(connections),
	}
	if instructions := m.settings.Agent.Instructions; instructions != "" {
		options.System += "\n\n" + instructions
	}
	if m.monitorRoot != "" {
		options.Monitor = monitor.ReadTool{Root: m.monitorRoot}
	}
	return options
}

func (m *model) chatContextUsage() (used, limit int) {
	if m.settings.MainAgent.Effective() != backend.Plous {
		return 0, 0
	}
	if m.run != nil && m.run.Chat == m.chat() && m.run.ContextLimit > 0 {
		return m.run.ContextUsed, m.run.ContextLimit
	}
	model, ok := aimodel.LookupModel(m.settings.Agent.Provider, m.settings.Agent.Model)
	if !ok {
		return 0, 0
	}
	options := m.assistantOptions()
	messages := m.chatMessages(options, m.draft().Text, true)
	return appchat.EstimateTokens(messages) + ai.EstimateTools(ai.ToolDefinitions(options)), model.ContextWindow
}

func (m *model) chatContextLine(width int) string {
	if m.chat().Restore != nil {
		return ui.Clipped("Context: restoring saved conversation", width)
	}
	used, limit := m.chatContextUsage()
	if limit == 0 {
		if m.settings.MainAgent.Effective() != backend.Plous {
			return ui.Clipped("Context: managed by "+m.settings.MainAgent.Label(), width)
		}
		return ui.Clipped("Context: no model", width)
	}
	label := fmt.Sprintf("Context ~%s / %s tokens", appchat.CompactTokens(used), appchat.CompactTokens(limit))
	if len(label) > width {
		label = fmt.Sprintf("~%s/%s tokens", appchat.CompactTokens(used), appchat.CompactTokens(limit))
	}
	return ui.Clipped(label, width)
}

func (m *model) assistantPageContext() appchat.AssistantPage {
	names := []string{"Overview", "Explore", "Activity"}
	page := appchat.AssistantPage{View: names[max(0, min(m.mode, len(names)-1))]}
	c := m.currentConnection()
	if c != nil {
		page.ConnectionID, page.Connection, page.Tool, page.Context = c.ID, c.Name, c.Tool, c.Settings["context"]
	}
	if c != nil && m.savedConnection() != nil && monitorCluster(c.Tool) && m.savedConnection().Tool != c.Tool {
		page.ConnectionID += "/" + c.Tool
	}

	if m.gitOpsVisible() {
		v := m.gitOpsCurrent()
		page.Tool = "kubernetes"
		if m.savedConnection().Tool != "kubernetes" {
			page.ConnectionID = c.ID + "/kubernetes"
		}
		page.View = "Explore / " + v.State.Title()
		page.Namespace, page.Filter = assistantPageNamespace(v.State.Namespace), v.State.Query
		if item := v.State.Detail; item != nil {
			page.Name, page.Namespace = item.Ref.Name, item.Ref.Namespace
			if resource, ok := gitops.FindResource(v.Cluster.Catalog, item.Ref); ok {
				page.Resource = resource.Key()
			}
		}
		return page
	}
	if m.helmActive() {
		h := m.helmer()
		page.View = "Explore / Helm / " + h.Preference.Section
		page.Namespace = assistantPageNamespace(h.Preference.Namespace)
		page.Filter = h.Preference.Filter
		if h.Timeline != nil {
			page.Name, page.Namespace, page.Resource = h.Timeline.Release.Name, h.Timeline.Release.Namespace, "release"
			page.Revision = h.Timeline.Selected()
		}
		if h.Detail != nil {
			page.Name = h.Detail.Name
			page.Namespace = h.Detail.Namespace
			page.Resource = "release"
			page.Revision = h.Detail.Version
		}
		if h.WorkflowVisible() && h.Plan != nil {
			page.Name = h.Plan.Name
			page.Namespace = h.Plan.Namespace
			page.Resource = "release"
			page.Revision = h.Plan.CurrentRevision
		}
		return page
	}
	e := m.explore
	if m.mode != 1 || c == nil || c.Tool != "kubernetes" || e == nil {
		return page
	}
	expected := strings.Join([]string{m.active, c.ID, c.Settings["kubeconfig"], c.Settings["context"], c.Settings["namespace"]}, "\x00")
	if e.Key != expected {
		return page
	}
	page.Resource = e.Resource.Key()
	if e.Resource.Namespaced {
		page.Namespace = assistantPageNamespace(e.Preference.Namespace)
	}
	page.Filter = e.Preference.Filter
	page.Labels = e.Preference.Selector
	if e.Detail != nil {
		page.Name = e.Detail.GetName()
		page.UID = string(e.Detail.GetUID())
		page.Namespace = e.Detail.GetNamespace()
	}
	return page
}

func assistantPageNamespace(namespace string) string {
	if namespace == "" {
		return "*"
	}
	return namespace
}

func (m *model) assistantConnections() []ai.Connection {
	selected := m.savedConnection()
	if selected == nil || !monitorCluster(selected.Tool) || m.historyView != "" {
		return nil
	}
	captured := ai.Connection{TargetCaptured: true, Target: selected.Target, ConnectionID: selected.ID, Name: selected.Name, Tool: selected.Tool, Path: selected.Settings["kubeconfig"], Context: selected.Settings["context"], Namespace: selected.Settings["namespace"], Driver: m.helmDriver(m.active, *selected)}
	return ai.ClusterConnections(captured)
}
