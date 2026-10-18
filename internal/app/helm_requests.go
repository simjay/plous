package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
	diffview "github.com/simjay/plous/internal/ui/diff"
	"github.com/simjay/plous/internal/workspace"
)

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
			if h.Namespaces != nil && h.Namespaces.Run != nil {
				h.Namespaces.Run.Cancel()
				h.Namespaces.Run = nil
			}
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
