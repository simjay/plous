package app

import (
	"context"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	argomonitor "github.com/simjay/plous/internal/integrations/argo/monitor"
	fluxmonitor "github.com/simjay/plous/internal/integrations/flux/monitor"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	textview "github.com/simjay/plous/internal/ui/text"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type overviewReturn struct {
	View   *overviewView
	Tool   string
	GitOps *gitOpsView
	Helm   *monitor.HelmState
}

type overviewView struct {
	State       monitor.OverviewState
	Cluster     *clusterState
	Cancel      context.CancelFunc
	Run         uint64
	Poll        bool
	Refresh     bool
	Pending     *monitor.OverviewTarget
	Inspecting  bool
	UsageCancel context.CancelFunc
	UsageRun    uint64
	UsagePoll   bool
	UsageNext   time.Time
}
type overviewResult struct {
	View  *overviewView
	Run   uint64
	Data  monitor.OverviewData
	Error string
}
type overviewPoll struct {
	View *overviewView
	Run  uint64
}
type overviewInspectResult struct {
	View   *overviewView
	Run    uint64
	Target monitor.OverviewTarget
	Object *unstructured.Unstructured
	Err    error
}

func (m *model) overviewActive() bool {
	c := m.selectedConnection()
	return m.mode == 0 && c != nil && monitorCluster(c.Tool)
}
func (m *model) overviewVisible() bool {
	return m.overviewActive() && m.page == "" && m.form == nil && m.detail == ""
}
func (m *model) overviewRead() *overviewView {
	if !m.overviewActive() {
		return nil
	}
	return m.overviews[clusterKey(m.active, *m.selectedConnection())]
}
func (m *model) overviewCurrent() *overviewView {
	if !m.overviewActive() {
		return nil
	}
	cluster := m.clusterFor(m.active, *m.savedConnection())
	if m.overviews == nil {
		m.overviews = map[string]*overviewView{}
	}
	v := m.overviews[cluster.Key]
	if v == nil || v.Cluster != cluster {
		if v != nil {
			v.stop()
		}
		v = &overviewView{Cluster: cluster}
		m.overviews[cluster.Key] = v
	}
	return v
}
func (v *overviewView) stop() {
	v.stopInventory()
	if v.UsageCancel != nil {
		v.UsageCancel()
		v.UsageCancel = nil
	}
	v.UsageRun++
	v.UsagePoll = false
	v.UsageNext = time.Time{}
	v.Inspecting = false
}
func (v *overviewView) stopInventory() {
	if v.Cancel != nil {
		v.Cancel()
		v.Cancel = nil
		v.State.Loading = false
	}
	v.Run++
	v.Poll = false
}
func (m *model) syncOverview() tea.Cmd {
	if back := m.overviewBack; back != nil && (m.mode != 1 || m.active != back.View.Cluster.Workspace || m.connection != back.View.Cluster.Connection.ID) {
		m.overviewBack = nil
	}
	var current *overviewView
	if !m.exploreClosed && !m.quitting && m.overviewVisible() {
		current = m.overviewCurrent()
	}
	for _, v := range m.overviews {
		if v != current && (v.Cancel != nil || v.Poll || v.UsageCancel != nil || v.UsagePoll) {
			v.stop()
		}
	}
	if current == nil {
		return nil
	}
	v := current
	v.State.Resize(m.panelWidths()[1]-4, m.mainBodyHeight())
	if v.Pending != nil {
		target := *v.Pending
		v.Pending = nil
		return m.openOverviewTarget(v, target)
	}
	return tea.Batch(m.syncOverviewUsage(v), m.syncOverviewInventory(v))
}
func (m *model) syncOverviewInventory(v *overviewView) tea.Cmd {
	if v.Cancel != nil {
		return nil
	}
	if !v.Cluster.Loaded {
		return nil
	}
	if (!v.State.Loaded || v.Refresh) && v.State.Error == "" {
		if v.Cluster.Err != nil || v.Cluster.Client == nil {
			v.State.Retained = v.State.Loaded
			v.State.Error = "Cluster discovery unavailable. g Refresh to retry."
			if v.Cluster.Err != nil {
				v.State.Error = kubernetes.ErrorText(v.Cluster.Err)
			}
			return nil
		}
		if !v.Cluster.HelmChecked {
			return nil
		}
		return m.readOverview(v)
	}
	if !v.Poll {
		v.Poll = true
		run := v.Run
		return tea.Tick(15*time.Second, func(time.Time) tea.Msg { return overviewPoll{v, run} })
	}
	return nil
}
func (m *model) readOverview(v *overviewView) tea.Cmd {
	v.stopInventory()
	v.State.Loading = true
	v.Refresh = false
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	v.Cancel = cancel
	run, client, catalog, warning := v.Run, v.Cluster.Client, v.Cluster.Catalog, v.Cluster.Warning
	driver, helmError := v.Cluster.HelmDriver, v.Cluster.HelmErr
	return func() tea.Msg {
		defer cancel()
		data := monitor.OverviewData{}
		data.Notices = append(data.Notices, "Network: no telemetry provider configured")
		var wg sync.WaitGroup
		wg.Go(func() { data.Kubernetes = client.OverviewInventory(ctx, catalog) })
		if driver != "" {
			wg.Go(func() { h := helm.ReadOverview(ctx, client, driver); data.Helm = &h })
		} else if helmError != nil {
			data.Notices = append(data.Notices, "Helm discovery inaccessible: "+kubernetes.ErrorText(helmError))
		} else {
			data.Notices = append(data.Notices, "Helm: no release metadata detected in readable storage")
		}
		if argomonitor.Supports(catalog) {
			wg.Go(func() { a := argomonitor.New(client).LoadWithCatalog(ctx, "", catalog, warning); data.Argo = &a })
		} else {
			data.Notices = append(data.Notices, "Argo CD API not detected here; Applications may live in a management cluster")
		}
		if fluxmonitor.Supports(catalog) {
			data.Flux = make([]gitops.Snapshot, 3)
			for i, section := range []gitops.Section{gitops.Kustomizations, gitops.HelmReleases, gitops.Sources} {
				wg.Go(func() { data.Flux[i] = fluxmonitor.New(client, section).LoadWithCatalog(ctx, "", catalog, warning) })
			}
		} else {
			data.Notices = append(data.Notices, "Flux APIs not detected in this cluster")
		}
		wg.Wait()
		if warning != "" {
			data.Notices = append(data.Notices, "Discovery partial: "+warning)
		}
		for _, key := range []string{"nodes", "pods", "deployments.apps", "statefulsets.apps", "daemonsets.apps", "horizontalpodautoscalers.autoscaling"} {
			r := data.Kubernetes.Reads[key]
			if r.Error != "" {
				data.Notices = append(data.Notices, key+": "+r.Error)
			} else if r.Partial {
				data.Notices = append(data.Notices, key+": partial inventory (read limit reached)")
			}
		}
		if data.Helm != nil && (data.Helm.Error != "" || data.Helm.Partial) {
			data.Notices = append(data.Notices, "Helm inventory partial or inaccessible: "+data.Helm.Error)
		}
		snapshots := append([]gitops.Snapshot{}, data.Flux...)
		if data.Argo != nil {
			snapshots = append(snapshots, *data.Argo)
		}
		for _, snapshot := range snapshots {
			for _, issue := range snapshot.Issues {
				data.Notices = append(data.Notices, issue.Kind+": "+issue.Message)
			}
			if snapshot.Truncated {
				data.Notices = append(data.Notices, "Controller inventory partial: object limit reached")
			}
		}
		failed := 0
		for _, r := range data.Kubernetes.Reads {
			if r.Error != "" {
				failed++
			}
		}
		failure := ""
		if failed == len(data.Kubernetes.Reads) {
			failure = "All Kubernetes inventory reads failed. See Snapshot Availability for read errors."
		}
		return overviewResult{v, run, data, failure}
	}
}
func (m *model) applyOverview(r overviewResult) {
	v := r.View
	if v == nil || v.Run != r.Run || v.Cancel == nil {
		return
	}
	v.Cancel = nil
	if r.Error != "" && v.State.Loaded {
		v.State.Error = r.Error + "\n" + strings.Join(r.Data.Notices, "\n")
		v.State.Loading = false
		v.State.Retained = true
		return
	}
	v.State.Apply(r.Data)
	v.State.Error = r.Error
}
func (m *model) overviewCommand(command string) bool {
	if !m.overviewVisible() || m.helpVisible() || m.settingsOverlay != nil {
		return false
	}
	v := m.overviewCurrent()
	switch command {
	case "refresh":
		v.stop()
		v.Refresh = true
		v.State.Error = ""
		v.Cluster.refresh()
	case "expandsection":
		if v.State.MetricDetail != nil {
			return true
		}
		v.State.Expanded = !v.State.Expanded
	default:
		return false
	}
	return true
}
func (m *model) overviewKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil || m.commandFocused() || !m.overviewVisible() {
		return false, nil
	}
	v := m.overviewCurrent()
	s := &v.State
	s.Resize(m.panelWidths()[1]-4, m.mainBodyHeight())
	key := msg.String()
	if d := s.MetricDetail; d != nil {
		r := d.Reader()
		if key == "esc" && m.shortcutPrefix != "" && !r.Searching {
			return false, nil
		}
		if r.Searching || key != "enter" && key != "g" {
			if handled, copied := r.Key(key, msg.Text); handled {
				m.shortcutPrefix = ""
				return true, m.copyWorkspaceText(copied)
			}
		}
		switch key {
		case "enter":
			if r.Query != "" {
				r.Key("n", "")
			} else if !r.Selecting {
				d.Sources = true
			}
		case "esc":
			if d.Sources {
				d.Sources = false
			} else {
				s.MetricDetail = nil
			}
		case "g":
			m.overviewCommand("refresh")
		default:
			return false, nil
		}
		m.shortcutPrefix = ""
		return true, nil
	}
	if s.Input != nil {
		switch key {
		case "esc":
			s.Input = nil
			s.Query = ""
		case "enter":
			s.Query = s.Input.Text
			s.Input = nil
			if !s.OpenGroup() && !s.OpenInventory() {
				v.Pending = s.Selected()
			}
		case "up", "down", "pgup", "pgdown":
			s.Move(key)
		default:
			s.Input.Key(msg, false)
			s.Cursor = [4]int{}
		}
		m.shortcutPrefix = ""
		return true, nil
	}
	if key == "esc" && m.shortcutPrefix != "" {
		return false, nil
	}
	switch key {
	case "/", "e", "g", "enter", "left", "right", "l", "up", "down", "j", "k", "pgup", "pgdown", "home", "end":
		m.shortcutPrefix = ""
		m.status = ""
	}
	switch key {
	case "/":
		field := textview.NewField(s.Query)
		s.Input = &field
		if s.Pane == 0 {
			s.Pane = 1
		}
		return true, nil
	case "e":
		m.overviewCommand("expandsection")
		return true, nil
	case "g":
		m.overviewCommand("refresh")
		return true, nil
	case "enter":
		if !s.OpenMetric() {
			if !s.OpenGroup() && !s.OpenInventory() {
				v.Pending = s.Selected()
			}
		}
		return true, nil
	case "esc":
		if s.Query != "" {
			s.Query = ""
			return true, nil
		}
		if s.CloseGroup() {
			return true, nil
		}
		if s.CloseInventory() {
			return true, nil
		}
		if v.Inspecting {
			v.stop()
			v.Pending = nil
			return true, nil
		}
		if s.Expanded {
			s.Expanded = false
			return true, nil
		}
	case "left", "right", "l", "up", "down", "j", "k", "pgup", "pgdown", "home", "end":
		s.Move(key)
		return true, nil
	}
	return false, nil
}
func (m *model) overviewBody(width, height int) string {
	v := m.overviewRead()
	if v == nil {
		return (&monitor.OverviewState{}).Render(m.selectedConnection().Name, width, height, m.focus == 1, time.Now())
	}
	return v.State.Render(m.selectedConnection().Name, width, height, m.focus == 1, time.Now())
}

