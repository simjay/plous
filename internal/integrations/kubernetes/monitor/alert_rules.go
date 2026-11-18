package monitor

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type AlertRule struct {
	Name   string
	Checks int
}

type AlertSignal struct {
	Rule               AlertRule
	Container, Detail  string
	Bad, ZeroAvailable bool
}

// Names are persisted in alert records. Checks applies to opening and recovery.
var (
	stalledRollout     = AlertRule{"stalled rollout", 1}
	lowAvailability    = AlertRule{"low availability", 2}
	containerCrashLoop = AlertRule{"container crash loop", 2}
	noReadyEndpoints   = AlertRule{"no ready endpoints", 2}
)

func evaluateAlertRules(check *CheckResult, obj *ResourceCheck, resource unstructured.Unstructured, endpoints EndpointReadiness, pods []unstructured.Unstructured, podsComplete bool) {
	key := obj.Kind + "/" + obj.Name
	if obj.Kind == "Service" {
		if !endpoints.ObservedAt.IsZero() && endpoints.State == "" && !endpoints.Partial && endpoints.Unknown == 0 {
			obj.Signals = append(obj.Signals, AlertSignal{Rule: noReadyEndpoints, Bad: endpoints.Ready == 0, Detail: fmt.Sprintf("%d ready endpoints", endpoints.Ready)})
		} else {
			check.Coverage[key] = "EndpointSlice readiness is incomplete or unavailable."
		}
		return
	}
	workloadAlertSignals(check, obj, resource)
	if obj.Inactive != "" || !podsComplete {
		return
	}
	signals, partial := crashLoopSignals(pods)
	obj.Signals = append(obj.Signals, signals...)
	if partial {
		check.Coverage[key+" Pods"] = "Some active container states are unavailable."
	} else {
		obj.CompleteRules = append(obj.CompleteRules, containerCrashLoop)
	}
}

func workloadAlertSignals(check *CheckResult, obj *ResourceCheck, o unstructured.Unstructured) {
	counts := readWorkload(workloadFields(checkResources[obj.Kind].Key()), o.Object)
	desired := counts.desired
	if obj.Kind == "Deployment" && desired == nil {
		defaultReplicas := int64(1)
		desired = &defaultReplicas
	}
	if obj.Kind == "Deployment" && desired != nil && *desired == 0 {
		obj.Inactive = "scaled down"
		return
	}
	generation := counts.generationObserved()
	if desired == nil || generation == nil || !*generation {
		check.Coverage[obj.Kind+"/"+obj.Name] = "Current controller status is unavailable."
		return
	}
	if obj.Kind == "DaemonSet" {
		if *desired == 0 {
			obj.Inactive = "no eligible nodes"
			return
		}
		available, updated := counts.available, counts.updated
		if available == nil || updated == nil {
			check.Coverage[obj.Kind+"/"+obj.Name] = "DaemonSet availability or update counts are unavailable."
			return
		}
		budget := 0
		if *updated < *desired && Value(o.Object, "spec", "updateStrategy", "type") != "OnDelete" {
			value := Value(o.Object, "spec", "updateStrategy", "rollingUpdate", "maxUnavailable")
			if value == "" {
				value = "1"
			}
			var err error
			maxUnavailable := intstr.Parse(value)
			budget, err = intstr.GetScaledValueFromIntOrPercent(&maxUnavailable, int(*desired), true)
			if err != nil {
				check.Coverage[obj.Kind+"/"+obj.Name] = "DaemonSet update budget is unavailable."
				return
			}
		}
		obj.Signals = append(obj.Signals, AlertSignal{Rule: lowAvailability, Bad: *desired-*available > int64(budget), ZeroAvailable: *available == 0, Detail: fmt.Sprintf("%d of %d eligible nodes available (%d updated)", *available, *desired, *updated)})
		return
	}
	paused, _, _ := unstructured.NestedBool(o.Object, "spec", "paused")
	availabilityKnown, progressKnown := false, paused
	for _, condition := range Maps(o.Object, "status", "conditions") {
		status, reason := Value(condition, "status"), Value(condition, "reason")
		switch Value(condition, "type") {
		case "Available":
			if status == "True" || status == "False" {
				availabilityKnown = true
				n := counts.available
				detail := reason
				if n != nil {
					detail = fmt.Sprintf("%d of %d desired replicas available (%s)", *n, *desired, reason)
				}
				obj.Signals = append(obj.Signals, AlertSignal{Rule: lowAvailability, Bad: status == "False", ZeroAvailable: n != nil && *n == 0, Detail: detail})
			}
		case "Progressing":
			progressKnown = paused || status == "True" || status == "False"
			if !paused && (status == "False" && reason == "ProgressDeadlineExceeded" || status == "True" && reason == "NewReplicaSetAvailable") {
				obj.Signals = append(obj.Signals, AlertSignal{Rule: stalledRollout, Bad: status == "False", Detail: reason})
			}
		}
	}
	if !availabilityKnown || !progressKnown {
		check.Coverage[obj.Kind+"/"+obj.Name] = "One or more Deployment conditions are unavailable."
	}
}

func crashLoopSignals(pods []unstructured.Unstructured) ([]AlertSignal, bool) {
	byContainer := map[string]AlertSignal{}
	missing := map[string]bool{}
	for _, pod := range pods {
		for _, field := range []struct{ spec, status string }{{"initContainers", "initContainerStatuses"}, {"containers", "containerStatuses"}} {
			observed := map[string]bool{}
			for _, status := range Maps(pod.Object, "status", field.status) {
				name := Value(status, "name")
				state, _, _ := unstructured.NestedMap(status, "state")
				if len(state) == 0 {
					continue
				}
				observed[name] = true
				if name == "" || byContainer[name].Bad {
					continue
				}
				signal := AlertSignal{Rule: containerCrashLoop, Container: name}
				if Value(status, "state", "waiting", "reason") == "CrashLoopBackOff" {
					signal.Bad = true
					signal.Detail = "CrashLoopBackOff in Pod " + pod.GetName()
					if node := Value(pod.Object, "spec", "nodeName"); node != "" {
						signal.Detail += " on " + node
					}
				}
				byContainer[name] = signal
			}
			for _, container := range Maps(pod.Object, "spec", field.spec) {
				if name := Value(container, "name"); name != "" && !observed[name] {
					missing[name] = true
				}
			}
		}
	}
	signals := make([]AlertSignal, 0, len(byContainer))
	for name, signal := range byContainer {
		if signal.Bad || !missing[name] {
			signals = append(signals, signal)
		}
	}
	return signals, len(missing) != 0
}
