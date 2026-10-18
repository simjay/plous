package monitor

import (
	"context"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
)

const UsageInterval = 15 * time.Second

const UsageHistory = 15 * time.Minute

type ExploreUsage struct {
	Key     string
	Ctx     context.Context
	Cancel  context.CancelFunc
	Events  chan ExploreUsageResult
	Samples map[string]kubernetes.Usage
	Latest  kubernetes.Usage
	Points  []kubernetes.Usage
	Err     string
	Note    string
	Loaded  bool
	Charts  UsageCharts
}

type ExploreUsageResult struct {
	Run     *ExploreUsage
	Samples map[string]kubernetes.Usage
	Usage   kubernetes.Usage
	Note    string
	Err     error
	Done    bool
}

func WaitExploreUsage(run *ExploreUsage) tea.Cmd {
	return func() tea.Msg {
		select {
		case result, ok := <-run.Events:
			if ok {
				return result
			}
		case <-run.Ctx.Done():
		}
		return ExploreUsageResult{Run: run, Done: true}
	}
}

// UsageCharts is transient and belongs to the same target as the sampling run.
type UsageCharts struct {
	Inspect  bool
	Scroll   int
	Window   time.Duration
	Selected time.Time
	Notice   string
	Maximum  [2]float64
	ScaledAt time.Time
}

func (c *UsageCharts) Range() time.Duration {
	if c.Window == 5*time.Minute {
		return c.Window
	}
	return UsageHistory
}

func (r *ExploreUsage) VisiblePoints(now time.Time) []kubernetes.Usage {
	var points []kubernetes.Usage
	for _, p := range r.Points {
		if !usagePlotTime(p).Before(now.Add(-r.Charts.Range())) && !usagePlotTime(p).After(now) {
			points = append(points, p)
		}
	}
	return points
}

func (r *ExploreUsage) ReconcileCharts(now time.Time) {
	points := r.VisiblePoints(now)
	found := r.Charts.Selected.IsZero()
	peaks := [2]float64{}
	for _, p := range points {
		found = found || usagePlotTime(p).Equal(r.Charts.Selected)
		peaks[0], peaks[1] = max(peaks[0], p.CPU), max(peaks[1], p.Memory)
	}
	if !found {
		r.Charts.Selected = time.Time{}
		r.Charts.Notice = "Selected sample left this range; following latest."
	}
	shrink := r.Charts.Selected.IsZero() && now.Sub(r.Charts.ScaledAt) >= time.Minute
	for i, p := range peaks {
		unit := 1.0
		if i == 1 {
			unit = 1024 * 1024
			if p >= 1024*1024*1024 {
				unit = 1024 * 1024 * 1024
			}
		}
		next := ui.ChartMaximum(p/unit) * unit
		if p == 0 && i == 0 {
			next = .001
		}
		if r.Charts.Maximum[i] == 0 || p > r.Charts.Maximum[i] || shrink && next < r.Charts.Maximum[i] {
			r.Charts.Maximum[i] = next
		}
	}
	if shrink || r.Charts.ScaledAt.IsZero() {
		r.Charts.ScaledAt = now
	}
}

func (r *ExploreUsage) SelectSample(at, now time.Time) bool {
	for _, p := range r.VisiblePoints(now) {
		if usagePlotTime(p).Equal(at) {
			r.Charts.Selected, r.Charts.Notice = at, ""
			return true
		}
	}
	return false
}

func (r *ExploreUsage) MoveSample(step int, now time.Time) {
	points := r.VisiblePoints(now)
	if len(points) == 0 {
		r.Charts.Notice = "No samples in this range yet."
		return
	}
	index := len(points) - 1
	for i, p := range points {
		if usagePlotTime(p).Equal(r.Charts.Selected) {
			index = i
			break
		}
	}
	r.SelectSample(usagePlotTime(points[min(len(points)-1, max(0, index+step))]), now)
}

func (r *ExploreUsage) Apply(result ExploreUsageResult, now time.Time) {
	r.Loaded, r.Err, r.Note = true, kubernetes.UsageErrorText(result.Err), result.Note
	if result.Usage.Aggregated {
		r.Note = fmt.Sprintf("Metrics from %d/%d active Pods", result.Usage.Coverage.Observed, result.Usage.Expected)
	}
	r.Samples = result.Samples
	if result.Err == nil && !result.Usage.Timestamp.IsZero() {
		sample := result.Usage
		if len(r.Points) > 0 {
			previous := r.Points[len(r.Points)-1]
			if sample.Snapshot != "" && sample.Snapshot == previous.Snapshot || !usagePlotTime(sample).After(usagePlotTime(previous)) {
				sample = previous
			}
		}
		r.Latest = sample
		if len(r.Points) == 0 || usagePlotTime(sample).After(usagePlotTime(r.Points[len(r.Points)-1])) {
			r.Points = append(r.Points, sample)
		}
	}
	cutoff := now.Add(-UsageHistory)
	r.Points = slices.DeleteFunc(r.Points, func(u kubernetes.Usage) bool { return usagePlotTime(u).Before(cutoff) })
	if len(r.Points) > 120 {
		r.Points = slices.Clone(r.Points[len(r.Points)-120:])
	}
	r.ReconcileCharts(now)
}

func (r *ExploreUsage) ChartCommand(command string, detail bool, now time.Time) string {
	switch command {
	case "charts":
		r.Charts.Inspect = !r.Charts.Inspect
		r.Charts.Scroll = 0
	case "chartrange":
		if !detail {
			return "List charts show current samples only."
		}
		if r.Charts.Range() == UsageHistory {
			r.Charts.Window = 5 * time.Minute
		} else {
			r.Charts.Window = UsageHistory
		}
	case "chartlatest":
		r.Charts.Selected = time.Time{}
		r.Charts.Notice = ""
		r.Charts.Scroll = 0
	}
	r.ReconcileCharts(now)
	return ""
}

// Global commands and panel navigation are handled before these local keys.
func (r *ExploreUsage) ChartKey(key string, detail bool, page, limit int, now time.Time) {
	r.Charts.Scroll = min(r.Charts.Scroll, limit)
	switch key {
	case "esc", "I":
		r.ChartCommand("charts", detail, now)
	case "W":
		r.ChartCommand("chartrange", detail, now)
	case "P":
		r.ChartCommand("chartlatest", detail, now)
	case "left", "right":
		if detail {
			step := -1
			if key == "right" {
				step = 1
			}
			r.MoveSample(step, now)
			r.Charts.Scroll = 0
		}
	case "up", "k":
		r.Charts.Scroll = max(0, r.Charts.Scroll-1)
	case "down", "j":
		r.Charts.Scroll = min(limit, r.Charts.Scroll+1)
	case "pgup":
		r.Charts.Scroll = max(0, r.Charts.Scroll-max(1, page))
	case "pgdown":
		r.Charts.Scroll = min(limit, r.Charts.Scroll+max(1, page))
	case "home":
		r.Charts.Scroll = 0
	case "end":
		r.Charts.Scroll = limit
	}
}

func usagePlotTime(u kubernetes.Usage) time.Time {
	if u.Aggregated {
		return u.Collected
	}
	return u.Timestamp
}
