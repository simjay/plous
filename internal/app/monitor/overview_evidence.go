package monitor

import (
	"fmt"
	"strings"
	"time"
)

func (s *OverviewState) SelectedEvidence() string {
	rows := s.Filtered(s.Pane)
	if len(rows) == 0 {
		return s.Evidence()
	}
	row := rows[min(s.Cursor[s.Pane], len(rows)-1)]
	if s.Pane == 1 && row.Historical {
		return overviewChangeEvidence(row)
	}
	return strings.Join([]string{"Overview resource evidence", row.Title, "Source: " + row.Source, "Namespace: " + row.Target.Namespace, "Status: " + row.Status, row.Detail, "Source time: " + overviewEvidenceTime(row.At), "Observed: " + overviewEvidenceTime(s.Data.Kubernetes.ReadAt), "", s.Evidence()}, "\n")
}
func (s *OverviewState) Evidence() string {
	k := s.Data.Kubernetes
	lines := []string{"Overview evidence", "Scope: selected cluster, all namespaces. Counts cover readable objects only.", "Collected: " + overviewEvidenceTime(k.ReadAt), "", "CPU and memory usage: metrics.k8s.io Node observations.", "Metrics fetched: " + overviewEvidenceTime(k.MetricsReadAt), fmt.Sprintf("Coverage: %d/%d observed Nodes", k.MetricsObserved, k.MetricsExpected), "Source time range: " + overviewEvidenceTime(k.MetricsAt) + " to " + overviewEvidenceTime(k.MetricsEnd), k.MetricsError, "", "Requests and limits: declared active Pod specifications, including unscheduled Pods.", fmt.Sprintf("Active Pods: %d. CPU requests missing: %d, limits missing: %d. Memory requests missing: %d, limits missing: %d.", k.ActivePods, k.CPU.RequestsMissing, k.CPU.LimitsMissing, k.Memory.RequestsMissing, k.Memory.LimitsMissing), "Configured limits are not a finite cluster ceiling. Unspecified container limits may be unbounded.", ""}
	if d := s.Delta; d.Restarts != nil {
		lines = append(lines, fmt.Sprintf("Restart change: %+d, between %s and %s. Same observed Pod UIDs and containers; complete reads.", *d.Restarts, overviewEvidenceTime(d.RestartsSince), overviewEvidenceTime(d.RestartsUntil)))
	}
	if d := s.Delta; d.Replicas != nil {
		lines = append(lines, fmt.Sprintf("HPA current replica change: %+d, between %s and %s. Same observed HPA UIDs; complete reads.", *d.Replicas, overviewEvidenceTime(d.ReplicasSince), overviewEvidenceTime(d.ReplicasUntil)))
	}
	if !s.ComparedAt.IsZero() {
		lines = append(lines, "NEW errors compare complete readable observations from "+overviewEvidenceTime(s.ComparedSince)+" to "+overviewEvidenceTime(s.ComparedAt)+". They are newly observed here, not inferred incident start times.")
	}
	for _, tile := range s.tiles(time.Now()) {
		lines = append(lines, tile.title+": "+tile.value, strings.Join(tile.lines, "\n"), "")
	}
	for _, key := range []string{"nodes", "pods", "deployments.apps", "statefulsets.apps", "daemonsets.apps", "horizontalpodautoscalers.autoscaling"} {
		read := k.Reads[key]
		lines = append(lines, fmt.Sprintf("%s: %d objects, partial=%t, collected %s. %s", key, read.Count, read.Partial, overviewEvidenceTime(read.ReadAt), read.Error))
	}
	lines = append(lines, "", "Up to 200 reported controller changes and storage revisions are retained in this session. This is not a deployment audit log. Missing timestamps are not invented.")
	for _, row := range s.Rows[3] {
		if row.Target.Tool != "" {
			lines = append(lines, row.Title+": "+row.Detail+" · "+row.Status+" · collected "+overviewEvidenceTime(row.At))
		}
	}
	lines = append(lines, s.notices()...)
	if s.Error != "" {
		lines = append(lines, s.Error)
	}
	return strings.Join(lines, "\n")
}

func overviewEvidenceTime(at time.Time) string {
	if at.IsZero() {
		return "Not reported"
	}
	return at.UTC().Format(time.RFC3339)
}
