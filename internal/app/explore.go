package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func (m *model) explorer() *monitor.ExploreState {
	if m.explore == nil {
		m.explore = &monitor.ExploreState{}
	}
	if m.explorePreferences == nil {
		m.explorePreferences = map[string]monitor.ExplorePreference{}
	}
	return m.explore
}

func (m *model) exploreActive() bool {
	c := m.connectionView(m.selectedConnection())
	return m.mode == 1 && c != nil && c.Tool == "kubernetes"
}

func (m *model) exploreVisible() bool {
	return m.explore != nil && m.mode == 1 && m.page == "" && m.form == nil && (m.exploreActive() || (m.explore != nil && m.explore.Picker == "connections"))
}

func (m *model) rememberExplore() {
	if m.gitOpsReturn != nil || m.overviewBack != nil {
		return
	}
	e := m.explorer()
	if e.Temporary {
		return
	}
	if c := m.currentConnection(); c != nil && c.Tool == "kubernetes" {
		m.explorePreferences[m.active+"/"+m.connection] = e.Preference
	}
	m.persist()
}

func (m *model) stopExplore() { m.explore.Stop() }

func (m *model) syncExplore() tea.Cmd {
	if m.exploreClosed {
		return nil
	}
	if !m.exploreActive() {
		if m.explore != nil {
			m.stopExplore()
			m.explore.Key = ""
			if m.explore.Picker != "connections" {
				m.explore.PendingCommand = ""
			}
		}
		return nil
	}
	e := m.explorer()
	c := m.currentConnection()
	key := strings.Join([]string{m.active, c.ID, c.Settings["kubeconfig"], c.Settings["context"], c.Settings["namespace"]}, "\x00")
	if key != e.Key {
		request, pending := e.Requested, e.PendingCommand
		m.stopExplore()
		m.exploreGeneration++
		preference, restored := m.explorePreferences[m.active+"/"+c.ID]
		if !restored {
			preference = monitor.ExplorePreference{Resource: "pods", Namespace: c.Settings["namespace"]}
		}
		if request != "" {
			preference.Resource = request
		}
		if preference.Resource == "" {
			preference.Resource = "pods"
		}
		*e = monitor.ExploreState{Key: key, Generation: m.exploreGeneration, Preference: preference, Connecting: true, Status: "Connecting…", DefaultNamespace: !restored && preference.Namespace == ""}
		e.PreserveTree = request == ""
		e.PendingCommand = pending
	}
	if e.Connecting {
		cluster := m.clusterFor(m.active, *m.savedConnection())
		if !cluster.Loaded {
			if cluster.Cancel == nil {
				return m.discoverCluster(cluster)
			}
			return nil
		}
		m.updateExploreMessage(monitor.ExploreConnected{Generation: e.Generation, Client: cluster.Client, Resources: cluster.Catalog, Warning: cluster.Warning, Err: cluster.Err})
	}
	namespaceCommand := m.syncNamespacePicker()
	if e.Connecting || e.Client == nil {
		return namespaceCommand
	}
	resource, ok := kubernetes.Find(e.Resources, e.Preference.Resource)
	if !ok {
		m.stopExploreUsage()
		e.Err = "Resource type unavailable or ambiguous. Choose a resource in the sidebar, or use its API-qualified name."
		return namespaceCommand
	}
	e.Resource = resource
	ns := e.Preference.Namespace
	if !resource.Namespaced {
		ns = ""
	}
	streamKey := resource.Key() + "/" + resource.Version + "/" + ns + "/" + e.Preference.Selector
	commands := []tea.Cmd{namespaceCommand}
	if e.Stream == nil || e.Stream.Key != streamKey {
		if e.Stream != nil {
			e.Stream.Cancel()
		}
		ctx, cancel := context.WithCancel(context.Background())
		stream := &monitor.ExploreStream{Key: streamKey, Cancel: cancel, Ctx: ctx, Events: make(chan kubernetes.Snapshot, 4)}
		e.Stream = stream
		if !e.Preloaded {
			e.Collection = kubernetes.Collection{}
			e.Listed = false
		}
		e.Preloaded = false
		e.Status = "Loading…"
		e.Err = ""
		client, selector := e.Client, e.Preference.Selector
		commands = append(commands, func() tea.Msg {
			go func() {
				defer close(stream.Events)
				client.Observe(ctx, resource, ns, selector, func(snapshot kubernetes.Snapshot) {
					select {
					case stream.Events <- snapshot:
					case <-ctx.Done():
					}
				})
			}()
			return monitor.WaitExplore(stream)()
		})
	}
	commands = append(commands, m.syncExploreDetail(), m.syncExploreUsage())
	return tea.Batch(commands...)
}

