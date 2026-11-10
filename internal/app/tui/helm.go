package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
	diffview "github.com/simjay/plous/internal/ui/diff"
	"github.com/simjay/plous/internal/workspace"
)

func (m *model) helmActive() bool {
	c := m.connectionView(m.selectedConnection())
	return m.mode == 1 && c != nil && c.Tool == "helm"
}

func (m *model) helmVisible() bool { return m.helmActive() && m.page == "" && m.form == nil }

func (m *model) helmer() *monitor.HelmState {
	c := m.currentConnection()
	if c == nil || c.Tool != "helm" {
		return nil
	}
	if back := m.overviewBack; back != nil && back.Helm != nil && back.Helm.Connection == c.ID && back.Helm.Workspace == m.active {
		return back.Helm
	}
	config := monitor.HelmConfig(c)
	key := m.active + "/" + c.ID + "/" + config.Fingerprint()
	if m.helmStates == nil {
		m.helmStates = map[string]*monitor.HelmState{}
	}
	if m.helmPreferences == nil {
		m.helmPreferences = map[string]monitor.HelmPreference{}
	}
	h := m.helmStates[key]
	if h == nil {
		p := m.helmPreferences[m.active+"/"+c.ID]
		if !monitor.ContainsString(monitor.HelmSections, p.Section) {
			p.Section = "Releases"
		}
		if p.Namespace == "" {
			p.Namespace = config.Namespace
		}
		h = &monitor.HelmState{Key: key, Workspace: m.active, Connection: c.ID, Config: config, Preference: p}
		m.helmStates[key] = h
	}
	return h
}

func (m *model) rememberHelm(h *monitor.HelmState) {
	if h.Temporary || m.overviewBack != nil {
		return
	}
	m.helmPreferences[h.Workspace+"/"+h.Connection] = h.Preference
	m.persist()
}

func (m *model) chooseHelmSection(section string) bool {
	h := m.helmer()
	if h == nil || !monitor.ContainsString(monitor.HelmSections, section) {
		return false
	}
	if h.Document != nil && !h.Document.ReadOnly {
		m.status = "Close or save the values editor before switching Helm sections."
		return false
	}
	if h.Busy {
		if h.Running == "execute" {
			m.status = "Wait for the Helm operation before changing this view."
			return false
		}
		if h.Cancel != nil {
			h.Cancel()
		}
		h.Run++
		h.Busy = false
		h.Cancel = nil
	}
	h.Pending = ""
	if h.Workflow != nil {
		h.Workflow.Active = false
	}
	h.SavePosition()
	h.Preference.Section = section
	h.Subcursor = 0
	h.Cursor = h.Positions[section]
	h.StopTimelineLoad()
	h.Timeline = nil
	h.Find = nil
	h.Detail = nil
	h.Menu = nil
	h.Document = nil
	h.Comparison = nil
	h.Confirm = false
	h.Input = nil
	h.StopNamespacePicker()
	h.Err = ""
	m.rememberHelm(h)
	return true
}

func (m *model) stopHelm() {
	for _, h := range m.helmStates {
		h.Stop()
	}
}

func (m *model) syncHelm() tea.Cmd {
	if m.exploreClosed {
		return nil
	}
	if m.helmVisible() {
		h := m.helmer()
		if h.Client == nil && !h.Busy && h.Pending == "" && h.Err == "" {
			h.Pending = "connect"
		}
		if h.Client != nil && !h.Loaded && !h.Busy && !h.WorkflowVisible() && h.Pending == "" && h.Err == "" && h.Input == nil && (h.Preference.Section == "Releases" || h.Preference.Section == "Charts") {
			h.Pending = "list"
		}
	}
	var cmds []tea.Cmd
	for _, h := range m.helmStates {
		if m.helmVisible() && h == m.helmer() {
			cmds = append(cmds, m.syncHelmNamespaces(h))
			cmds = append(cmds, h.SyncTimeline())
		} else {
			h.StopTimelineLoad()
			h.Namespaces.Cancel()
		}
		if h.Pending != "" && !h.Busy {
			cmds = append(cmds, m.startHelmJob(h))
		}
	}
	return tea.Batch(cmds...)
}

