package monitor

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func TestOverviewChartsKeepLowUsageVisibleAndScaleExplicit(t *testing.T) {
	s, now := overviewDesignFixture()
	data := s.Data
	data.Kubernetes.MetricsAt = now.Add(15 * time.Second)
	data.Kubernetes.ReadAt = data.Kubernetes.MetricsAt
	data.Kubernetes.CPU.RequestsMissing = 1
	s.Apply(data)
	for i, scale := range []string{"0–5%", "0–50%"} {
		if got := strings.Join(s.metricHistory(i, 40, 7, data.Kubernetes.ReadAt), "\n"); !strings.Contains(got, scale) {
			t.Fatal("independent chart scale missing", got)
		}
	}
	for _, size := range [][2]int{{90, 34}, {150, 44}} {
		s.Pane = 0
		view := ansi.Strip(s.Render("cluster", size[0], size[1], true, data.Kubernetes.ReadAt))
		plottedRows := 0
		for _, row := range strings.Split(view, "\n") {
			if strings.ContainsAny(row, "▁▂▃▄▅▆▇█") {
				plottedRows++
			}
		}
		if plottedRows < 3 || !strings.Contains(view, "0–5%") || !strings.Contains(view, "unspecified") {
			t.Fatalf("%v squeezed low usage or hid its scale/budget note:\n%s", size, view)
		}
	}
	s.Pane, s.Expanded = 0, true
	if got := strings.Join(s.metricHistory(0, 40, 7, data.Kubernetes.ReadAt), "\n"); !strings.Contains(got, "0–5%") {
		t.Fatal("expansion changed the chart scale", got)
	}
}

func TestOverviewStartupChartUsesCollectedRange(t *testing.T) {
	s, now := overviewDesignFixture()
	data := s.Data
	for i := 1; i <= 3; i++ {
		data.Kubernetes.MetricsAt = now.Add(time.Duration(i) * 15 * time.Second)
		data.Kubernetes.ReadAt = data.Kubernetes.MetricsAt
		s.Apply(data)
	}
	chart := s.metricHistory(0, 60, 6, data.Kubernetes.ReadAt)
	if !strings.Contains(ansi.Strip(chart[4]), "-1m") || !strings.Contains(chart[5], "45s sampled") {
		t.Fatal("startup history was squeezed into a full fifteen-minute range", chart)
	}
	baseline := []rune(ansi.Strip(chart[3]))
	if strings.TrimSpace(string(baseline[:15])) != "" || strings.Contains(string(baseline[15:]), " ") {
		t.Fatal("startup chart backfilled history or disconnected consecutive observations", chart)
	}
}

func TestOverviewCompactHealthKeepsProblemsAndUnknownCoverage(t *testing.T) {
	now := time.Now()
	row := OverviewRow{Target: OverviewTarget{Tool: "helm"}, At: now, Counters: []OverviewCounter{{"Releases", "2"}, {"Deployed", "1"}, {"Pending", "0"}, {"Failed", "1"}}}
	got := ansi.Strip(strings.Join(overviewCompactIntegration(row, 40, false, 0, now), "\n"))
	if !strings.Contains(got, "Helm") || !strings.Contains(got, "2 releases") || !strings.Contains(got, "1 failed") {
		t.Fatal(got)
	}
	got = ansi.Strip(strings.Join(overviewCompactIntegration(row, 24, true, 3, now), "\n"))
	for _, fact := range []string{"2 releases", "› 0 pending", "× 1 failed"} {
		if !strings.Contains(got, fact) {
			t.Fatal("wrapped selection lost inventory, counter, or condition", fact, got)
		}
	}
	row.Status = "Partial"
	row.Counters[3].Value = "?"
	got = ansi.Strip(strings.Join(overviewCompactIntegration(row, 40, false, 0, now), "\n"))
	if strings.Contains(got, "× ?") || !strings.Contains(got, "? failed") || !strings.Contains(got, "Partial") {
		t.Fatal("unknown count implied failure/health", got)
	}
	row.At = now.Add(-time.Minute)
	if got := ansi.Strip(strings.Join(overviewCompactIntegration(row, 40, false, 0, now), "\n")); !strings.Contains(got, "stale") {
		t.Fatal(got)
	}
}

func TestOverviewPaneFootersStayVisibleAcrossHeights(t *testing.T) {
	s, now := overviewDesignFixture()
	for height := 10; height <= 40; height++ {
		got := ansi.Strip(s.renderPane(1, 44, height, false, now))
		if !strings.Contains(got, "of 75") {
			t.Fatalf("height %d lost scroll context:\n%s", height, got)
		}
	}
}

func TestOverviewNarrowRowsWrapWithoutLosingScrollTargets(t *testing.T) {
	s, now := overviewDesignFixture()
	s.Pane = 2
	s.Rows[2] = []OverviewRow{
		{ID: "first", Title: "first-checkout", Source: "Kubernetes", Reason: "CrashLoopBackOff", Severity: 2, At: now.Add(-time.Minute), Target: OverviewTarget{Namespace: "shop"}},
		{ID: "second", Title: "second-checkout", Source: "Kubernetes", Reason: "ReadinessGatesNotReady", Severity: 1, At: now.Add(-2 * time.Minute), Target: OverviewTarget{Namespace: "shop"}},
	}
	s.Resize(20, 14)
	for index, reason := range []string{"CrashLoopBackOff", "ReadinessGatesNotReady"} {
		if index > 0 {
			s.Move("down")
			if s.Scroll[2] == 0 {
				t.Fatal("selection did not scroll through wrapped rows")
			}
		}
		if s.Cursor[2] != index {
			t.Fatal("wrapped rows changed selection", s.Cursor[2])
		}
		for _, focused := range []bool{true, false} {
			p := s.paneContent(2, 20, 10, focused, now)
			row := ansi.Strip(strings.Join(p.lines[p.starts[index]:p.ends[index]], ""))
			row = strings.ReplaceAll(row, " ", "")
			for _, fact := range []string{reason, s.Rows[2][index].Title, "K8s·shop"} {
				if !strings.Contains(row, fact) {
					t.Fatalf("wrapped row lost %q: %s", fact, row)
				}
			}
			view := ansi.Strip(s.renderPane(2, 20, 10, focused, now))
			if !strings.Contains(view, "Needs Attention") || !strings.Contains(view, s.Rows[2][index].Title) || !strings.Contains(view, "of 2") {
				t.Fatal("scroll lost selected row or context", view)
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > 20 {
					t.Fatal("wrapped row overflowed", line)
				}
			}
		}
	}
}