func (m *model) updateExploreMessage(msg tea.Msg) tea.Cmd {
	e := m.explorer()
	switch msg := msg.(type) {
	case monitor.ExploreUsageResult:
		return m.updateExploreUsage(msg)
	case monitor.NamespaceResult:
		m.updateNamespaceResult(msg)
	case monitor.ExploreConnected:
		if msg.Generation != e.Generation || !e.Connecting || !m.exploreActive() {
			return nil
		}
		e.Connecting = false
		if msg.Err != nil {
			if msg.Client == nil {
				e.Err = msg.Err.Error()
			} else {
				e.Err = kubernetes.ErrorText(msg.Err)
			}
			e.Status = "Connection unavailable"
			return nil
		}
		e.Client, e.Resources, e.Warning = msg.Client, msg.Resources, msg.Warning
		if e.DefaultNamespace {
			e.Preference.Namespace = msg.Client.Namespace
			e.DefaultNamespace = false
		}
		e.Err = ""
		m.rememberExplore()
		if command := e.PendingCommand; command != "" {
			e.PendingCommand = ""
			m.runExploreCommand(command, false)
		}
	case monitor.ExploreUpdate:
		if msg.Stream != e.Stream || msg.Done {
			return nil
		}
		e.Status = msg.Snapshot.Status
		if msg.Snapshot.Err != nil {
			e.Err = kubernetes.ErrorText(msg.Snapshot.Err)
		}
		if msg.Snapshot.Collection != nil {
			e.Collection = *msg.Snapshot.Collection
			e.Inventory = nil
			e.Listed = true
			e.Err = ""
			m.reconcileExploreSelection()
			if e.Detail != nil {
				for _, object := range e.Collection.Objects {
					if object.GetUID() == e.Detail.GetUID() {
						e.Detail = object.DeepCopy()
						if e.Resource.Key() == "services" {
							if readiness, found := e.Collection.ServiceEndpoints[kubernetes.Identity(object)]; found && (e.SupplementAt.IsZero() || readiness.ObservedAt.After(e.SupplementAt)) && (e.DetailSections[0].At.IsZero() || readiness.ObservedAt.After(e.DetailSections[0].At)) {
								e.Supplement, e.SupplementNote = kubernetes.EndpointReadinessSummary(readiness)
								e.SupplementAt = readiness.ObservedAt
								e.DetailSections[0].Supplement, e.DetailSections[0].Note, e.DetailSections[0].At = e.Supplement, e.SupplementNote, e.SupplementAt
							}
						}
						break
					}
				}
			}
		}
		return monitor.WaitExplore(e.Stream)
	case monitor.ExploreDetailResult:
		if msg.Run != e.DetailRun {
			return nil
		}
		e.DetailRun = nil
		e.DetailValid = msg.Object != nil && msg.Err == nil
		e.DetailError = kubernetes.ErrorText(msg.Err)
		e.DetailText = msg.Text
		if e.Resource.Key() != "services" || !e.SupplementAt.After(msg.SupplementAt) {
			e.Supplement, e.SupplementNote = msg.Supplement, msg.Note
			e.SupplementAt = msg.SupplementAt
		}
		e.References = monitor.SortedReferences(msg.References)
		e.DetailLoaded = msg.Err == nil && msg.Object != nil
		if e.Tab < 4 {
			e.DetailSections[e.Tab] = monitor.DetailSection{Text: e.DetailText, Note: e.SupplementNote, References: e.References, Supplement: e.Supplement, At: e.SupplementAt, Loaded: e.DetailLoaded}
		}
		if msg.Object != nil {
			e.Detail = msg.Object
			if !e.MergedLogs && e.Resource.Key() == "pods" && !slices.Contains(kubernetes.Containers(*msg.Object), e.Container) {
				e.Container = monitor.PreferredPodContainer(*msg.Object)
			}
		}
	case monitor.ExploreLogEvent:
		if e.Log == nil || msg.Run != e.Log {
			return nil
		}
		if msg.WorkloadStatus != nil {
			e.WorkloadLogStatus = *msg.WorkloadStatus
		}
		if msg.Connected {
			e.LogConnected = true
		}
		if msg.Err != nil {
			e.LogError = kubernetes.LogErrorText(msg.Err, e.Previous)
		}
		m.appendExploreLogs(msg.Text)
		if msg.Done {
			e.Log.Cancel()
			e.Log = nil
			return nil
		}
		return monitor.WaitExploreLog(e.Log)
	}
	return nil
}

