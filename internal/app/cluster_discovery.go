package app

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/workspace"
)

type clusterState struct {
	Key, Workspace string
	Connection     workspace.Connection
	Client         *kubernetes.Client
	Catalog        []kubecommon.Resource
	Warning        string
	Err            error
	Loaded         bool
	Run            uint64
	Cancel         context.CancelFunc
	HelmDriver     string
	HelmErr        error
	HelmChecked    bool
	HelmRun        uint64
	HelmCancel     context.CancelFunc
}

type clusterDiscoveryResult struct {
	Cluster *clusterState
	Run     uint64
	Client  *kubernetes.Client
	Target  *kubecommon.Target
	Catalog []kubecommon.Resource
	Warning string
	Err     error
}

func clusterKey(ws string, c workspace.Connection) string {
	return strings.Join([]string{ws, c.ID, c.Settings["kubeconfig"], c.Settings["context"], c.Settings["namespace"]}, "\x00")
}

func clusterKubeconfig(c workspace.Connection) string {
	if c.Tool == "helm" && c.Settings["kubeconfig"] == "" {
		return "~/.kube/config"
	}
	return c.Settings["kubeconfig"]
}

func (m *model) clusterFor(ws string, c workspace.Connection) *clusterState {
	if m.clusterStates == nil {
		m.clusterStates = map[string]*clusterState{}
	}
	key := clusterKey(ws, c)
	cluster := m.clusterStates[key]
	if cluster != nil {
		previous, next := cluster.Connection.Target, c.Target
		changedTarget := c.TargetCaptured && (previous == nil) != (next == nil) || previous != nil && next != nil && previous.Fingerprint != next.Fingerprint
		if changedTarget || cluster.Connection.Settings["driver"] != c.Settings["driver"] {
			cluster.stopDiscovery()
			cluster = nil
		}
	}
	if cluster == nil {
		cluster = &clusterState{Key: key, Workspace: ws, Connection: c}
		m.clusterStates[key] = cluster
	}
	return cluster
}

func (m *model) discoverCluster(c *clusterState) tea.Cmd {
	c.Run++
	run, connection := c.Run, c.Connection
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	c.Cancel = cancel
	return func() tea.Msg {
		defer cancel()
		r := clusterDiscoveryResult{Cluster: c, Run: run}
		if connection.Target == nil && connection.TargetCaptured {
			r.Err = errors.New("The captured cluster configuration is unavailable. Reselect the connection.")
		} else if connection.Target == nil {
			connection.Target, r.Err = kubecommon.ResolveTarget(clusterKubeconfig(connection), connection.Settings["context"], connection.Settings["namespace"])
		}
		if r.Err == nil {
			r.Target = connection.Target
			r.Client, r.Err = kubernetes.NewTarget(connection.Target, connection.Settings["namespace"])
		}
		if r.Err == nil {
			r.Catalog, r.Warning, r.Err = r.Client.Discover(ctx)
		}
		return r
	}
}

func (m *model) applyClusterDiscovery(r clusterDiscoveryResult) {
	c := r.Cluster
	if c == nil || c.Run != r.Run || c.Cancel == nil {
		return
	}
	c.Cancel, c.Loaded = nil, true
	c.Client, c.Catalog, c.Warning, c.Err = r.Client, r.Catalog, r.Warning, r.Err
	c.Connection.Target, c.Connection.TargetCaptured = r.Target, true
	if e := m.explore; e != nil && e.Key == c.Key && !e.Connecting && m.exploreActive() && r.Err == nil {
		e.Client, e.Resources, e.Warning = c.Client, c.Catalog, c.Warning
	}
}

func (m *model) helmDriver(ws string, c workspace.Connection) string {
	if driver := c.Settings["driver"]; driver != "" {
		return driver
	}
	if cluster := m.clusterStates[clusterKey(ws, c)]; cluster != nil && cluster.HelmDriver == "configmap" {
		return "configmap"
	}
	return ""
}

type helmDetectionResult struct {
	Cluster *clusterState
	Run     uint64
	Driver  string
	Err     error
}

func (m *model) syncClusters() tea.Cmd {
	if m.exploreClosed || m.quitting {
		return nil
	}
	var cmds []tea.Cmd
	for _, w := range m.treeWorkspaces() {
		for _, c := range m.sidebarConnections(w) {
			if !monitorCluster(c.Tool) {
				continue
			}
			savedOpen, set := m.expanded["saved-workspaces"]
			areaOpen, areaSet := m.expanded[w.ID+"/monitor"]
			visible := m.expanded[w.ID] && m.expanded[c.ID] && (!areaSet || areaOpen) && (workspace.IsCurrentScope(w.ID) || !set || savedOpen)
			active := m.active == w.ID && m.connection == c.ID && (monitorCluster(m.tool) || gitOpsTool(m.tool))
			if !visible && !active {
				continue
			}
			cluster := m.clusterFor(w.ID, c)
			if !cluster.Loaded && cluster.Cancel == nil {
				cmds = append(cmds, m.discoverCluster(cluster))
			}
			if cluster.Loaded && cluster.Client != nil && !cluster.HelmChecked && cluster.HelmCancel == nil {
				cmds = append(cmds, m.detectHelm(cluster))
			}
		}
	}
	return tea.Batch(cmds...)
}

func (m *model) clusterCapabilities(ws string, c workspace.Connection) []string {
	tools := []string{"kubernetes"}
	if cluster := m.clusterStates[clusterKey(ws, c)]; cluster != nil && cluster.HelmDriver != "" {
		tools = append(tools, "helm")
	}
	return append(tools, m.gitOpsCapabilities(ws, c)...)
}

func (m *model) detectHelm(c *clusterState) tea.Cmd {
	c.HelmRun++
	run, client, driver := c.HelmRun, c.Client, c.Connection.Settings["driver"]
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	c.HelmCancel = cancel
	return func() tea.Msg {
		defer cancel()
		found, err := helm.Detect(ctx, client, driver)
		return helmDetectionResult{Cluster: c, Run: run, Driver: found, Err: err}
	}
}

func (m *model) applyHelmDetection(r helmDetectionResult) {
	c := r.Cluster
	if c == nil || c.HelmRun != r.Run || c.HelmCancel == nil {
		return
	}
	c.HelmCancel, c.HelmChecked, c.HelmErr = nil, true, r.Err
	// Failed refreshes cannot disprove a previously observed installation.
	if r.Err == nil || r.Driver != "" {
		c.HelmDriver = r.Driver
	}
}

func (c *clusterState) stopDiscovery() {
	if c.Cancel != nil {
		c.Cancel()
		c.Cancel = nil
		c.Run++
	}
	if c.HelmCancel != nil {
		c.HelmCancel()
		c.HelmCancel = nil
		c.HelmRun++
	}
}

func (c *clusterState) refresh() {
	c.stopDiscovery()
	c.Loaded, c.HelmChecked = false, false
}

func (m *model) stopClusters() {
	for _, c := range m.clusterStates {
		c.stopDiscovery()
	}
}