func TestOverviewSnapshotMarksPartialCountsAndOldestRead(t *testing.T) {
	s, now := overviewDesignFixture()
	data := s.Data
	data.Kubernetes.Reads = map[string]kubernetes.OverviewRead{}
	for key, read := range s.Data.Kubernetes.Reads {
		data.Kubernetes.Reads[key] = read
	}
	read := data.Kubernetes.Reads["pods"]
	read.ReadAt = now.Add(-20 * time.Second)
	data.Kubernetes.Reads["pods"] = read
	data.Kubernetes.Reads["nodes"] = kubernetes.OverviewRead{Error: "forbidden"}
	s.Apply(data)
	row := s.Rows[3][0]
	if row.Status != "Partial" || !row.At.Equal(read.ReadAt) {
		t.Fatal("coverage or oldest read missing", row.Status, row.At)
	}
	if row.Counters[3].Value != "Unavailable" {
		t.Fatal("inaccessible nodes appeared as a count", row.Counters)
	}
}

func TestOverviewMetadataColumnsAndSpecificActions(t *testing.T) {
	s, _ := overviewDesignFixture()
	a := OverviewRow{Source: "Kubernetes", Action: "Rollout", Target: OverviewTarget{Namespace: "team"}}
	b := OverviewRow{Source: "Argo CD", Action: "Sync", Target: OverviewTarget{Namespace: "team"}}
	for _, width := range []int{40, 60, 90} {
		aa := ansi.Strip(s.deploymentMeta(a, width, "r24"))
		bb := ansi.Strip(s.deploymentMeta(b, width, "abc12"))
		if strings.Index(aa, "team") != strings.Index(bb, "team") || strings.Index(aa, "Rollout") != strings.Index(bb, "Sync") {
			t.Fatal("shifted metadata columns", aa, bb)
		}
	}
	s.Pane = 2
	if got := s.EnterLabel(); got != "3 affected resources" {
		t.Fatal(got)
	}
	s.Pane = 3
	s.Cursor[3] = 1
	s.Counter = 4
	if got := s.EnterLabel(); got != "Failed releases" {
		t.Fatal(got)
	}
}

func TestOverviewNewFailureCueNeedsComparableCoverage(t *testing.T) {
	s, now := overviewDesignFixture()
	data := s.Data
	old := data.Kubernetes.Attention[0]
	old.UID = "new-pod"
	old.Name = "new-failure"
	old.GroupKey = ""
	old.Severity = 2
	data.Kubernetes.Attention = append(append(data.Kubernetes.Attention[:0:0], data.Kubernetes.Attention...), old)
	data.Kubernetes.ReadAt = now.Add(15 * time.Second)
	s.Apply(data)
	total := 0
	for _, row := range s.Rows[2] {
		total += row.NewFailures
	}
	if total != 1 {
		t.Fatalf("new failures=%d", total)
	}
	s.Retained = true
	for _, row := range s.Rows[2] {
		if row.NewFailures > 0 && strings.Contains(ansi.Strip(strings.Join(s.rowLines(row, 2, 55, false, data.Kubernetes.ReadAt), "\n")), "NEW") {
			t.Fatal("retained read still suggests a fresh change")
		}
	}
}

func TestOverviewNewFailuresResetAcrossReadGapsAndPartialCoverage(t *testing.T) {
	for _, invalid := range []string{"gap", "partial", "missing source time"} {
		t.Run(invalid, func(t *testing.T) {
			s, now := overviewDesignFixture()
			data := s.Data
			data.Kubernetes.Reads = map[string]kubernetes.OverviewRead{}
			for key, read := range s.Data.Kubernetes.Reads {
				data.Kubernetes.Reads[key] = read
			}
			issue := data.Kubernetes.Attention[0]
			issue.UID = "new-error"
			issue.GroupKey = ""
			issue.Name = "new-error"
			data.Kubernetes.Attention = append(append(data.Kubernetes.Attention[:0:0], data.Kubernetes.Attention...), issue)
			data.Kubernetes.ReadAt = now.Add(15 * time.Second)
			switch invalid {
			case "gap":
				data.Kubernetes.ReadAt = now.Add(time.Minute)
			case "partial":
				r := data.Kubernetes.Reads["pods"]
				r.Partial = true
				data.Kubernetes.Reads["pods"] = r
			case "missing source time":
				r := data.Kubernetes.Reads["pods"]
				r.ReadAt = time.Time{}
				data.Kubernetes.Reads["pods"] = r
			}
			s.Apply(data)
			if !s.ComparedAt.IsZero() {
				t.Fatal("incomparable observations produced a comparison window")
			}
			for _, row := range s.Rows[2] {
				if row.NewFailures != 0 {
					t.Fatal("incomparable error labeled new", row.Title)
				}
			}
		})
	}
}
