package common

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Resource struct {
	Group, Version, Name, Kind string
	Namespaced                 bool
	Verbs, ShortNames          []string
	Subresources               map[string][]string
}

func (r Resource) Supports(verb, subresource string) bool {
	if subresource == "" {
		return slices.Contains(r.Verbs, verb)
	}
	return slices.Contains(r.Subresources[subresource], verb)
}

func (r Resource) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: r.Group, Version: r.Version, Resource: r.Name}
}

func (r Resource) Key() string {
	if r.Group == "" {
		return r.Name
	}
	return r.Name + "." + r.Group
}

func (r Resource) Matches(name string) bool {
	return name == r.Key() || name == r.Name || strings.EqualFold(name, r.Kind) || slices.Contains(r.ShortNames, name)
}
