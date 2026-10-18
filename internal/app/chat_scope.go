package app

import (
	"github.com/simjay/plous/internal/workspace"
	"path/filepath"
	"strings"

	agent "github.com/simjay/plous/internal/ai"
	appchat "github.com/simjay/plous/internal/app/chat"
	appcode "github.com/simjay/plous/internal/app/code"
	appsettings "github.com/simjay/plous/internal/app/settings"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
)

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
	if m.codeActive() {
		page.Tool = "code"
		page.View = "Explore / Code / " + appsettings.ToolNames[m.tool]
		if s := m.coder(); s != nil {
			selection := s.AssistantSelection()
			page.Source = &selection
			page.View = "Explore / Code / " + s.ViewLabel()
		}
		return page
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

func (m *model) assistantConnections() []agent.Connection {
	selected := m.savedConnection()
	if selected == nil || m.historyView != "" {
		return nil
	}
	connections := make([]agent.Connection, 0, 2)
	for _, c := range []workspace.Connection{*selected} {
		captured := agent.Connection{TargetCaptured: true, Target: c.Target, ConnectionID: c.ID, Name: c.Name, Tool: c.Tool, Path: c.Settings["kubeconfig"], Context: c.Settings["context"], Namespace: c.Settings["namespace"], Driver: c.Settings["driver"]}
		if monitorCluster(c.Tool) {
			captured.Driver = m.helmDriver(m.active, c)
		}
		if sourceConnection(c.Tool) {
			config := appcode.ConfigForConnection(&c)
			captured.Tool, captured.Path = "source", ""
			if config.Repository == "" && config.Path != "" {
				captured.Tool = "source"
				captured.Path = config.Path
				if absolute, err := filepath.Abs(captured.Path); err == nil {
					captured.Path = absolute
				}
				if canonical, err := filepath.EvalSymlinks(captured.Path); err == nil {
					captured.Path = canonical
				}
				if config.Subdirectory != "" {
					captured.Path = filepath.Join(captured.Path, config.Subdirectory)
				}
			}
		}
		connections = append(connections, agent.ClusterConnections(captured)...)
	}
	return connections
}

// Capture completed Code work once, before the asynchronous conversation starts.
func (m *model) captureSourceReads(connections []agent.Connection) {
	for n := range connections {
		c := &connections[n]
		if c.Tool != "source" {
			continue
		}
		if s := m.savedCodeState(m.active, c.ConnectionID); s != nil {
			c.SourceView = s.AssistantReadView()
		}
	}
}
