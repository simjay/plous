package app

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/kubernetes"
	"github.com/simjay/plous/internal/logview"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type explorePreference struct {
	Resource   string `json:"resource"`
	Namespace  string `json:"namespace"`
	Filter     string `json:"filter,omitempty"`
	Selector   string `json:"selector,omitempty"`
	Sort       int    `json:"sort,omitempty"`
	Descending bool   `json:"descending,omitempty"`
}
type detailSectionReturn struct {
	tab, scroll, relatedCursor int
}

type exploreLocation struct {
	yamlOrigin                 *detailSectionReturn
	preference                 explorePreference
	resource                   kubernetes.Resource
	selected                   string
	object                     *unstructured.Unstructured
	tab, scroll, relatedCursor int
	container                  string
}
type exploreState struct {
	key                                string
	generation                         uint64
	client                             *kubernetes.Client
	resources                          []kubernetes.Resource
	resource                           kubernetes.Resource
	preference                         explorePreference
	collection                         kubernetes.Collection
	listed                             bool
	selected                           string
	status, err, warning               string
	connecting                         bool
	defaultNamespace                   bool
	connectionCancel                   context.CancelFunc
	stream                             *exploreStream
	detail                             *unstructured.Unstructured
	detailIdentity                     string
	detailLoaded                       bool
	detailSections                     [4]detailSection
	detailValid                        bool
	supplement                         []kubernetes.DetailSection
	supplementNote                     string
	supplementAt                       time.Time
	detailRun                          *exploreDetailRun
	detailText, detailError, detailKey string
	references                         []kubernetes.Reference
	relatedCursor                      int
	yamlDocument                       *yamlDocument
	yamlOrigin                         *detailSectionReturn
	tab, scroll                        int
	trail                              []exploreLocation
	logOrigin                          *exploreLocation
	log                                *exploreLogRun
	logError, logKey, container        string
	logViewer                          *logview.Model
	previous, reveal                   bool
	picker                             string
	pickerCursor, group                int
	input                              *editor
	inputKind                          string
	requested                          string
	pendingCommand                     string
	namespaces                         *namespacePicker
	action                             *resourceActionState
	terminal                           *resourceTerminal
	logSince                           int64
	logTimestamps                      bool
	mergedLogs                         bool
	workloadLogStatus                  kubernetes.WorkloadLogStatus
	logConnected                       bool
}
type exploreConnected struct {
	generation uint64
	client     *kubernetes.Client
	resources  []kubernetes.Resource
	warning    string
	err        error
}
type exploreStream struct {
	key    string
	cancel context.CancelFunc
	ctx    context.Context
	events chan kubernetes.Snapshot
}
type exploreUpdate struct {
	stream   *exploreStream
	snapshot kubernetes.Snapshot
	done     bool
}
type detailSection struct {
	text, note string
	references []kubernetes.Reference
	supplement []kubernetes.DetailSection
	at         time.Time
	loaded     bool
}
type exploreDetailRun struct{ cancel context.CancelFunc }
type exploreDetailResult struct {
	run          *exploreDetailRun
	object       *unstructured.Unstructured
	text         string
	references   []kubernetes.Reference
	supplement   []kubernetes.DetailSection
	note         string
	supplementAt time.Time
	err          error
}
type exploreLogRun struct {
	cancel context.CancelFunc
	ctx    context.Context
	events chan exploreLogEvent
}
type exploreLogEvent struct {
	workloadStatus *kubernetes.WorkloadLogStatus
	run            *exploreLogRun
	text           string
	err            error
	done           bool
	connected      bool
}

