package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func overviewDesignFixture() (OverviewState, time.Time) {
	now := time.Date(2026, 10, 13, 19, 54, 11, 0, time.UTC)
	k := kubernetes.Overview{ReadAt: now, MetricsAt: now, MetricsEnd: now, MetricsObserved: 13, MetricsExpected: 13, Reads: map[string]kubernetes.OverviewRead{}, NodesReady: 13, PodsRunning: 323, PodsFailed: 3, PodsSucceeded: 21, Restarts: 82, HPACurrent: 28, HPADesired: 31, HPAUp: 1, HPAStable: 5, HPALastScale: now.Add(-2 * time.Minute)}
	k.CPU = kubernetes.OverviewMetric{Used: new(4.34), Allocatable: new(118.79), Requests: new(70.12), Limits: new(79.68)}
	k.Memory = kubernetes.OverviewMetric{Used: new(178.3 * (1 << 30)), Allocatable: new(569.5 * (1 << 30)), Requests: new(275.4 * (1 << 30)), Limits: new(298.7 * (1 << 30))}
	for key, count := range map[string]int{"nodes": 13, "pods": 347, "deployments.apps": 73, "statefulsets.apps": 21, "daemonsets.apps": 8, "horizontalpodautoscalers.autoscaling": 6} {
		k.Reads[key] = kubernetes.OverviewRead{Count: count, ReadAt: now}
	}
	deployment := kubecommon.Resource{Group: "apps", Version: "v1", Name: "deployments", Kind: "Deployment", Namespaced: true}
	pod := kubecommon.Resource{Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true}
	for i := 0; i < 75; i++ {
		k.Changes = append(k.Changes, kubernetes.OverviewEntry{Resource: deployment, Namespace: "services", Name: fmt.Sprintf("scheduled-payment-processing-service-%03d", i), UID: fmt.Sprint(i), Revision: fmt.Sprint(1992 - i), Outcome: "Available", Reason: "Rollout condition: NewReplicaSetAvailable", At: now.Add(-time.Duration(3+i) * time.Minute)})
	}
	for i := 0; i < 3; i++ {
		k.Attention = append(k.Attention, kubernetes.OverviewEntry{Resource: pod, Name: fmt.Sprintf("scheduled-weekly-jobs-298612-abc%03d", i), Namespace: "services", UID: fmt.Sprint(i), Reason: "Error · worker · exit 1", Severity: 2, At: now.Add(-5 * time.Minute), GroupKey: "services/job-uid/Error", GroupTitle: "Job scheduled-weekly-jobs-298612"})
	}
	for i := 0; i < 2; i++ {
		k.Attention = append(k.Attention, kubernetes.OverviewEntry{Resource: pod, Name: fmt.Sprintf("provisioner-monitor-29865354-%03d", i), Namespace: "provisioners", UID: fmt.Sprint(i + 3), Reason: "ReadinessGatesNotReady: external gate failed", Summary: "Readiness gate blocked", Severity: 1})
	}
	s := OverviewState{}
	s.Apply(OverviewData{Kubernetes: k, Helm: &helm.Overview{ReadAt: now, Releases: []helm.OverviewRelease{{Name: "metrics-server", Namespace: "kube-system", Status: "deployed", Revision: 2}, {Name: "ingress-nginx", Namespace: "ingress", Status: "deployed", Revision: 4}}}, Notices: []string{"Argo CD API not detected here; Applications may live in a management cluster", "Flux APIs not detected in this cluster", "Network: no telemetry provider configured"}})
	return s, now
}

