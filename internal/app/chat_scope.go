package app

import (
	"github.com/simjay/plous/internal/agent"
	"strings"
)

type assistantPage struct {
	View      string `json:"view"`
	Resource  string `json:"resource,omitempty"`
	Name      string `json:"name,omitempty"`
	UID       string `json:"uid,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Filter    string `json:"filter,omitempty"`
	Labels    string `json:"labels,omitempty"`
}

func (m *model) assistantPageContext() assistantPage {
	names := []string{"Overview", "Explore", "Activity"}
	page := assistantPage{View: names[max(0, min(m.mode, len(names)-1))]}
	c := m.currentConnection()
	e := m.explore
	if m.mode != 1 || c == nil || c.Tool != "kubernetes" || e == nil {
		return page
	}
	expected := strings.Join([]string{m.active, c.ID, c.Settings["kubeconfig"], c.Settings["context"], c.Settings["namespace"]}, "\x00")
	if e.key != expected {
		return page
	}
	page.Resource = e.resource.Key()
	page.Namespace = e.preference.Namespace
	page.Filter = e.preference.Filter
	page.Labels = e.preference.Selector
	if e.detail != nil {
		page.Name = e.detail.GetName()
		page.UID = string(e.detail.GetUID())
		page.Namespace = e.detail.GetNamespace()
	}
	return page
}
func (m *model) assistantKubernetesScope() *agent.KubernetesScope {
	c := m.currentConnection()
	if c == nil || c.Tool != "kubernetes" {
		return nil
	}
	scope := &agent.KubernetesScope{Path: c.Settings["kubeconfig"], Context: c.Settings["context"], Namespace: c.Settings["namespace"], ConnectionID: c.ID}
	page := m.assistantPageContext()
	if page.Namespace != "" && page.Namespace != "*" {
		scope.Namespace = page.Namespace
	}
	return scope
}