func (m *model) explorer() *exploreState {
	if m.explore == nil {
		m.explore = &exploreState{}
	}
	if m.explorePreferences == nil {
		m.explorePreferences = map[string]explorePreference{}
	}
	return m.explore
}
func (m *model) exploreActive() bool {
	c := m.currentConnection()
	return m.mode == 1 && c != nil && c.Tool == "kubernetes"
}
func (m *model) exploreVisible() bool {
	return m.mode == 1 && m.page == "" && m.form == nil && (m.exploreActive() || (m.explore != nil && m.explore.picker == "connections"))
}
func (m *model) rememberExplore() {
	e := m.explorer()
	if c := m.currentConnection(); c != nil && c.Tool == "kubernetes" {
		m.explorePreferences[m.active+"/"+m.connection] = e.preference
	}
	m.persist()
}
func (m *model) stopExplore() {
	if e := m.explore; e != nil {
		m.stopResourceAction()
		m.stopResourceTerminal()
		m.stopNamespacePicker()
		if e.connectionCancel != nil {
			e.connectionCancel()
			e.connectionCancel = nil
		}
		if e.stream != nil {
			e.stream.cancel()
			e.stream = nil
		}
		if e.detailRun != nil {
			e.detailRun.cancel()
			e.detailRun = nil
		}
		if e.log != nil {
			e.log.cancel()
			e.log = nil
		}
		e.detailKey = ""
		e.logKey = ""
	}
}

func (m *model) syncExplore() tea.Cmd {
	if m.exploreClosed {
		return nil
	}
	if !m.exploreActive() {
		if m.explore != nil {
			m.stopExplore()
			m.explore.key = ""
			if m.explore.picker != "connections" {
				m.explore.pendingCommand = ""
			}
		}
		return nil
	}
	e := m.explorer()
	c := m.currentConnection()
	key := strings.Join([]string{m.active, c.ID, c.Settings["kubeconfig"], c.Settings["context"], c.Settings["namespace"]}, "\x00")
	if key != e.key {
		request, picker, pending := e.requested, e.picker, e.pendingCommand
		m.stopExplore()
		m.exploreGeneration++
		preference, restored := m.explorePreferences[m.active+"/"+c.ID]
		if !restored {
			preference = explorePreference{Resource: "pods", Namespace: c.Settings["namespace"]}
		}
		if request != "" {
			preference.Resource = request
		}
		if preference.Resource == "" {
			preference.Resource = "pods"
		}
		ctx, cancel := context.WithCancel(context.Background())
		*e = exploreState{key: key, generation: m.exploreGeneration, preference: preference, connecting: true, status: "Connecting…", connectionCancel: cancel, defaultNamespace: !restored && preference.Namespace == ""}
		e.pendingCommand = pending
		if picker == "kinds" {
			e.picker = "kinds"
			input := newEditor("")
			e.input = &input
			e.inputKind = "kinds"
		}
		paths, contextName, namespace, generation := c.Settings["kubeconfig"], c.Settings["context"], c.Settings["namespace"], e.generation
		return func() tea.Msg {
			client, err := kubernetes.New(paths, contextName, namespace)
			if err != nil {
				return exploreConnected{generation: generation, err: err}
			}
			resources, warning, err := client.Discover(ctx)
			if !restored && preference.Namespace == "" {
				namespace = client.Namespace
			} else {
				namespace = preference.Namespace
			}
			client.Namespace = namespace
			return exploreConnected{generation: generation, client: client, resources: resources, warning: warning, err: err}
		}
	}
	namespaceCommand := m.syncNamespacePicker()
	if e.connecting || e.client == nil {
		return namespaceCommand
	}
	resource, ok := kubernetes.Find(e.resources, e.preference.Resource)
	if !ok {
		e.err = "Resource type unavailable or ambiguous. Use :kinds and choose an API group."
		return namespaceCommand
	}
	e.resource = resource
	ns := e.preference.Namespace
	if !resource.Namespaced {
		ns = ""
	}
	streamKey := resource.Key() + "/" + resource.Version + "/" + ns + "/" + e.preference.Selector
	commands := []tea.Cmd{namespaceCommand}
	if e.stream == nil || e.stream.key != streamKey {
		if e.stream != nil {
			e.stream.cancel()
		}
		ctx, cancel := context.WithCancel(context.Background())
		stream := &exploreStream{key: streamKey, cancel: cancel, ctx: ctx, events: make(chan kubernetes.Snapshot, 4)}
		e.stream = stream
		e.collection = kubernetes.Collection{}
		e.listed = false
		e.status = "Loading…"
		e.err = ""
		client, selector := e.client, e.preference.Selector
		commands = append(commands, func() tea.Msg {
			go func() {
				defer close(stream.events)
				client.Observe(ctx, resource, ns, selector, func(snapshot kubernetes.Snapshot) {
					select {
					case stream.events <- snapshot:
					case <-ctx.Done():
					}
				})
			}()
			return waitExplore(stream)()
		})
	}
	commands = append(commands, m.syncExploreDetail())
	return tea.Batch(commands...)
}

