package monitor

import (
	"github.com/simjay/plous/internal/workspace"
	"k8s.io/apimachinery/pkg/util/validation"
)

var ResourceCommands = map[string]string{
	"po": "pods", "pod": "pods", "pods": "pods",
	"deploy": "deployments.apps", "deployment": "deployments.apps", "deployments": "deployments.apps",
	"sts": "statefulsets.apps", "statefulsets": "statefulsets.apps",
	"ds": "daemonsets.apps", "daemonsets": "daemonsets.apps",
	"rs": "replicasets.apps", "replicasets": "replicasets.apps",
	"job": "jobs.batch", "jobs": "jobs.batch", "cj": "cronjobs.batch", "cronjobs": "cronjobs.batch",
	"svc": "services", "services": "services", "ep": "endpoints", "endpoints": "endpoints",
	"eps": "endpointslices.discovery.k8s.io", "endpointslices": "endpointslices.discovery.k8s.io",
	"ing": "ingresses.networking.k8s.io", "ingresses": "ingresses.networking.k8s.io",
	"netpol": "networkpolicies.networking.k8s.io", "networkpolicies": "networkpolicies.networking.k8s.io",
	"cm": "configmaps", "configmaps": "configmaps", "sec": "secrets", "secrets": "secrets",
	"pvc": "persistentvolumeclaims", "persistentvolumeclaims": "persistentvolumeclaims",
	"pv": "persistentvolumes", "persistentvolumes": "persistentvolumes",
	"sc": "storageclasses.storage.k8s.io", "storageclasses": "storageclasses.storage.k8s.io",
	"no": "nodes", "nodes": "nodes", "ns": "namespaces", "namespaces": "namespaces",
	"ev": "events", "events": "events", "sa": "serviceaccounts", "serviceaccounts": "serviceaccounts",
	"crd": "customresourcedefinitions.apiextensions.k8s.io", "customresourcedefinitions": "customresourcedefinitions.apiextensions.k8s.io",
}

func ValidExploreNamespace(text string) bool {
	return text == "*" || len(validation.IsDNS1123Label(text)) == 0
}

func ExploreNamespace(text string) string {
	if text == "*" {
		return ""
	}
	return text
}

type ExploreConnection struct {
	Workspace, Label string
	Connection       workspace.Connection
}
