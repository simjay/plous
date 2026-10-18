package monitor

import (
	"fmt"
	"strings"
	"time"

	fluxcommon "github.com/simjay/plous/internal/integrations/flux/common"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func ParseFlux(object unstructured.Unstructured, readAt time.Time) gitops.Item {
	item := gitops.BaseItem(object, readAt)
	m := object.Object
	item.Conditions = gitops.ParseConditions(m, object.GetGeneration(), true)
	item.Suspended, _, _ = unstructured.NestedBool(m, "spec", "suspend")
	item.ObservedLabel = "Condition changed"
	item.Status, item.Reason = "Unknown", "The controller has not reported a current Ready condition."
	var chosen *gitops.Condition
	for i := range item.Conditions {
		condition := &item.Conditions[i]
		if conditionPriority(*condition) > 0 && (chosen == nil || conditionPriority(*condition) > conditionPriority(*chosen)) {
			chosen = condition
		}
	}
	if chosen != nil {
		item.ObservedAt = chosen.ChangedAt
		item.Stale = chosen.Stale
		item.Reason = gitops.Fallback(chosen.Message, gitops.Fallback(chosen.Reason, item.Reason))
		switch {
		case chosen.Stale:
			item.Status = "Awaiting status"
			item.Reason = fmt.Sprintf("Waiting for the controller to report generation %d. Previous %s: %s", object.GetGeneration(), chosen.Type, item.Reason)
		case chosen.Type == "Stalled":
			item.Status = "Stalled"
		case chosen.Type == "Reconciling":
			item.Status = "Reconciling"
		case chosen.Status == "True":
			item.Status = "Ready"
		case chosen.Status == "False":
			item.Status = "Not ready"
		}
	}
	if item.Suspended {
		item.Status = "Suspended"
		item.Reason = "Reconciliation is suspended. Previous observation: " + item.Reason
	}
	item.Destination.Namespace = gitops.Str(m, "spec", "targetNamespace")
	if item.Ref.Kind == "HelmRelease" && item.Destination.Namespace == "" {
		item.Destination.Namespace = item.Ref.Namespace
	}
	_, item.Destination.Remote, _ = unstructured.NestedMap(m, "spec", "kubeConfig")
	for _, dependency := range gitops.Maps(m, "spec", "dependsOn") {
		if name := gitops.Str(dependency, "name"); name != "" {
			item.Dependencies = append(item.Dependencies, gitops.Reference{Group: item.Ref.Group, Version: item.Ref.Version, Kind: item.Ref.Kind, Namespace: gitops.Fallback(gitops.Str(dependency, "namespace"), item.Ref.Namespace), Name: name})
		}
	}
	parseFluxSources(&item, m)
	parseInventory(&item, m)
	gitops.AddField(&item, "Interval", gitops.Str(m, "spec", "interval"))
	gitops.AddField(&item, "Path", gitops.Str(m, "spec", "path"))
	gitops.AddField(&item, "Target namespace", item.Destination.Namespace)
	gitops.AddField(&item, "Last applied revision", gitops.Str(m, "status", "lastAppliedRevision"))
	if item.Ref.Kind == "HelmRelease" {
		gitops.AddField(&item, "Last attempted chart revision", gitops.Str(m, "status", "lastAttemptedRevision"))
		gitops.AddField(&item, "Last attempted action", gitops.Str(m, "status", "lastAttemptedReleaseAction"))
		gitops.AddField(&item, "Release name", gitops.Str(m, "spec", "releaseName"))
		gitops.AddField(&item, "Storage namespace", gitops.Fallback(gitops.Str(m, "status", "storageNamespace"), gitops.Str(m, "spec", "storageNamespace")))
		gitops.AddField(&item, "Chart", gitops.Str(m, "spec", "chart", "spec", "chart"))
		gitops.AddField(&item, "Chart version constraint", gitops.Str(m, "spec", "chart", "spec", "version"))
		if history := gitops.Maps(m, "status", "history"); len(history) > 0 {
			gitops.AddField(&item, "Latest release status", gitops.Str(history[0], "status"))
			gitops.AddField(&item, "Latest release chart version", gitops.Str(history[0], "chartVersion"))
			gitops.AddField(&item, "Latest release name", gitops.Str(history[0], "name"))
		}
	} else {
		gitops.AddField(&item, "Last attempted revision", gitops.Str(m, "status", "lastAttemptedRevision"))
	}
	if item.Ref.Group == "source.toolkit.fluxcd.io" {
		gitops.AddField(&item, "URL", gitops.SafeURL(gitops.Str(m, "spec", "url")))
		gitops.AddField(&item, "Provider", gitops.Str(m, "spec", "provider"))
		gitops.AddField(&item, "Bucket", gitops.Str(m, "spec", "bucketName"))
		for _, key := range []string{"branch", "tag", "semver", "commit", "name"} {
			gitops.AddField(&item, "Requested "+key, gitops.Str(m, "spec", "ref", key))
		}
		gitops.AddField(&item, "Fetched revision", gitops.Str(m, "status", "artifact", "revision"))
		gitops.AddField(&item, "Artifact updated", gitops.Str(m, "status", "artifact", "lastUpdateTime"))
		gitops.AddField(&item, "Artifact digest", gitops.Str(m, "status", "artifact", "digest"))
		gitops.AddField(&item, "Chart", gitops.Str(m, "spec", "chart"))
		gitops.AddField(&item, "Chart version constraint", gitops.Str(m, "spec", "version"))
	}
	return item
}

func conditionPriority(condition gitops.Condition) int {
	priority := 0
	switch {
	case condition.Type == "Stalled" && condition.Status == "True":
		priority = 4
	case condition.Type == "Ready" && condition.Status == "False":
		priority = 3
	case condition.Type == "Reconciling" && condition.Status == "True":
		priority = 2
	case condition.Type == "Ready":
		priority = 1
	}
	if priority > 0 && !condition.Stale {
		priority += 10
	}
	return priority
}

func parseFluxSources(item *gitops.Item, m map[string]any) {
	item.Sources = fluxcommon.SourceReferences(m, item.Ref.Namespace)
	seen := map[string]bool{}
	for _, source := range item.Sources {
		seen[source.Ref.Key()] = true
	}
	if chart := gitops.Str(m, "status", "helmChart"); chart != "" {
		namespace, name, ok := strings.Cut(chart, "/")
		if ok && namespace != "" && name != "" {
			ref := gitops.Reference{Group: "source.toolkit.fluxcd.io", Kind: "HelmChart", Namespace: namespace, Name: name}
			if !seen[ref.Key()] {
				item.Sources = append(item.Sources, gitops.SourceReference{Ref: ref})
			}
		}
	}
}

func parseInventory(item *gitops.Item, m map[string]any) {
	entries, found, _ := unstructured.NestedSlice(m, "status", "inventory", "entries")
	item.InventoryReported = found
	invalid := 0
	for _, value := range entries {
		entry, ok := value.(map[string]any)
		if !ok {
			invalid++
			continue
		}
		parts := strings.Split(gitops.Str(entry, "id"), "_")
		if len(parts) != 4 || parts[1] == "" || parts[3] == "" {
			invalid++
			continue
		}
		item.Resources = append(item.Resources, gitops.ManagedResource{Ref: gitops.Reference{Namespace: parts[0], Name: parts[1], Group: parts[2], Kind: parts[3], Version: gitops.Str(entry, "v")}, Health: "Not reported"})
	}
	if invalid > 0 {
		gitops.AddField(item, "Inventory note", fmt.Sprintf("%d inventory entries could not be interpreted.", invalid))
	}
}