func waitExplore(stream *exploreStream) tea.Cmd {
	return func() tea.Msg {
		select {
		case snapshot, ok := <-stream.events:
			return exploreUpdate{stream: stream, snapshot: snapshot, done: !ok}
		case <-stream.ctx.Done():
			return exploreUpdate{stream: stream, done: true}
		}
	}
}
func (m *model) updateExploreMessage(msg tea.Msg) tea.Cmd {
	e := m.explorer()
	switch msg := msg.(type) {
	case namespaceResult:
		m.updateNamespaceResult(msg)
	case exploreConnected:
		if msg.generation != e.generation || !e.connecting || !m.exploreActive() {
			return nil
		}
		e.connecting = false
		if msg.err != nil {
			if msg.client == nil {
				e.err = msg.err.Error()
			} else {
				e.err = kubernetes.ErrorText(msg.err)
			}
			e.status = "Connection unavailable"
			return nil
		}
		e.client, e.resources, e.warning = msg.client, msg.resources, msg.warning
		if e.defaultNamespace {
			e.preference.Namespace = msg.client.Namespace
			e.defaultNamespace = false
		}
		e.err = ""
		m.rememberExplore()
		if command := e.pendingCommand; command != "" {
			e.pendingCommand = ""
			m.runExploreCommand(command, false)
		}
	case exploreUpdate:
		if msg.stream != e.stream || msg.done {
			return nil
		}
		e.status = msg.snapshot.Status
		if msg.snapshot.Err != nil {
			e.err = kubernetes.ErrorText(msg.snapshot.Err)
		}
		if msg.snapshot.Collection != nil {
			e.collection = *msg.snapshot.Collection
			e.listed = true
			e.err = ""
			_, rows := m.exploreRows()
			found := false
			for _, row := range rows {
				if kubernetes.Identity(row.Object) == e.selected {
					found = true
				}
			}
			if !found {
				e.selected = ""
				if len(rows) > 0 {
					e.selected = kubernetes.Identity(rows[0].Object)
				}
			}
			if e.detail != nil {
				for _, object := range e.collection.Objects {
					if object.GetUID() == e.detail.GetUID() {
						e.detail = object.DeepCopy()
						if e.resource.Key() == "services" {
							if readiness, found := e.collection.ServiceEndpoints[kubernetes.Identity(object)]; found && (e.supplementAt.IsZero() || readiness.ObservedAt.After(e.supplementAt)) && (e.detailSections[0].at.IsZero() || readiness.ObservedAt.After(e.detailSections[0].at)) {
								e.supplement, e.supplementNote = kubernetes.EndpointReadinessSummary(readiness)
								e.supplementAt = readiness.ObservedAt
								e.detailSections[0].supplement, e.detailSections[0].note, e.detailSections[0].at = e.supplement, e.supplementNote, e.supplementAt
							}
						}
						break
					}
				}
			}
		}
		return waitExplore(e.stream)
	case exploreDetailResult:
		if msg.run != e.detailRun {
			return nil
		}
		e.detailRun = nil
		e.detailValid = msg.object != nil && msg.err == nil
		e.detailError = kubernetes.ErrorText(msg.err)
		e.detailText = msg.text
		if e.resource.Key() != "services" || !e.supplementAt.After(msg.supplementAt) {
			e.supplement, e.supplementNote = msg.supplement, msg.note
			e.supplementAt = msg.supplementAt
		}
		e.references = sortedReferences(msg.references)
		e.detailLoaded = msg.err == nil && msg.object != nil
		if e.tab < 4 {
			e.detailSections[e.tab] = detailSection{e.detailText, e.supplementNote, e.references, e.supplement, e.supplementAt, e.detailLoaded}
		}
		if msg.object != nil {
			e.detail = msg.object
			if !e.mergedLogs && e.resource.Key() == "pods" && !slices.Contains(kubernetes.Containers(*msg.object), e.container) {
				e.container = preferredPodContainer(*msg.object)
			}
		}
	case exploreLogEvent:
		if e.log == nil || msg.run != e.log {
			return nil
		}
		if msg.workloadStatus != nil {
			e.workloadLogStatus = *msg.workloadStatus
		}
		if msg.connected {
			e.logConnected = true
		}
		if msg.err != nil {
			e.logError = kubernetes.LogErrorText(msg.err, e.previous)
		}
		m.appendExploreLogs(msg.text)
		if msg.done {
			e.log.cancel()
			e.log = nil
			return nil
		}
		return waitExploreLog(e.log)
	}
	return nil
}

