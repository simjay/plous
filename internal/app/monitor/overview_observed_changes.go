package monitor

import (
	"fmt"
	"strings"
	"time"
)

func overviewFailureIDs(rows []OverviewRow) map[string]bool {
	ids := map[string]bool{}
	for _, row := range rows {
		if len(row.Members) > 0 {
			for id := range overviewFailureIDs(row.Members) {
				ids[id] = true
			}
			continue
		}
		if row.Severity != 2 {
			continue
		}
		target := row.Target
		if target.UID != "" {
			ids[target.Tool+"/"+target.UID] = true
		} else if target.Tool == "helm" {
			ids["helm/"+target.Namespace+"/"+target.Name] = true
		}
	}
	return ids
}

func overviewSourceSet(data OverviewData) string {
	sources := []string{}
	if data.Helm != nil {
		sources = append(sources, "helm")
	}
	if data.Argo != nil {
		sources = append(sources, "argo")
	}
	sources = append(sources, fmt.Sprintf("flux/%d", len(data.Flux)))
	return strings.Join(sources, ",")
}

func (s *OverviewState) healthComplete() bool {
	if s.partial() {
		return false
	}
	for _, key := range []string{"nodes", "pods", "deployments.apps", "statefulsets.apps", "daemonsets.apps", "horizontalpodautoscalers.autoscaling"} {
		at := s.Data.Kubernetes.Reads[key].ReadAt
		if at.IsZero() || s.Data.Kubernetes.ReadAt.Sub(at) > 45*time.Second {
			return false
		}
	}
	current := func(at time.Time) bool { return !at.IsZero() && s.Data.Kubernetes.ReadAt.Sub(at) <= 45*time.Second }
	if s.Data.Helm != nil && !current(s.Data.Helm.ReadAt) {
		return false
	}
	if s.Data.Argo != nil && !current(s.Data.Argo.ReadAt) {
		return false
	}
	for _, snapshot := range s.Data.Flux {
		if !current(snapshot.ReadAt) {
			return false
		}
	}
	return true
}

func overviewInterval(since, until time.Time) string {
	if until.Sub(since) < time.Second {
		return "<1s"
	}
	return fmt.Sprintf("%.0fs", until.Sub(since).Seconds())
}

func overviewDeltaCurrent(until, now time.Time) bool {
	return !until.IsZero() && !now.Before(until) && now.Sub(until) <= 45*time.Second
}