func (m *model) openOverviewTarget(v *overviewView, target monitor.OverviewTarget) tea.Cmd {
	if target.Evidence != "" {
		m.detail = target.Evidence
		m.cursor = 0
		return nil
	}
	if m.width < 80 && target.Name != "" {
		m.detail = v.State.SelectedEvidence()
		m.cursor = 0
		return nil
	}
	if target.Tool == "" {
		m.detail = v.State.Evidence()
		m.cursor = 0
		return nil
	}
	back := &overviewReturn{View: v, Tool: m.tool}
	if target.Tool == "helm" {
		m.mode, m.tool = 1, "helm"
		original := m.helmer()
		h := &monitor.HelmState{Temporary: true, Key: original.Key + "/overview", Workspace: original.Workspace, Connection: original.Connection, Config: original.Config, Preference: monitor.HelmPreference{Section: "Releases", Namespace: "*"}}
		back.Helm, m.overviewBack = h, back
		m.helmStates[h.Key] = h
		if !m.chooseHelmSection("Releases") {
			m.mode = 0
			m.overviewBack = nil
			return nil
		}
		h.Preference.Namespace = "*"
		h.Preference.Filter = ""
		h.Loaded = false
		m.overviewBack = back
		if target.Name != "" {
			h.RequestedRelease = &helm.Release{Name: target.Name, Namespace: target.Namespace, Revision: target.Revision}
			h.JobRevision = target.Revision
			if h.Client != nil {
				h.Pending = "detail"
			}
		}
		return nil
	}
	if gitOpsTool(target.Tool) {
		m.mode, m.tool = 1, target.Tool
		section := gitops.NormalizeSection(gitops.Tool(target.Tool), "")
		if target.Name != "" {
			section = gitops.SectionFor(target.Ref)
		}
		m.stopGitOps()
		g := &gitOpsView{Temporary: true, Cluster: v.Cluster, State: monitor.NewGitOpsState(gitops.Tool(target.Tool), "")}
		g.State.Section = section
		back.GitOps = g
		if m.gitOpsViews == nil {
			m.gitOpsViews = map[string]*gitOpsView{}
		}
		m.gitOpsViews[v.Cluster.Key+"/overview/"+target.Tool] = g
		m.overviewBack = back
		if target.Name != "" {
			g.State.Pending = &monitor.GitOpsAction{Kind: "detail", Ref: target.Ref}
		}
		return nil
	}
	if v.Cluster.Client == nil || v.Cluster.Err != nil {
		m.status = "Cluster unavailable. g Refresh before inspecting this resource."
		return nil
	}
	if target.Name == "" {
		m.mode, m.tool = 1, "kubernetes"
		m.stopExplore()
		m.exploreGeneration++
		m.explore = &monitor.ExploreState{Temporary: true, Key: v.Cluster.Key, Generation: m.exploreGeneration, Client: v.Cluster.Client, Resources: v.Cluster.Catalog, Resource: target.Resource, Preference: monitor.ExplorePreference{Resource: target.Resource.Key()}, PreserveTree: true}
		m.overviewBack = back
		return nil
	}
	v.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	v.Cancel = cancel
	v.State.Loading = true
	v.Inspecting = true
	run, client := v.Run, v.Cluster.Client
	return func() tea.Msg {
		defer cancel()
		object, err := client.Get(ctx, target.Resource, target.Namespace, target.Name)
		return overviewInspectResult{v, run, target, object, err}
	}
}
func (m *model) applyOverviewInspect(r overviewInspectResult) {
	v := r.View
	if v == nil || v.Run != r.Run || v.Cancel == nil {
		return
	}
	v.Cancel = nil
	v.State.Loading = false
	v.Inspecting = false
	if !m.overviewVisible() || m.overviewCurrent() != v || m.focus != 1 {
		return
	}
	if r.Err != nil {
		m.status = kubernetes.ErrorText(r.Err)
		return
	}
	if r.Object == nil {
		m.status = "Resource unavailable. Refresh Overview."
		return
	}
	if r.Target.UID != "" && string(r.Object.GetUID()) != r.Target.UID {
		m.status = "Resource was replaced. Refresh Overview before inspecting it."
		return
	}
	m.overviewBack = &overviewReturn{View: v, Tool: m.tool}
	m.mode, m.tool = 1, "kubernetes"
	m.stopExplore()
	m.exploreGeneration++
	m.explore = &monitor.ExploreState{Temporary: true, Key: v.Cluster.Key, Generation: m.exploreGeneration, Client: v.Cluster.Client, Resources: v.Cluster.Catalog, Resource: r.Target.Resource, Preference: monitor.ExplorePreference{Resource: r.Target.Resource.Key(), Namespace: r.Target.Namespace}, Selected: kubernetes.Identity(*r.Object), Detail: r.Object, DetailValid: true, Container: monitor.PreferredPodContainer(*r.Object), PreserveTree: true}
}

func (m *model) returnToOverview() bool {
	back := m.overviewBack
	if back == nil || m.active != back.View.Cluster.Workspace || m.connection != back.View.Cluster.Connection.ID {
		return false
	}
	m.stopExplore()
	if back.GitOps != nil && back.GitOps.Cancel != nil {
		back.GitOps.Cancel()
		back.GitOps.Cancel = nil
		back.GitOps.Run++
	}
	if back.Helm != nil {
		back.Helm.Stop()
	}
	m.mode, m.tool, m.focus = 0, back.Tool, 1
	m.page, m.detail, m.status = "", "", ""
	m.overviewBack = nil
	return true
}