func (m *model) syncExploreDetail() tea.Cmd {
	e := m.explorer()
	if e.detail == nil || m.mode != 1 {
		return nil
	}
	object := e.detail.DeepCopy()
	resource, client := e.resource, e.client
	key := resource.Key() + "/" + resource.Version + "/" + kubernetes.Identity(*object) + "/" + fmt.Sprint(e.tab)
	var commands []tea.Cmd
	if e.detailKey != key {
		if e.detailRun != nil {
			e.detailRun.cancel()
		}
		ctx, cancel := context.WithCancel(context.Background())
		run := &exploreDetailRun{cancel: cancel}
		e.detailRun = run
		identity := resource.Key() + "/" + resource.Version + "/" + kubernetes.Identity(*object) + "/" + string(object.GetUID())
		if e.detailIdentity != identity {
			e.detailSections = [4]detailSection{}
			e.detailIdentity = identity
		}
		section := detailSection{}
		if e.tab < 4 {
			section = e.detailSections[e.tab]
		}
		e.detailValid, e.detailLoaded = section.loaded, section.loaded
		e.detailKey, e.detailError = key, ""
		e.detailText, e.references = section.text, section.references
		e.supplement, e.supplementNote, e.supplementAt = section.supplement, section.note, section.at
		tab, resources := e.tab, slices.Clone(e.resources)
		commands = append(commands, func() tea.Msg {
			defer cancel()
			current, err := client.Get(ctx, resource, object.GetNamespace(), object.GetName())
			result := exploreDetailResult{run: run, object: current, err: err}
			if err != nil {
				return result
			}
			if object.GetUID() != "" && current.GetUID() != object.GetUID() {
				result.object = nil
				result.text = "This object was deleted and replaced. Go back and select the replacement."
				return result
			}
			if tab == 2 {
				result.text, result.err = client.Events(ctx, *current)
			}
			if tab == 0 {
				result.supplement, result.note = client.SummarySupplement(ctx, resource, *current, resources)
				result.supplementAt = time.Now()
			}
			if tab == 3 {
				result.references, result.text = client.Related(ctx, resource, *current, resources)
			}
			return result
		})
	}
	if e.tab != 4 {
		if e.log != nil {
			e.log.cancel()
			e.log = nil
		}
		e.logKey = ""
		return tea.Batch(commands...)
	}
	if !e.detailValid {
		return tea.Batch(commands...)
	}
	containers := kubernetes.Containers(*object)
	if !e.mergedLogs && !slices.Contains(containers, e.container) {
		e.container = preferredPodContainer(*object)
	}
	logKey := key + "/" + e.container + fmt.Sprint(e.previous, e.logSince)
	if e.container == "" && !e.mergedLogs {
		if e.log != nil {
			e.log.cancel()
			e.log = nil
		}
		return tea.Batch(commands...)
	}
	if e.logKey != logKey {
		if e.log != nil {
			e.log.cancel()
		}
		ctx, cancel := context.WithCancel(context.Background())
		run := &exploreLogRun{cancel: cancel, ctx: ctx, events: make(chan exploreLogEvent, 32)}
		e.log = run
		e.logKey = logKey
		m.resetExploreLogs()
		options := kubernetes.LogOptions{Namespace: object.GetNamespace(), Pod: object.GetName(), UID: object.GetUID(), Container: e.container, Previous: e.previous, Follow: !e.previous}
		timestamps := true
		options.Timestamps = &timestamps
		if e.logSince > 0 {
			since := e.logSince
			options.SinceSeconds = &since
		}
		merged, resources := e.mergedLogs, slices.Clone(e.resources)
		commands = append(commands, func() tea.Msg {
			go func() {
				defer close(run.events)
				send := func(event exploreLogEvent) {
					select {
					case run.events <- event:
					case <-ctx.Done():
					}
				}
				options.OnConnect = func() { send(exploreLogEvent{run: run, connected: true}) }
				var err error
				if merged {
					err = client.WorkloadLogs(ctx, resource, object, resources, options, func(event kubernetes.WorkloadLogEvent) {
						send(exploreLogEvent{run: run, text: event.Text, workloadStatus: event.Status})
					})
				} else {
					err = client.Logs(ctx, options, func(text string) { send(exploreLogEvent{run: run, text: text}) })
				}
				send(exploreLogEvent{run: run, err: err, done: true})
			}()
			return waitExploreLog(run)()
		})
	}
	return tea.Batch(commands...)
}
func waitExploreLog(run *exploreLogRun) tea.Cmd {
	return func() tea.Msg {
		select {
		case event, ok := <-run.events:
			if ok {
				return event
			}
			return exploreLogEvent{run: run, done: true}
		case <-run.ctx.Done():
			return exploreLogEvent{run: run, done: true}
		}
	}
}

