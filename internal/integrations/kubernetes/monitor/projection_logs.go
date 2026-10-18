package monitor

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
)

func (e *Projector) workloadLogSnapshot(ctx context.Context, c Command, result observation.Result) (observation.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resource, err := e.discoverResource(ctx, c.Resource)
	if err != nil {
		return result, err
	}
	if !HasPodTargets(resource) || resource.Version != "v1" {
		return result, &observation.ReadFailure{Message: "Workload logs require a supported workload API"}
	}
	reader := &projectedObjectReader{projector: e}
	object, err := reader.Get(ctx, resource, c.Namespace, c.Name)
	if err != nil {
		return result, err
	}
	resources := []kubecommon.Resource{resource}
	keys := []string{"pods"}
	if resource.Key() == "deployments.apps" {
		keys = append(keys, "replicasets.apps")
	}
	if resource.Key() == "cronjobs.batch" {
		keys = append(keys, "jobs.batch")
	}
	for _, key := range keys {
		r, err := e.discoverResource(ctx, key)
		if err != nil {
			return result, err
		}
		resources = append(resources, r)
	}
	pods, err := podTargets(ctx, reader, resource, object, resources)
	if err != nil {
		return result, err
	}
	result.Schema = "kubernetes.workload_logs.v1"
	result.Selection["uid"], result.Selection["container"], result.Selection["previous"] = string(object.GetUID()), c.Container, fmt.Sprint(c.Previous)
	result.Facts["owned_pods"] = observation.Number(int64(len(pods)))
	result.Partial("selected log excerpts, not complete log history")
	podResource := resources[1]
	remaining, observed, failed := c.MaxBytes, 0, 0
	for i, pod := range pods {
		if i == MaxAgentLogSources || remaining == 0 {
			break
		}
		row := observation.Row{Identity: usageIdentity(podResource, pod), Facts: map[string]observation.Value{
			"container": observation.Text(c.Container), "previous": observation.Boolean(c.Previous),
		}}
		current, readErr := reader.Get(ctx, podResource, c.Namespace, pod.GetName())
		if readErr == nil && (pod.GetUID() == "" || current.GetUID() != pod.GetUID()) {
			readErr = &observation.ReadFailure{Message: "Pod was replaced before log collection"}
		}
		if readErr == nil && !slices.Contains(Containers(*current), c.Container) {
			readErr = &observation.ReadFailure{Message: "Selected container is absent from this Pod"}
		}
		var excerpt observation.Result
		if readErr == nil {
			q := c
			q.Resource, q.Name, q.MaxBytes = "pods", pod.GetName(), remaining
			excerpt = observation.New("kubernetes/pods", "logs", result.ObservedAt)
			excerpt.Selection = map[string]string{}
			excerpt, readErr = e.logs(ctx, q, excerpt)
			if readErr == nil {
				text := excerpt.Facts["excerpt"]
				remaining -= len(*text.Text)
				current, readErr = reader.Get(ctx, podResource, c.Namespace, pod.GetName())
				if readErr == nil && current.GetUID() != pod.GetUID() {
					readErr = &observation.ReadFailure{Message: "Pod was replaced during log collection"}
				}
			}
		}
		if readErr != nil {
			var failure *observation.ReadFailure
			if ctx.Err() != nil || !errors.As(readErr, &failure) {
				return result, readErr
			}
			row.Facts["read_status"], row.Facts["error"] = observation.Text("failed"), observation.Text(e.sanitizeText(failure.Error()))
			failed++
		} else {
			row.Facts["read_status"], row.Facts["excerpt"] = observation.Text("completed"), excerpt.Facts["excerpt"]
			row.Facts["tail_lines"] = excerpt.Facts["tail_lines"]
			row.Facts["truncated"] = observation.Boolean(len(excerpt.Coverage.Limitations) > 1)
			observed++
		}
		result.Rows = append(result.Rows, row)
	}
	result.Coverage.Observed = len(result.Rows)
	result.Facts["sources_observed"], result.Facts["sources_failed"] = observation.Number(int64(observed)), observation.Number(int64(failed))
	skipped := len(pods) - len(result.Rows)
	result.Facts["sources_skipped"], result.Facts["sources_complete"] = observation.Number(int64(skipped)), observation.Boolean(failed == 0 && skipped == 0)
	if failed > 0 {
		result.Partial("some Pod logs were unavailable")
	}
	if skipped > 0 {
		result.Partial("Pod or combined log byte limit reached")
	}
	return result, result.Validate()
}