func (m *model) syncExploreDetail() tea.Cmd {
	e := m.explorer()
	if e.Detail == nil || m.mode != 1 {
		return nil
	}
	object := e.Detail.DeepCopy()
	resource, client := e.Resource, e.Client
	key := resource.Key() + "/" + resource.Version + "/" + kubernetes.Identity(*object) + "/" + fmt.Sprint(e.Tab)
	var commands []tea.Cmd
	if e.DetailKey != key {
		if e.DetailRun != nil {
			e.DetailRun.Cancel()
		}
		ctx, cancel := context.WithCancel(context.Background())
		run := &monitor.ExploreDetailRun{Cancel: cancel}
		e.DetailRun = run
		identity := resource.Key() + "/" + resource.Version + "/" + kubernetes.Identity(*object) + "/" + string(object.GetUID())
		if e.DetailIdentity != identity {
			e.DetailSections = [4]monitor.DetailSection{}
			e.DetailIdentity = identity
		}
		section := monitor.DetailSection{}
		if e.Tab < 4 {
			section = e.DetailSections[e.Tab]
		}
		e.DetailValid, e.DetailLoaded = section.Loaded, section.Loaded
		e.DetailKey, e.DetailError = key, ""
		e.DetailText, e.References = section.Text, section.References
		e.Supplement, e.SupplementNote, e.SupplementAt = section.Supplement, section.Note, section.At
		tab, resources := e.Tab, slices.Clone(e.Resources)
		commands = append(commands, func() tea.Msg {
			defer cancel()
			current, err := client.Get(ctx, resource, object.GetNamespace(), object.GetName())
			result := monitor.ExploreDetailResult{Run: run, Object: current, Err: err}
			if err != nil {
				return result
			}
			if object.GetUID() != "" && current.GetUID() != object.GetUID() {
				result.Object = nil
				result.Text = "This object was deleted and replaced. Go back and select the replacement."
				return result
			}
			if tab == 2 {
				result.Text, result.Err = client.Events(ctx, *current)
			}
			if tab == 0 {
				result.Supplement, result.Note = client.SummarySupplement(ctx, resource, *current, resources)
				result.SupplementAt = time.Now()
			}
			if tab == 3 {
				result.References, result.Text = client.Related(ctx, resource, *current, resources)
			}
			return result
		})
	}
	if e.Tab != 4 {
		if e.Log != nil {
			e.Log.Cancel()
			e.Log = nil
		}
		e.LogKey = ""
		return tea.Batch(commands...)
	}
	if !e.DetailValid {
		return tea.Batch(commands...)
	}
	containers := kubernetes.Containers(*object)
	if !e.MergedLogs && !slices.Contains(containers, e.Container) {
		e.Container = monitor.PreferredPodContainer(*object)
	}
	logKey := key + "/" + e.Container + fmt.Sprint(e.Previous, e.LogSince)
	if e.Container == "" && !e.MergedLogs {
		if e.Log != nil {
			e.Log.Cancel()
			e.Log = nil
		}
		return tea.Batch(commands...)
	}
	if e.LogKey != logKey {
		if e.Log != nil {
			e.Log.Cancel()
		}
		ctx, cancel := context.WithCancel(context.Background())
		run := &monitor.ExploreLogRun{Cancel: cancel, Ctx: ctx, Events: make(chan monitor.ExploreLogEvent, 32)}
		e.Log = run
		e.LogKey = logKey
		m.resetExploreLogs()
		options := kubernetes.LogOptions{Namespace: object.GetNamespace(), Pod: object.GetName(), UID: object.GetUID(), Container: e.Container, Previous: e.Previous, Follow: !e.Previous}
		timestamps := true
		options.Timestamps = &timestamps
		if e.LogSince > 0 {
			since := e.LogSince
			options.SinceSeconds = &since
		}
		merged, resources := e.MergedLogs, slices.Clone(e.Resources)
		commands = append(commands, func() tea.Msg {
			go func() {
				defer close(run.Events)
				send := func(event monitor.ExploreLogEvent) {
					select {
					case run.Events <- event:
					case <-ctx.Done():
					}
				}
				options.OnConnect = func() { send(monitor.ExploreLogEvent{Run: run, Connected: true}) }
				var err error
				if merged {
					err = client.WorkloadLogs(ctx, resource, object, resources, options, func(event kubernetes.WorkloadLogEvent) {
						send(monitor.ExploreLogEvent{Run: run, Text: event.Text, WorkloadStatus: event.Status})
					})
				} else {
					err = client.Logs(ctx, options, func(text string) { send(monitor.ExploreLogEvent{Run: run, Text: text}) })
				}
				send(monitor.ExploreLogEvent{Run: run, Err: err, Done: true})
			}()
			return monitor.WaitExploreLog(run)()
		})
	}
	return tea.Batch(commands...)
}

func (m *model) reconcileExploreSelection() {
	e := m.explorer()
	_, rows := m.exploreRows()
	for _, row := range rows {
		if kubernetes.Identity(row.Object) == e.Selected {
			return
		}
	}
	e.Selected = ""
	if len(rows) > 0 {
		e.Selected = kubernetes.Identity(rows[0].Object)
	}
}
