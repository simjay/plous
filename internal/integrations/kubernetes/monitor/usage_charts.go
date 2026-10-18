package monitor

import (
	"fmt"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// DetailBar carries quantities and their meaning, without terminal presentation.
type DetailBar struct {
	Label, Detail, Metric string
	Value, Total          *float64
}

// UsageBudgets compares only like scopes: one measured container or one Node.
// Workload templates are not limits for an aggregated set of running Pods.
func UsageBudgets(r kubecommon.Resource, object unstructured.Unstructured, usage Usage) []DetailSection {
	section := DetailSection{Title: "Latest usage against resource budgets"}
	for _, budget := range usageBudgets(r, object, usage) {
		label, format := "CPU", CPUUsage
		if budget.metric == "memory" {
			label, format = "Memory", MemoryUsage
		}
		if budget.subject != "" {
			label = budget.subject + " · " + label + " " + budget.basis
		}
		section.Bars = append(section.Bars, DetailBar{Label: label, Metric: budget.metric, Value: budget.used, Total: quantityFloat(budget.total), Detail: format(*budget.used) + " usage / " + quantityLabel(budget.total, "not specified") + " " + budget.basis})
	}
	if r.Key() == "pods" {
		section.Fields = []DetailField{{Label: "Basis", Value: "Container budgets from the Describe snapshot. Requests reserve resources; limits constrain them. Pod-level budgets and runtime allocation may differ."}}
	}

	if len(section.Bars) == 0 {
		return nil
	}
	if r.Key() == "nodes" {
		section.Fields = []DetailField{{Label: "Basis", Value: "Allocatable capacity from the Describe snapshot."}}
	}
	section.Fields = append(section.Fields, DetailField{Label: "Sampled", Value: usage.Timestamp.Local().Format("15:04:05")})
	return []DetailSection{section}
}

func StatusCharts(r kubecommon.Resource, object unstructured.Unstructured) []DetailSection {
	o := object.Object
	if r.Key() == "resourcequotas" {
		section := DetailSection{Title: "Quota utilization", Fields: []DetailField{{Label: "Basis", Value: "Resource and object quota accounting, compared with reported hard limits."}}}
		for _, budget := range quotaBudgets(o) {
			used := "not reported"
			if budget.used != nil {
				used = Value(o, "status", "used", budget.metric)
			}
			section.Bars = append(section.Bars, DetailBar{Label: budget.metric, Detail: used + " used / " + quantityLabel(budget.total, "not reported") + " hard", Value: budget.used, Total: quantityFloat(budget.total)})
		}
		if len(section.Bars) > 0 {
			return []DetailSection{section}
		}
		return nil
	}
	switch r.Key() {
	case "daemonsets.apps", "deployments.apps", "statefulsets.apps", "replicasets.apps", "replicationcontrollers":
	default:
		return nil
	}
	facts := workloadFacts(r.Key(), o)
	ready, current, unready := facts["ready_replicas"], facts["current_replicas"], facts["current_unready_replicas"]
	if ready.Number == nil || current.Number == nil || unready.Number == nil {
		return nil
	}
	section := DetailSection{Title: "Workload readiness", Bars: []DetailBar{{Label: "Ready / current", Detail: fmt.Sprintf("%d / %d · %d not ready · desired %s", *ready.Number, *current.Number, *unready.Number, facts["desired_replicas"].String()), Value: new(float64(*ready.Number)), Total: new(float64(*current.Number))}}, Fields: []DetailField{{Label: "Basis", Value: "Reported controller status; desired replicas are a separate target."}}}
	if observed := facts["generation_observed"]; observed.Boolean != nil && !*observed.Boolean {
		section.Fields = append(section.Fields, DetailField{Label: "Status", Value: "Controller has not observed the latest specification yet."})
	}
	return []DetailSection{section}
}

func ContainerUsageSections(usage Usage) []DetailSection {
	if len(usage.Breakdown) == 0 {
		return nil
	}
	section := DetailSection{Title: "Latest container breakdown", Fields: []DetailField{{Label: "Basis", Value: "Share of this Pod's usage at " + usage.Timestamp.Local().Format("15:04:05")}}}
	for _, c := range usage.Breakdown {
		section.Bars = append(section.Bars,
			DetailBar{Label: c.Name + " · CPU", Detail: CPUUsage(c.CPU), Metric: "cpu", Value: new(c.CPU), Total: new(usage.CPU)},
			DetailBar{Label: c.Name + " · Memory", Detail: MemoryUsage(c.Memory), Metric: "memory", Value: new(c.Memory), Total: new(usage.Memory)})
	}
	return []DetailSection{section}
}