func (m *model) exploreRows() ([]string, []kubernetes.Row) {
	e := m.explorer()
	columns, all := kubernetes.Table(e.resource, e.collection, time.Now())
	rows := make([]kubernetes.Row, 0, len(all))
	filter := strings.ToLower(e.preference.Filter)
	for _, row := range all {
		if filter == "" || strings.Contains(strings.ToLower(strings.Join(row.Cells, " ")), filter) {
			rows = append(rows, row)
		}
	}
	column := max(0, min(e.preference.Sort, len(columns)-1))
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].Cells[column], rows[j].Cells[column]
		order := strings.Compare(a, b)
		if columns[column] == "Age" {
			order = rows[j].Object.GetCreationTimestamp().Time.Compare(rows[i].Object.GetCreationTimestamp().Time)
		} else if columns[column] != "Name" && columns[column] != "Namespace" {
			av, aErr := strconv.ParseFloat(a, 64)
			bv, bErr := strconv.ParseFloat(b, 64)
			switch {
			case aErr == nil && bErr == nil:
				order = cmp.Compare(av, bv)
			case aErr == nil:
				order = -1
			case bErr == nil:
				order = 1
			}
		}
		if order == 0 {
			order = strings.Compare(rows[i].Object.GetNamespace()+"/"+rows[i].Object.GetName(), rows[j].Object.GetNamespace()+"/"+rows[j].Object.GetName())
		}
		if e.preference.Descending {
			return order > 0
		}
		return order < 0
	})
	return columns, rows
}
