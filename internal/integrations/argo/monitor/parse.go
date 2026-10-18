package monitor

import (
	"fmt"
	"strings"
	"time"

	argocommon "github.com/simjay/plous/internal/integrations/argo/common"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func ParseApplication(object unstructured.Unstructured, readAt time.Time) gitops.Item {
	item := gitops.BaseItem(object, readAt)
	m := object.Object
	item.Sync = gitops.Fallback(gitops.Str(m, "status", "sync", "status"), "Not reported")
	item.Health = gitops.Fallback(gitops.Str(m, "status", "health", "status"), "Not reported")
	item.Status = item.Health
	item.Reason = gitops.Str(m, "status", "health", "message")
	item.ObservedAt = gitops.Timestamp(gitops.Str(m, "status", "reconciledAt"))
	item.ObservedLabel = "Argo reconciled"
	item.Destination = gitops.Destination{Name: gitops.Str(m, "spec", "destination", "name"), Server: gitops.SafeURL(gitops.Str(m, "spec", "destination", "server")), Namespace: gitops.Str(m, "spec", "destination", "namespace")}
	item.Conditions = gitops.ParseConditions(m, object.GetGeneration(), false)
	for _, condition := range item.Conditions {
		if strings.HasSuffix(condition.Type, "Error") {
			item.ControllerError = true
		}
	}
	if item.Reason == "" {
		for _, condition := range item.Conditions {
			if strings.HasSuffix(condition.Type, "Error") && condition.Message != "" {
				item.Reason = condition.Message
				break
			}
		}
	}
	resources, found, _ := unstructured.NestedSlice(m, "status", "resources")
	item.InventoryReported = found
	for _, value := range resources {
		resource, ok := value.(map[string]any)
		if !ok || gitops.Str(resource, "kind") == "" || gitops.Str(resource, "name") == "" {
			continue
		}
		ref := gitops.Reference{Group: gitops.Str(resource, "group"), Version: gitops.Str(resource, "version"), Kind: gitops.Str(resource, "kind"), Namespace: gitops.Str(resource, "namespace"), Name: gitops.Str(resource, "name")}
		managed := gitops.ManagedResource{Ref: ref, Sync: gitops.Fallback(gitops.Str(resource, "status"), "Not reported"), Health: gitops.Fallback(gitops.Str(resource, "health", "status"), "Not reported"), Message: gitops.Str(resource, "health", "message")}
		item.Resources = append(item.Resources, managed)
		if item.Reason == "" && (managed.Health == "Degraded" || managed.Health == "Missing") && managed.Message != "" {
			item.Reason = ref.Label() + ": " + managed.Message
		}
	}
	operationPhase := gitops.Str(m, "status", "operationState", "phase")
	if item.Reason == "" && (operationPhase == "Failed" || operationPhase == "Error") {
		item.Reason = gitops.Str(m, "status", "operationState", "message")
	}
	if item.Reason == "" {
		item.Reason = "Controller reports sync " + item.Sync + " and health " + item.Health + "."
	}
	spec, _, _ := unstructured.NestedMap(m, "spec")
	item.Sources = argocommon.Sources(spec)
	gitops.AddField(&item, "Project", gitops.Str(m, "spec", "project"))
	gitops.AddField(&item, "Compared revision", gitops.Str(m, "status", "sync", "revision"))
	for i, revision := range gitops.StringsAt(m, "status", "sync", "revisions") {
		gitops.AddField(&item, fmt.Sprintf("Compared revision %d", i+1), revision)
	}
	policy := "Manual"
	if automated, found, _ := unstructured.NestedMap(m, "spec", "syncPolicy", "automated"); found {
		policy = "Automated"
		if enabled, exists, _ := unstructured.NestedBool(automated, "enabled"); exists && !enabled {
			policy = "Manual (automated sync disabled)"
		}
	}
	gitops.AddField(&item, "Sync policy", policy)
	gitops.AddField(&item, "Latest operation", operationPhase)
	gitops.AddField(&item, "Operation message", gitops.Str(m, "status", "operationState", "message"))
	gitops.AddField(&item, "Operation started", gitops.Str(m, "status", "operationState", "startedAt"))
	gitops.AddField(&item, "Operation finished", gitops.Str(m, "status", "operationState", "finishedAt"))
	if operationPhase == "Succeeded" {
		gitops.AddField(&item, "Successful operation revision", gitops.Str(m, "status", "operationState", "syncResult", "revision"))
		for i, revision := range gitops.StringsAt(m, "status", "operationState", "syncResult", "revisions") {
			gitops.AddField(&item, fmt.Sprintf("Successful operation revision %d", i+1), revision)
		}
	}
	return item
}
