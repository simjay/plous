package monitor

import (
	"fmt"
	"slices"
	"strings"
	"time"

	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Parse(object unstructured.Unstructured, readAt time.Time) gitops.Item {
	switch object.GetKind() {
	case "ApplicationSet":
		return parseApplicationSet(object, readAt)
	case "AppProject":
		return parseProject(object, readAt)
	default:
		return ParseApplication(object, readAt)
	}
}

func parseApplicationSet(object unstructured.Unstructured, readAt time.Time) gitops.Item {
	item := gitops.BaseItem(object, readAt)
	item.Status, item.Reason = "Not reported", "The controller has not reported an ApplicationSet condition."
	item.ObservedLabel = "Condition changed"
	item.Conditions = gitops.ParseConditions(object.Object, object.GetGeneration(), false)
	priority := 0
	for _, condition := range item.Conditions {
		rank, status := 0, ""
		switch {
		case condition.Type == "ErrorOccurred" && condition.Status == "True":
			rank, status = 3, "Failed"
		case condition.Type == "ResourcesUpToDate" && condition.Status == "True":
			rank, status = 2, "Up to date"
		case condition.Type == "ResourcesUpToDate" && condition.Status == "False":
			rank, status = 2, "Reconciling"
		case condition.Type == "ParametersGenerated" && condition.Status == "True":
			rank, status = 1, "Generated"
		}
		if rank > priority {
			priority = rank
			item.Status, item.Reason = status, gitops.Fallback(condition.Message, condition.Reason)
			item.ObservedAt = condition.ChangedAt
		}
	}
	var generators []string
	for _, generator := range gitops.Maps(object.Object, "spec", "generators") {
		for _, kind := range []string{"list", "clusters", "git", "matrix", "merge", "scmProvider", "clusterDecisionResource", "pullRequest", "plugin"} {
			if _, ok := generator[kind]; ok && !slices.Contains(generators, kind) {
				generators = append(generators, kind)
			}
		}
	}
	gitops.AddField(&item, "Generator", strings.Join(generators, ", "))
	gitops.AddField(&item, "Template project", gitops.Str(object.Object, "spec", "template", "spec", "project"))
	gitops.AddField(&item, "Applications policy", gitops.Str(object.Object, "spec", "syncPolicy", "applicationsSync"))
	return item
}

func parseProject(object unstructured.Unstructured, readAt time.Time) gitops.Item {
	item := gitops.BaseItem(object, readAt)
	m := object.Object
	item.Reason = gitops.Fallback(gitops.Str(m, "spec", "description"), "Allowed repositories and destinations for Applications in this project.")
	repositories := gitops.StringsAt(m, "spec", "sourceRepos")
	destinations := gitops.Maps(m, "spec", "destinations")
	gitops.AddField(&item, "Repositories", fmt.Sprint(len(repositories)))
	gitops.AddField(&item, "Destinations", fmt.Sprint(len(destinations)))
	for _, repository := range repositories {
		gitops.AddField(&item, "Allowed repository", gitops.SafeURL(repository))
	}
	for _, destination := range destinations {
		cluster := gitops.Fallback(gitops.Str(destination, "name"), gitops.SafeURL(gitops.Str(destination, "server")))
		gitops.AddField(&item, "Allowed destination", cluster+" / "+gitops.Str(destination, "namespace"))
	}
	gitops.AddField(&item, "Allowed application namespaces", strings.Join(gitops.StringsAt(m, "spec", "sourceNamespaces"), ", "))
	for _, rule := range []struct{ key, label string }{
		{"clusterResourceWhitelist", "Allowed cluster kind"},
		{"namespaceResourceWhitelist", "Allowed namespaced kind"},
		{"clusterResourceBlacklist", "Denied cluster kind"},
		{"namespaceResourceBlacklist", "Denied namespaced kind"},
	} {
		for _, resource := range gitops.Maps(m, "spec", rule.key) {
			gitops.AddField(&item, rule.label, gitops.Str(resource, "group")+"/"+gitops.Str(resource, "kind"))
		}
	}
	return item
}
