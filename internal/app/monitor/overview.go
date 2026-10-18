package monitor

import (
	"fmt"
	"sort"
	"strings"
	"time"

	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	helpview "github.com/simjay/plous/internal/ui/help"
	textview "github.com/simjay/plous/internal/ui/text"
)

type OverviewData struct {
	Kubernetes kubernetes.Overview
	Helm       *helm.Overview
	Argo       *gitops.Snapshot
	Flux       []gitops.Snapshot
	Notices    []string
}

type OverviewTarget struct {
	Tool                 string
	Resource             kubecommon.Resource
	Name, Namespace, UID string
	Revision             int
	Ref                  gitops.Reference
	Evidence             string
}

type OverviewRow struct {
	ID, Title, Detail, Status, Source             string
	Reason, Revision, GroupKey, GroupTitle, Cause string
	Action                                        string
	ObservedAt                                    time.Time
	Historical                                    bool
	At                                            time.Time
	Severity                                      int
	Target                                        OverviewTarget
	Members                                       []OverviewRow
	Counters                                      []OverviewCounter
	NewFailures                                   int
}

type OverviewCounter struct{ Label, Value string }

type OverviewState struct {
	Data                             OverviewData
	Delta                            kubernetes.OverviewDelta
	ComparedSince, ComparedAt        time.Time
	Loaded, Loading                  bool
	Retained                         bool
	Error                            string
	Pane                             int
	Expanded                         bool
	Cursor                           [4]int
	Query                            string
	Input                            *textview.Field
	Rows                             [4][]OverviewRow
	Scroll                           [4]int
	Width, Height                    int
	Group                            string
	GroupCursor                      int
	GroupScroll                      int
	GroupQuery                       string
	Samples                          [2][]ui.ChartSample
	Usage                            *kubernetes.OverviewUsage
	MetricDetail                     *OverviewMetricDetail
	Scales                           [2]float64
	Changes                          []OverviewRow
	Counter                          int
	InventoryTool, InventoryCounter  string
	InventoryCursor, InventoryScroll int
	InventoryQuery                   string
}

var OverviewPanes = []string{"Cluster Pulse", "Recent Deployments", "Needs Attention", "Infrastructure Snapshot"}