func TestOverviewRealisticLayoutsPreserveEssentialFacts(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {76, 18}, {116, 36}, {126, 44}, {176, 44}, {220, 54}} {
		s, now := overviewDesignFixture()
		s.Resize(size[0], size[1])
		out := s.Render("development-v2", size[0], size[1], true, now)
		plain := ansi.Strip(out)
		for _, line := range strings.Split(plain, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("overflow at %v: %s", size, line)
			}
		}
		if len(strings.Split(plain, "\n")) > size[1] {
			t.Fatal("height overflow", size)
		}
		if size[0] >= 116 {
			for _, value := range []string{"4% used", "31% used", "118.79c", "70.12c", "3 errors", "2 warnings", "Available", "r1992"} {
				if !strings.Contains(plain, value) {
					t.Errorf("%v lost %q:\n%s", size, value, plain)
				}
			}
			if size[0] >= 120 {
				for _, word := range []string{"Autoscaling", "Kubernetes", "347", "73", "statefulsets", "2 integrations", "569.5Gi", "275.4Gi", "21 done"} {
					if !strings.Contains(plain, word) {
						t.Errorf("%v missing inventory/tile %s", size, word)
					}
				}
			} else {
				for _, index := range []int{1, 3} {
					s.Cursor[0] = index
					selected := ansi.Strip(s.Render("development-v2", size[0], size[1], true, now))
					value := "275.4Gi"
					if index == 3 {
						value = "21 done"
					}
					if !strings.Contains(selected, value) {
						t.Fatal("selected detail missing", value, selected)
					}
				}
				s.Pane = 3
				snapshot := ansi.Strip(s.Render("development-v2", size[0], size[1], true, now))
				if !strings.Contains(snapshot, "2 integrations") {
					t.Fatal("medium layout cannot reach inventory")
				}
			}
			if strings.Contains(plain, "New…") || strings.Contains(plain, "Coverage") {
				t.Errorf("unreadable outcomes or old inventory notices at %v", size)
			}
		}
		if directory := os.Getenv("PLOUS_REVIEW_DIR"); directory != "" {
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			for suffix, content := range map[string]string{"ansi": out, "txt": plain} {
				if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("overview-main-%dx%d.%s", size[0], size[1], suffix)), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestOverviewScrollKeepsContextAndIndependentPositions(t *testing.T) {
	s, now := overviewDesignFixture()
	s.Pane = 1
	s.Resize(126, 44)
	s.Move("down")
	if s.Scroll[1] != 0 {
		t.Fatal("scroll jumped before selection reached edge")
	}
	for range 20 {
		s.Move("down")
	}
	offset := s.Scroll[1]
	if offset == 0 {
		t.Fatal("selection did not scroll into view")
	}
	s.Move("right")
	s.Move("down")
	if s.Scroll[1] != offset {
		t.Fatal("another panel moved deployments")
	}
	s.Resize(116, 36)
	if !strings.Contains(ansi.Strip(s.renderPane(1, 36, 20, true, now)), "021") {
		t.Fatal("resize lost selected resource")
	}
}

func TestOverviewGroupsPreserveEveryMemberAndSearchIdentity(t *testing.T) {
	s, _ := overviewDesignFixture()
	s.Pane = 2
	if len(s.Rows[2]) != 3 || len(s.Rows[2][0].Members) != 3 {
		t.Fatal("related Pods were not grouped", s.Rows[2])
	}
	s.Query = "abc002"
	if len(s.Filtered(2)) != 1 || !s.OpenGroup() {
		t.Fatal("cannot find/open a group by its member")
	}
	if len(s.Filtered(2)) != 3 {
		t.Fatal("group omitted affected resources")
	}
	for i := 0; i < 3; i++ {
		s.Cursor[2] = i
		if !strings.HasSuffix(s.Selected().Name, fmt.Sprintf("abc%03d", i)) {
			t.Fatal("member target lost")
		}
	}
	if !s.CloseGroup() || s.Query != "abc002" {
		t.Fatal("group back lost search")
	}
}

func TestOverviewHistoryUsesDistinctRealSamplesAndExpires(t *testing.T) {
	s, now := overviewDesignFixture()
	s.Apply(s.Data)
	if len(s.Samples[0]) != 1 {
		t.Fatal("duplicate source timestamp fabricated history")
	}
	data := s.Data
	data.Kubernetes.MetricsAt = now.Add(15 * time.Second)
	data.Kubernetes.ReadAt = now.Add(15 * time.Second)
	s.Apply(data)
	if len(s.Samples[0]) != 2 {
		t.Fatal("new observation not sampled")
	}
	out := strings.Join(s.metricTileSized(0, 26, 8, now.Add(15*time.Second)), "\n")
	if !strings.Contains(out, "sampled") {
		t.Fatal("real history not shown")
	}
	out = strings.Join(s.metricTileSized(0, 26, 8, now.Add(3*time.Minute)), "\n")
	if strings.Contains(out, "sampled") || !strings.Contains(out, "STALE") {
		t.Fatal("stale measurements still looked current")
	}
}

func TestOverviewRetainsChangesWithoutRewritingEvidence(t *testing.T) {
	s, now := overviewDesignFixture()
	s.Apply(s.Data)
	if len(s.Rows[1]) != 75 {
		t.Fatal("poll duplicated changes")
	}
	data := s.Data
	data.Kubernetes.Changes = append([]kubernetes.OverviewEntry(nil), data.Kubernetes.Changes...)
	data.Kubernetes.Changes[0].Revision = "1993"
	data.Kubernetes.Changes[0].At = now.Add(time.Minute)
	data.Kubernetes.ReadAt = now.Add(time.Minute)
	s.Apply(data)
	if len(s.Rows[1]) != 76 || s.Rows[1][0].Revision != "1993" {
		t.Fatal("new revision replaced history")
	}
	s.Pane = 1
	s.Cursor[1] = 1
	if !s.Rows[1][1].Historical || !strings.Contains(s.Selected().Evidence, "1992") || !strings.Contains(s.SelectedEvidence(), "previously reported") {
		t.Fatal("older change opens current state")
	}
	data.Kubernetes.Changes = nil
	s.Apply(data)
	if len(s.Rows[1]) != 76 || !s.Rows[1][0].Historical {
		t.Fatal("deleted or inaccessible resources removed evidence")
	}
	for i := 0; i < 250; i++ {
		data.Kubernetes.Changes = []kubernetes.OverviewEntry{{Name: "rolling", UID: "same", Revision: fmt.Sprint(i), At: now.Add(time.Duration(i+2) * time.Minute)}}
		s.Apply(data)
	}
	if len(s.Rows[1]) != 200 {
		t.Fatal("change history is unbounded")
	}
}

func TestOverviewCompactHealthAndChartResolution(t *testing.T) {
	s, now := overviewDesignFixture()
	s.Pane = 2
	out := ansi.Strip(s.Render("development-v2", 76, 18, true, now))
	for _, value := range []string{"CPU 4%", "MEM 31%", "Nodes 13/13", "Needs Attention"} {
		if !strings.Contains(out, value) {
			t.Fatal("compact health missing", value)
		}
	}
	data := s.Data
	for i := 1; i < 40; i++ {
		data.Kubernetes.ReadAt = now.Add(time.Duration(i) * 15 * time.Second)
		data.Kubernetes.MetricsAt = data.Kubernetes.ReadAt
		data.Kubernetes.CPU.Used = new(3.0 + float64(i%7))
		s.Apply(data)
	}
	chart := s.metricTileSized(0, 24, 8, data.Kubernetes.ReadAt)
	if len(chart) != 8 || s.Scales[0] >= 100 {
		t.Fatal("low utilization chart lacks resolution", s.Scales)
	}
	nonempty := 0
	for _, line := range chart[2:5] {
		if strings.TrimSpace(ansi.Strip(line)) != "" {
			nonempty++
		}
	}
	if nonempty < 2 {
		t.Fatal("history collapsed to a single row")
	}
}
