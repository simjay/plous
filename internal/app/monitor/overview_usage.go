package monitor

import (
	"strings"
	"time"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
)

func (s *OverviewState) ApplyUsage(usage kubernetes.OverviewUsage) {
	if s.Usage != nil && !usage.ReadAt.After(s.Usage.ReadAt) {
		return
	}
	s.Usage = &usage
	usage.Apply(&s.Data.Kubernetes)
	s.captureUsage()
	s.updateCoverage()
	s.Cursor[3] = min(s.Cursor[3], max(0, len(s.Filtered(3))-1))
}

func (s *OverviewState) captureUsage() {
	k := s.Data.Kubernetes
	readAt := k.MetricsReadAt
	if readAt.IsZero() {
		readAt = k.ReadAt
	}
	for i, metric := range []kubernetes.OverviewMetric{k.CPU, k.Memory} {
		if metric.Used == nil || metric.Allocatable == nil || *metric.Allocatable <= 0 || k.MetricsAt.IsZero() || readAt.Sub(k.MetricsAt) > kubernetes.UsageMaxAge {
			continue
		}
		points := s.Samples[i]
		if len(points) == 0 || k.MetricsAt.After(points[len(points)-1].Time) {
			points = append(points, ui.ChartSample{Time: k.MetricsAt, Value: 100 * *metric.Used / *metric.Allocatable})
		}
		for len(points) > 0 && (len(points) > 180 || readAt.Sub(points[0].Time) > 15*time.Minute) {
			points = points[1:]
		}
		s.Samples[i] = points
		s.Scales[i] = max(s.Scales[i], ui.ChartMaximum(100**metric.Used / *metric.Allocatable))
	}
}

func (s *OverviewState) notices() []string {
	notices := append([]string(nil), s.Data.Notices...)
	if issue := s.Data.Kubernetes.MetricsError; issue != "" {
		for _, note := range notices {
			if note == issue {
				return notices
			}
		}
		notices = append(notices, issue)
	}
	return notices
}

func (s *OverviewState) updateCoverage() {
	for i, row := range s.Rows[3] {
		if row.ID == "coverage" {
			s.Rows[3] = append(s.Rows[3][:i], s.Rows[3][i+1:]...)
			break
		}
	}
	if notices := s.notices(); len(notices) > 0 {
		s.Rows[3] = append(s.Rows[3], OverviewRow{ID: "coverage", Title: "Availability", Detail: strings.Join(notices, "\n"), Source: "Availability", At: s.Data.Kubernetes.ReadAt})
	}
}