func (s *OverviewState) Apply(data OverviewData) {
	if s.Usage != nil {
		s.Usage.Apply(&data.Kubernetes)
	}
	selected := [4]string{}
	for p := range selected {
		rows := s.Filtered(p)
		if len(rows) > 0 {
			selected[p] = rows[min(s.Cursor[p], len(rows)-1)].ID
		}
	}
	previous := s.Data
	previousFailures := overviewFailureIDs(s.Rows[2])
	compareHealth := s.Loaded && !s.Retained && s.Error == "" && s.healthComplete()
	s.Delta = kubernetes.OverviewDelta{}
	if s.Loaded && !s.Retained && s.Error == "" {
		s.Delta = kubernetes.CompareOverview(previous.Kubernetes, data.Kubernetes)
	}
	s.ComparedSince, s.ComparedAt = time.Time{}, time.Time{}
	s.Data, s.Loaded, s.Loading, s.Error = data, true, false, ""
	s.Retained = false
	s.Rows = [4][]OverviewRow{}
	k := data.Kubernetes
	s.captureUsage()
	for _, e := range k.Changes {
		s.Rows[1] = append(s.Rows[1], overviewKubeRow(e))
	}
	for _, e := range k.Attention {
		s.Rows[2] = append(s.Rows[2], overviewKubeRow(e))
	}
	counts := []string{}
	counters := []OverviewCounter{}
	status := ""
	readAt := time.Time{}
	for _, key := range []string{"pods", "deployments.apps", "statefulsets.apps", "nodes"} {
		r := k.Reads[key]
		if r.Error != "" || r.Partial || r.ReadAt.IsZero() {
			status = "Partial"
		}
		if !r.ReadAt.IsZero() && (readAt.IsZero() || r.ReadAt.Before(readAt)) {
			readAt = r.ReadAt
		}
		n := "Unavailable"
		if r.Error == "" && !r.ReadAt.IsZero() {
			n = fmt.Sprint(r.Count)
			if r.Partial {
				n += "+ (partial)"
			}
		}
		label := strings.Split(key, ".")[0]
		counts = append(counts, label+" "+n)
		counters = append(counters, OverviewCounter{Label: map[string]string{"pods": "Pods", "deployments": "Deployments", "statefulsets": "StatefulSets", "nodes": "Nodes"}[label], Value: n})
	}
	s.Rows[3] = append(s.Rows[3], OverviewRow{ID: "kubernetes", Title: "⎈ Kubernetes", Detail: strings.Join(counts, " · "), Source: "Kubernetes API", Status: status, At: readAt, Target: OverviewTarget{Tool: "kubernetes", Resource: kubecommon.Resource{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true}}})
	s.Rows[3][0].Counters = counters
	if h := data.Helm; h != nil {
		if h.Error == "" || len(h.Releases) > 0 {
			deployed, pending, failed := 0, 0, 0
			for _, release := range h.Releases {
				if overviewHelmCounter("Deployed", release) {
					deployed++
				}
				if overviewHelmCounter("Pending", release) {
					pending++
				}
				if overviewHelmCounter("Failed", release) {
					failed++
				}
				row := OverviewRow{ID: "helm/" + release.Namespace + "/" + release.Name, Title: release.Name, Detail: fmt.Sprintf("%s · storage revision %d", release.Namespace, release.Revision), Source: "Helm", Status: release.Status, At: release.Updated, Target: OverviewTarget{Tool: "helm", Name: release.Name, Namespace: release.Namespace, Revision: release.Revision}}
				row.Revision, row.Reason, row.Action = fmt.Sprint(release.Revision), "Release "+release.Status, "Release"
				if !row.At.IsZero() {
					s.Rows[1] = append(s.Rows[1], row)
				}
				if release.Status == "failed" || strings.HasPrefix(release.Status, "pending-") {
					row.Severity = 1
					if release.Status == "failed" {
						row.Severity = 2
					}
					s.Rows[2] = append(s.Rows[2], row)
				}
			}
			status := ""
			if h.Partial || h.Error != "" {
				status = "Partial"
			}
			s.Rows[3] = append(s.Rows[3], OverviewRow{ID: "helm", Title: "◉ Helm", Detail: fmt.Sprintf("Releases %d · deployed %d · pending %d · failed %d", len(h.Releases), deployed, pending, failed), Status: status, Source: "Helm storage metadata", At: h.ReadAt, Target: OverviewTarget{Tool: "helm"}})
			s.Rows[3][len(s.Rows[3])-1].Counters = []OverviewCounter{{"Releases", fmt.Sprint(len(h.Releases))}, {"Deployed", fmt.Sprint(deployed)}, {"Pending", fmt.Sprint(pending)}, {"Failed", fmt.Sprint(failed)}}
			if status != "" {
				for i := range s.Rows[3][len(s.Rows[3])-1].Counters {
					s.Rows[3][len(s.Rows[3])-1].Counters[i].Value += "+"
				}
			}
		}
	}
	if data.Argo != nil {
		s.addGitOps("argocd", []gitops.Snapshot{*data.Argo})
	}
	if len(data.Flux) > 0 {
		s.addGitOps("flux", data.Flux)
	}
	s.retainChanges(k.ReadAt)
	grouped := []OverviewRow{}
	positions := map[string]int{}
	for _, row := range s.Rows[2] {
		id := row.Target.Tool + "/" + row.Target.Resource.Key() + "/" + row.Target.Namespace + "/" + row.Target.Name
		if row.Target.Ref.Kind != "" {
			id = row.Target.Tool + "/" + row.Target.Ref.Key()
		}
		if i, ok := positions[id]; ok {
			grouped[i].Detail += "; " + row.Detail
			if row.Severity > grouped[i].Severity {
				grouped[i].Reason, grouped[i].Status = row.Reason, row.Status
			}
			grouped[i].Severity = max(grouped[i].Severity, row.Severity)
			if row.At.After(grouped[i].At) {
				grouped[i].At = row.At
			}
		} else {
			row.ID = id
			positions[id] = len(grouped)
			grouped = append(grouped, row)
		}
	}
	s.Rows[2] = grouped
	groups := []OverviewRow{}
	positions = map[string]int{}
	for _, row := range grouped {
		if row.GroupKey == "" {
			groups = append(groups, row)
			continue
		}
		id := row.GroupKey + fmt.Sprint(row.Severity)
		if index, ok := positions[id]; ok {
			groups[index].Members = append(groups[index].Members, row)
			if row.At.After(groups[index].At) {
				groups[index].At = row.At
			}
		} else {
			positions[id] = len(groups)
			copy := row
			copy.ID, copy.Members = id, []OverviewRow{row}
			groups = append(groups, copy)
		}
	}
	for i := range groups {
		if len(groups[i].Members) > 1 {
			groups[i].Title = groups[i].GroupTitle
			for _, member := range groups[i].Members {
				if member.Reason != groups[i].Reason && groups[i].Cause != "" {
					groups[i].Reason = groups[i].Cause
					break
				}
			}
		}
	}
	s.Rows[2] = groups
	gap := k.ReadAt.Sub(previous.Kubernetes.ReadAt)
	if compareHealth && s.healthComplete() && gap > 0 && gap <= 45*time.Second && overviewSourceSet(previous) == overviewSourceSet(data) {
		s.ComparedSince, s.ComparedAt = previous.Kubernetes.ReadAt, k.ReadAt
		for i, row := range s.Rows[2] {
			for key := range overviewFailureIDs([]OverviewRow{row}) {
				if !previousFailures[key] {
					s.Rows[2][i].NewFailures++
				}
			}
		}
	}
	sort.SliceStable(s.Rows[2], func(i, j int) bool {
		a, b := s.Rows[2][i], s.Rows[2][j]
		if a.Severity != b.Severity {
			return a.Severity > b.Severity
		}
		if !a.At.Equal(b.At) {
			return a.At.After(b.At)
		}
		return a.ID < b.ID
	})
	s.updateCoverage()
	for p := range selected {
		if p == 0 {
			continue
		}
		rows := s.Filtered(p)
		s.Cursor[p] = min(s.Cursor[p], max(0, len(rows)-1))
		for i, row := range rows {
			if row.ID == selected[p] {
				s.Cursor[p] = i
				break
			}
		}
	}
	if s.Group != "" {
		found := false
		for _, row := range s.Rows[2] {
			found = found || row.ID == s.Group && len(row.Members) > 1
		}
		if !found {
			s.Group = ""
			s.Cursor[2] = 0
			s.Scroll[2] = 0
		}
	}
	s.Resize(s.Width, s.Height)
}

