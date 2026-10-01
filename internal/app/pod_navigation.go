package app

import (
	"slices"

	"github.com/simjay/plous/internal/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func preferredPodContainer(object unstructured.Unstructured) string {
	names := kubernetes.Containers(object)
	if len(names) == 0 {
		return ""
	}
	chosen, priority := names[0], 0
	for _, section := range []string{"initContainerStatuses", "containerStatuses", "ephemeralContainerStatuses"} {
		for _, state := range kubernetes.Maps(object.Object, "status", section) {
			name := kubernetes.Value(state, "name")
			if !slices.Contains(names, name) {
				continue
			}
			score := 0
			if reason := kubernetes.Value(state, "state", "waiting", "reason"); reason != "" {
				score = 3
				if reason == "ContainerCreating" || reason == "PodInitializing" {
					score = 2
				}
			}
			if reason := kubernetes.Value(state, "state", "terminated", "reason"); reason != "" && reason != "Completed" {
				score = 3
			}
			if code := kubernetes.Value(state, "state", "terminated", "exitCode"); code != "" && code != "0" {
				score = 3
			}
			if score == 0 && kubernetes.Value(state, "ready") != "true" && kubernetes.Value(state, "state", "terminated", "reason") != "Completed" {
				score = 1
			}
			if score > priority {
				chosen, priority = name, score
			}
		}
	}
	return chosen
}