func (m *model) startHelmJob(h *monitor.HelmState) tea.Cmd {
	kind := h.Pending
	h.Pending = ""
	h.Running = kind
	h.Busy = true
	h.Err = ""
	h.Run++
	run := h.Run
	timeout := 90 * time.Second
	if kind == "execute" {
		timeout = 6 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	h.Cancel = cancel
	config, client, ns := h.Config, h.Client, h.Preference.Namespace
	if ns == "" && client != nil {
		ns = client.Config.Namespace
	}
	var selected helmtool.Release
	if h.Detail != nil {
		selected = h.Detail.Release()
	} else if h.Timeline != nil {
		selected = h.Timeline.Release
	} else {
		rows := h.ReleaseRows()
		if len(rows) > 0 {
			selected = rows[min(h.Cursor, len(rows)-1)]
		}
	}
	if kind == "detail" && h.RequestedRelease != nil {
		selected = *h.RequestedRelease
		h.RequestedRelease = nil
	}
	revision := h.JobRevision
	if revision == 0 {
		revision = selected.Revision
	}
	h.JobRevision = 0
	plan := h.Plan
	var draft monitor.HelmDraft
	var fields map[string]string
	if h.Draft != nil {
		draft = *h.Draft
		fields = map[string]string{}
		for _, f := range draft.Form.Fields {
			fields[f.Key] = strings.TrimSpace(f.Value.Text)
		}
	}
	resourceIndex := h.Subcursor
	var resources []monitor.HelmInspectorResource
	var resourcesError error
	liveReveal := h.LiveReveal
	h.LiveReveal = false
	if h.Detail != nil {
		resources, resourcesError = h.InspectorResources()
	}
	store := m.store
	workspaceID, connectionID := h.Workspace, h.Connection
	return func() tea.Msg {
		defer cancel()
		r := monitor.HelmResult{State: h, Run: run, Kind: kind}
		switch kind {
		case "connect":
			r.Client, r.Err = helmtool.New(config)
		case "list":
			r.Releases, r.Err = client.List(ctx, ns)
		case "timeline":
			r.Snapshot = &helmtool.Snapshot{Name: selected.Name, Namespace: selected.Namespace}
			r.Snapshot.History, r.Err = client.History(ctx, selected.Name, selected.Namespace)
			if r.Err == nil && store != nil {
				if err := monitor.RecordHelmHistory(ctx, store, workspaceID, connectionID, config, r.Snapshot); err != nil {
					r.HistoryError = helmtool.SafeText(err.Error())
				}
			}
		case "detail":
			if revision == 0 {
				metadata, err := client.Metadata(ctx, selected.Name, selected.Namespace, 0)
				if err != nil {
					r.Err = err
					break
				}
				revision = metadata.Revision
			}
			r.Snapshot, r.Err = client.Snapshot(ctx, selected.Name, selected.Namespace, revision)
			if r.Err == nil && store != nil {
				if err := monitor.RecordHelmHistory(ctx, store, workspaceID, connectionID, config, r.Snapshot); err != nil {
					r.HistoryError = helmtool.SafeText(err.Error())
				}
			}
		case "prepare":
			a := draft.Artifact
			if a == nil || a.Reference != fields["chart"] || a.Repository != fields["repository"] || a.Version != fields["version"] {
				a, r.Err = client.Acquire(ctx, fields["chart"], fields["repository"], fields["version"])
			}
			if r.Err == nil {
				r.Plan, r.Err = client.PrepareChart(ctx, a, fields["release"], fields["namespace"], draft.Values, draft.Baseline)
				r.Artifact = a
			}
		case "rollback":
			r.Plan, r.Err = client.PrepareRollback(ctx, selected.Name, selected.Namespace, revision)
		case "live":
			if resourcesError != nil {
				r.Err = resourcesError
				break
			}
			if resourceIndex >= len(resources) {
				r.Err = fmt.Errorf("resource selection changed")
				break
			}
			ref := resources[resourceIndex]
			if ref.Name == "" {
				r.Err = fmt.Errorf("the recorded manifest has no fixed resource name to look up")
				break
			}
			var kube *kubernetes.Client
			kube, r.Err = kubernetes.NewTarget(config.Target, config.Namespace)
			if r.Err != nil {
				break
			}
			catalog, _, e := kube.Discover(ctx)
			if e != nil {
				r.Err = e
				break
			}
			found := false
			for _, resource := range catalog {
				gv := resource.Version
				if resource.Group != "" {
					gv = resource.Group + "/" + resource.Version
				}
				if resource.Kind == ref.Kind && gv == ref.APIVersion {
					found = true
					namespace := ref.Namespace
					if resource.Namespaced && namespace == "" {
						namespace = selected.Namespace
					}
					obj, e := kube.Get(ctx, resource, namespace, ref.Name)
					r.Err = e
					if e == nil {
						r.Live = obj.Object
						r.LiveReveal = liveReveal
					}
					break
				}
			}
			if !found {
				r.Err = fmt.Errorf("resource API is unavailable in this context")
			}
		case "execute":
			op := workspace.Operation{WorkspaceID: workspaceID, ConnectionID: connectionID, ConnectionFingerprint: config.Fingerprint(), Tool: "helm", Context: plan.Context, Namespace: plan.Namespace, Release: plan.Name, Kind: plan.Operation, Status: "submitted", Revision: plan.TargetRevision}
			if plan.Artifact != nil {
				op.Chart = fmt.Sprint(plan.Artifact.Metadata["name"])
				op.Version = plan.Artifact.Version
			}
			if store == nil {
				r.Err = fmt.Errorf("Activity storage is unavailable")
				break
			}
			op, r.Err = store.RecordOperation(op)
			if r.Err != nil {
				break
			}
			r.Outcome, r.Err = client.Execute(ctx, plan)
			op.Status = r.Outcome
			op.ObservedAt = time.Now().UTC()
			if _, e := store.RecordOperation(op); e != nil {
				r.Err = fmt.Errorf("%s; outcome could not be recorded: %s", r.Outcome, e)
			}
		}
		return r
	}
}

func (m *model) updateHelmResult(r monitor.HelmResult) {
	h := r.State
	if r.Kind == "revision-diff" {
		h.ApplyTimelineChanges(r)
		return
	}
	if h == nil || h.Run != r.Run {
		return
	}
	h.Busy = false
	h.Running = ""
	h.Cancel = nil
	if r.Err != nil {
		h.Err = helmtool.SafeText(r.Err.Error())
		if r.Kind == "list" && len(r.Releases) > 0 {
			h.Releases = r.Releases
			h.Loaded = true
			h.Cursor = min(h.Cursor, max(0, h.RowsCount()-1))
			h.Notice = "Incomplete inventory. Showing releases received before the error."
		}
		if r.Outcome != "" {
			h.Notice = "Helm " + r.Outcome + ". Check the release before retrying."
		}
		if r.Outcome == "unknown" {
			h.Err = "Helm result unknown. Inspect the release before retrying.\n" + h.Err
		}
		m.loadHistory()
		return
	}
	switch r.Kind {
	case "connect":
		h.Client = r.Client
		if h.RequestedRelease != nil {
			h.Pending = "detail"
		}
		if h.Preference.Namespace == "" {
			h.Preference.Namespace = r.Client.Config.Namespace
		}
		m.rememberHelm(h)
	case "list":
		if strings.HasPrefix(h.Notice, "Incomplete inventory.") {
			h.Notice = ""
		}
		key := h.CollectionSelectionKey()
		if h.Preference.Section == "Releases" || h.Preference.Section == "Charts" {
			if key == "" {
				key = h.Preference.Selected
			}
		}
		h.Releases = r.Releases
		h.Loaded = true
		h.RestoreCollectionSelection(key)
	case "timeline":
		h.OpenTimeline(r.Snapshot)
		if r.HistoryError != "" {
			h.Notice = "Could not save observed Helm history: " + r.HistoryError
		}
		m.loadHistory()
	case "detail":
		h.ApplyInspectorSnapshot(r.Snapshot)
		h.Notice = ""
		if r.HistoryError != "" {
			h.Notice = "Could not save observed Helm history: " + r.HistoryError
		}
		m.loadHistory()
	case "prepare":
		h.Subcursor = 0
		h.Plan = r.Plan
		h.Draft.Artifact = r.Artifact
		h.Notice = "Preview ready. No cluster changes made."
	case "rollback":
		h.BeginWorkflow()
		h.Subcursor = 0
		h.Plan = r.Plan
		h.Menu = nil
		h.Notice = "Rollback preview ready. No cluster changes made."
	case "live":
		resources, _ := h.InspectorResources()
		if h.Subcursor < len(resources) {
			recorded, live := manifestutil.MaskedComparison(resources[h.Subcursor].Object, r.Live)
			if r.LiveReveal {
				recorded, live = manifestutil.YAML(resources[h.Subcursor].Object, false), manifestutil.YAML(r.Live, false)
			}
			h.Comparison = diffview.New(recorded, live, h.Config.Context+" / "+resources[h.Subcursor].Key(), uint64(h.Detail.Version), false)
			h.Comparison.BeforeLabel = "Recorded revision"
			h.Comparison.AfterLabel = "Live object"
			h.Comparison.Notice = "Includes server defaults and controller changes. Sensitive fields masked."
			if r.LiveReveal {
				h.Comparison.Notice = "Revealed values. Includes server defaults and controller changes."
			}
		}
	case "execute":
		h.Confirm = false
		h.Notice = "Helm operation completed. Workload readiness is separate; refresh Resources to inspect live objects."
		h.Plan = nil
		h.Draft = nil
		h.Workflow = nil
		h.Loaded = false
		h.Preference.Section = "Releases"
		h.Preference.Selected = ""
		h.Pending = "list"
		m.rememberHelm(h)
		m.loadHistory()
	}
}

func (m *model) openHelmNamespacePicker(h *monitor.HelmState) {
	h.StopNamespacePicker()
	m.helmInput(h, "namespace", "")
	h.Namespaces = &monitor.NamespacePicker{}
	h.PickerCursor = 0
	if h.NamespaceScope() != "" {
		h.PickerCursor = 1
	}
	m.status = ""
}

func (m *model) syncHelmNamespaces(h *monitor.HelmState) tea.Cmd {
	config := h.Config
	load := h.Namespaces.Load(15*time.Second, func(ctx context.Context) (kubernetes.Collection, error) {
		client, err := kubernetes.NewTarget(config.Target, "")
		if err != nil {
			return kubernetes.Collection{}, err
		}
		return client.List(ctx, kubecommon.Resource{Name: "namespaces", Kind: "Namespace", Version: "v1"}, "", "")
	})
	if load == nil {
		return nil
	}
	return func() tea.Msg {
		return monitor.HelmNamespaceResult{State: h, NamespaceResult: load()}
	}
}

func (m *model) updateHelmNamespaceResult(r monitor.HelmNamespaceResult) {
	if r.State == nil || !r.State.Namespaces.Apply(r.NamespaceResult) {
		return
	}
	h := r.State
	h.PickerCursor = min(h.PickerCursor, len(monitor.NamespaceChoicesFor(h.Namespaces, h.Input, h.NamespaceScope()))-1)
}

func (m *model) chooseHelmNamespace(h *monitor.HelmState, value string) {
	h.StopNamespacePicker()
	if value == "" {
		value = "*"
	}
	h.Preference.Namespace = value
	h.Preference.Selected = ""
	h.Err, h.Notice = "", ""
	h.Loaded = false
	h.Releases, h.Positions = nil, nil
	h.Detail = nil
	h.StopTimelineLoad()
	h.Timeline = nil
	h.Find = nil
	h.Cursor, h.Subcursor = 0, 0
	h.Input, h.Menu = nil, nil
	m.status = ""
	m.rememberHelm(h)
}

func (m *model) updateHelmNamespaceInput(h *monitor.HelmState, msg tea.KeyPressMsg) {
	value, apply, close, status := monitor.UpdateNamespacePicker(h.Namespaces, &h.Input, &h.PickerCursor, h.NamespaceScope(), msg)
	m.status = status
	if apply {
		m.chooseHelmNamespace(h, value)
	} else if close {
		h.StopNamespacePicker()
		h.Input = nil
	}
}