func overviewKubeRow(e kubernetes.OverviewEntry) OverviewRow {
	detail := e.Resource.Kind + " · " + e.Namespace
	if e.Namespace == "" {
		detail = e.Resource.Kind + " · cluster"
	}
	if e.Revision != "" {
		detail += " · revision " + e.Revision
	}
	reason := e.Summary
	if reason == "" {
		reason = e.Reason
	}
	return OverviewRow{Action: "Rollout", ID: e.Resource.Key() + "/" + e.Namespace + "/" + e.Name + "/" + e.Reason, Title: e.Name, Detail: detail + " · " + e.Reason, Reason: reason, Cause: e.Cause, Revision: e.Revision, GroupKey: e.GroupKey, GroupTitle: e.GroupTitle, Status: e.Outcome, Source: "Kubernetes", At: e.At, Severity: e.Severity, Target: OverviewTarget{Tool: "kubernetes", Resource: e.Resource, Name: e.Name, Namespace: e.Namespace, UID: e.UID}}
}

func (s *OverviewState) addGitOps(tool string, snapshots []gitops.Snapshot) {
	total, synced, out, degraded, kustomizations, releases, sources, notReady := 0, 0, 0, 0, 0, 0, 0, 0
	readable, partial := false, false
	var readAt time.Time
	for _, snapshot := range snapshots {
		if !snapshot.ReadAt.IsZero() && (readAt.IsZero() || snapshot.ReadAt.Before(readAt)) {
			readAt = snapshot.ReadAt
		}
		if snapshot.State == gitops.Available || snapshot.State == gitops.Empty || snapshot.State == gitops.Partial {
			readable = true
		}
		if snapshot.State != gitops.Available && snapshot.State != gitops.Empty || snapshot.Truncated {
			partial = true
		}
		for _, item := range snapshot.Items {
			total++
			if overviewGitOpsCounter("Synced", item) {
				synced++
			}
			if overviewGitOpsCounter("OutOfSync", item) {
				out++
			}
			if overviewGitOpsCounter("Degraded", item) {
				degraded++
			}
			switch {
			case overviewGitOpsCounter("Kustomizations", item):
				kustomizations++
			case overviewGitOpsCounter("HelmReleases", item):
				releases++
			default:
				sources++
			}
			if overviewGitOpsCounter("NotReady", item) {
				notReady++
			}
			row := OverviewRow{ID: item.Ref.Key(), Title: item.Ref.Name, Detail: item.Ref.Kind + " · " + item.Ref.Namespace + " · " + item.Reason, Status: item.Status, Source: "Flux", At: item.ObservedAt, Target: OverviewTarget{Tool: tool, Ref: item.Ref, Name: item.Ref.Name, Namespace: item.Ref.Namespace, UID: item.UID}}
			revision := gitops.FieldValue(item, "Last applied revision")
			if tool == "argocd" {
				row.Action = "Sync"
				row.Source = "Argo CD"
				row.Status = gitops.FieldValue(item, "Latest operation")
				row.At = gitops.Timestamp(gitops.FieldValue(item, "Operation finished"))
				revision = gitops.FieldValue(item, "Successful operation revision")
				row.Detail = item.Ref.Namespace + " · Sync operation " + gitops.FieldValue(item, "Latest operation")
			} else {
				row.Action = "Reconcile"
				row.Detail = item.Ref.Kind + " · " + item.Ref.Namespace + " · Condition changed"
			}
			if revision != "" {
				row.Detail += " · " + revision
			}
			row.Revision, row.Reason = revision, item.Reason
			if !row.At.IsZero() {
				s.Rows[1] = append(s.Rows[1], row)
			}
			if item.ControllerError || item.Health == "Degraded" || item.Health == "Missing" || item.Sync == "OutOfSync" || item.Status == "Not ready" || item.Status == "Stalled" || item.Stale || gitops.FieldValue(item, "Latest operation") == "Failed" || gitops.FieldValue(item, "Latest operation") == "Error" {
				if tool == "argocd" {
					row.Status = item.Sync + " / " + item.Health
				}
				row.Detail = item.Ref.Namespace + " · " + item.Reason
				row.Severity = 1
				if item.ControllerError || item.Health == "Degraded" || item.Status == "Not ready" || item.Status == "Stalled" {
					row.Severity = 2
				}
				s.Rows[2] = append(s.Rows[2], row)
			}
		}
	}
	if !readable {
		return
	}
	count := func(n int) string {
		if partial {
			return fmt.Sprintf("%d+", n)
		}
		return fmt.Sprint(n)
	}
	fluxCount := func(index, n int) string {
		if index >= len(snapshots) {
			return "?"
		}
		snapshot := snapshots[index]
		if snapshot.State != gitops.Available && snapshot.State != gitops.Empty && snapshot.State != gitops.Partial {
			return "?"
		}
		if snapshot.State == gitops.Partial || snapshot.Truncated {
			return fmt.Sprintf("%d+", n)
		}
		return fmt.Sprint(n)
	}
	title, detail := "◇ Flux", fmt.Sprintf("Kustomizations %s · HelmReleases %s · Sources %s · NotReady %s", fluxCount(0, kustomizations), fluxCount(1, releases), fluxCount(2, sources), count(notReady))
	if tool == "argocd" {
		title = "◎ Argo CD"
		detail = fmt.Sprintf("Applications %s · Synced %s · OutOfSync %s · Degraded %s", count(total), count(synced), count(out), count(degraded))
	}

	status := ""
	if partial {
		status = "Partial"
	}
	s.Rows[3] = append(s.Rows[3], OverviewRow{ID: tool, Title: title, Detail: detail, Source: "Controller API", Status: status, At: readAt, Target: OverviewTarget{Tool: tool}})
	counters := []OverviewCounter{{"Kustomizations", fluxCount(0, kustomizations)}, {"HelmReleases", fluxCount(1, releases)}, {"Sources", fluxCount(2, sources)}, {"NotReady", count(notReady)}}
	if tool == "argocd" {
		counters = []OverviewCounter{{"Applications", count(total)}, {"Synced", count(synced)}, {"OutOfSync", count(out)}, {"Degraded", count(degraded)}}
	}
	s.Rows[3][len(s.Rows[3])-1].Counters = counters
}

func (s *OverviewState) partial() bool {
	for _, r := range s.Data.Kubernetes.Reads {
		if r.Error != "" || r.Partial {
			return true
		}
	}
	if h := s.Data.Helm; h != nil && (h.Error != "" || h.Partial) {
		return true
	}
	if a := s.Data.Argo; a != nil && (a.State != gitops.Available && a.State != gitops.Empty || a.Truncated) {
		return true
	}
	for _, f := range s.Data.Flux {
		if f.State != gitops.Available && f.State != gitops.Empty || f.Truncated {
			return true
		}
	}
	return false
}

func (s *OverviewState) Filtered(pane int) []OverviewRow {
	query := s.Query
	if s.Input != nil {
		query = s.Input.Text
	}
	var rows []OverviewRow
	source := s.Rows[pane]
	if pane == 3 && s.InventoryTool != "" {
		source = s.inventoryRows()
	}
	if pane == 2 && s.Group != "" {
		for _, row := range source {
			if row.ID == s.Group {
				source = row.Members
				break
			}
		}
	}
	for _, row := range source {
		text := row.Title + " " + row.Detail + " " + row.Source + " " + row.Status
		for _, member := range row.Members {
			text += " " + member.Title + " " + member.Detail
		}
		if ui.SearchContains(text, query) {
			rows = append(rows, row)
		}
	}
	return rows
}
func (s *OverviewState) Selected() *OverviewTarget {
	if s.Pane == 0 {
		resources := []kubecommon.Resource{
			{Name: "nodes", Version: "v1", Kind: "Node"},
			{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true},
			{Name: "horizontalpodautoscalers", Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler", Namespaced: true},
		}
		index := s.Cursor[0] - 2
		if index < 0 || index >= len(resources) {
			return nil
		}
		resource := resources[index]
		if read := s.Data.Kubernetes.Reads[resource.Key()]; read.Resource.Name != "" {
			resource = read.Resource
		}
		return &OverviewTarget{Tool: "kubernetes", Resource: resource}
	}
	rows := s.Filtered(s.Pane)
	if len(rows) == 0 {
		return nil
	}
	target := rows[min(s.Cursor[s.Pane], len(rows)-1)].Target
	if s.Pane == 3 && s.InventoryTool == "" && s.Counter > 0 && target.Tool == "kubernetes" {
		target.Resource = []kubecommon.Resource{
			{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true},
			{Name: "deployments", Group: "apps", Version: "v1", Kind: "Deployment", Namespaced: true},
			{Name: "statefulsets", Group: "apps", Version: "v1", Kind: "StatefulSet", Namespaced: true},
			{Name: "nodes", Version: "v1", Kind: "Node"},
		}[min(s.Counter-1, 3)]
	}
	if s.Pane == 1 && rows[min(s.Cursor[s.Pane], len(rows)-1)].Historical {
		target.Evidence = overviewChangeEvidence(rows[min(s.Cursor[s.Pane], len(rows)-1)])
	}
	return &target
}
func (s *OverviewState) Move(key string) {
	defer s.Resize(s.Width, s.Height)
	if s.Input == nil && s.moveArrow(key) {
		return
	}
	count := len(s.Filtered(s.Pane))
	if s.Pane == 0 {
		count = 5
	}
	switch key {
	case "up", "k":
		s.Cursor[s.Pane]--
	case "down", "j":
		s.Cursor[s.Pane]++
	case "pgup":
		s.Cursor[s.Pane] -= 5
	case "pgdown":
		s.Cursor[s.Pane] += 5
	case "home":
		s.Cursor[s.Pane] = 0
	case "end":
		s.Cursor[s.Pane] = count - 1
	}
	s.Cursor[s.Pane] = max(0, min(s.Cursor[s.Pane], max(0, count-1)))
	if s.Pane == 3 {
		s.Counter = 0
	}
}
func (s *OverviewState) Help() helpview.Section {
	if d := s.MetricDetail; d != nil {
		h := d.Reader().Help(d.Title(), "Return to Overview")
		if d.Sources {
			h = d.Reader().Help(d.Title()+" · Source details", "Return to chart")
		}
		if action := d.EnterLabel(); action != "" {
			h.Add("Details", helpview.Key("enter", "Enter", action))
		}
		h.Add("View", helpview.Command("fullscreen", "ff", "Fullscreen main"), helpview.Command("refresh", "g", "Refresh cluster observations"))
		return h
	}
	h := helpview.New("Overview · Selected cluster", helpview.Key("horizontal", "←→", "Move across tiles or columns"), helpview.Key("vertical", "↑↓ / j k", "Move through rows; cross sections at edges"), helpview.Key("inspect", "Enter", s.EnterLabel()))
	h.Add("View", helpview.Command("expandsection", "e", "Expand / restore selected section"), helpview.Command("fullscreen", "ff", "Fullscreen main"), helpview.Command("refresh", "g", "Refresh cluster observations"), helpview.Key("search", "/", "Filter Overview lists"))
	h.Add("Snapshot", helpview.Key("counter", "↑↓ / j k", "Move through integrations and their counters"))
	h.Note("Arrows follow the layout without wrapping. Down leaves Pulse for the section below; Up from the first row returns above. Stacked and compact sections connect at their first and last rows. Page keys and Home/End stay within the section.")
	h.Note("All namespaces in this captured cluster. Failed and truncated reads are reported. CPU and memory usage come from Node metrics; requests and limits come from active Pod specifications. Recent entries retain up to 200 reported changes in this session, not a complete audit trail.")
	return h
}

func (s *OverviewState) EnterLabel() string {
	if s.Pane == 0 {
		return []string{"CPU details", "Memory details", "Nodes", "Pods", "HPAs"}[min(s.Cursor[0], 4)]
	}
	rows := s.Filtered(s.Pane)
	if len(rows) == 0 {
		return "Evidence"
	}
	row := rows[min(s.Cursor[s.Pane], len(rows)-1)]
	if s.Pane == 1 && row.Historical {
		return "Recorded evidence"
	}
	if s.Pane == 2 && len(row.Members) > 1 {
		return fmt.Sprintf("%d affected resources", len(row.Members))
	}
	if s.Pane == 3 && s.InventoryTool == "" {
		if len(row.Counters) == 0 {
			return "Availability"
		}
		label := overviewCounterLabel(row.Counters[min(max(0, s.Counter-1), len(row.Counters)-1)].Label)
		if row.Target.Tool == "helm" && label != "Releases" {
			return label + " releases"
		}
		if row.Target.Tool == "argocd" && label != "Applications" {
			return label + " apps"
		}
		if label == "Not ready" {
			return "Not ready objects"
		}
		return label
	}
	kind := row.Target.Resource.Kind
	if kind == "" {
		kind = row.Target.Ref.Kind
	}
	if row.Target.Tool == "helm" {
		kind = "release"
	}
	if kind == "" {
		kind = "resource"
	}
	return "Inspect " + kind
}

func (s *OverviewState) OpenGroup() bool {
	if s.Pane != 2 || s.Group != "" {
		return false
	}
	rows := s.Filtered(2)
	if len(rows) == 0 {
		return false
	}
	row := rows[min(s.Cursor[2], len(rows)-1)]
	if len(row.Members) < 2 {
		return false
	}
	s.Group, s.GroupCursor = row.ID, s.Cursor[2]
	s.GroupQuery, s.GroupScroll = s.Query, s.Scroll[2]
	s.Query = ""
	s.Cursor[2], s.Scroll[2] = 0, 0
	return true
}

func (s *OverviewState) CloseGroup() bool {
	if s.Group == "" {
		return false
	}
	s.Group = ""
	s.Query = s.GroupQuery
	s.Cursor[2], s.Scroll[2] = s.GroupCursor, s.GroupScroll
	return true
}
