package common

import (
	"maps"
	"slices"

	apiVersion "k8s.io/apimachinery/pkg/version"
)

type Catalog struct {
	Resources []Resource
	Missing   []string
	Preferred map[string]string
}

func (c Catalog) Resolve(identity, version, verb string) (Resource, bool) {
	var best Resource
	for _, resource := range c.Resources {
		if resource.Key() != identity || version != "" && version != resource.Version || verb != "" && !resource.Supports(verb, "") {
			continue
		}
		if c.prefer(resource, best) {
			best = resource
		}
	}
	return best, best.Name != ""
}

// Listable returns one version per resource identity that advertises list.
func (c Catalog) Listable() []Resource {
	best := make(map[string]Resource)
	for _, resource := range c.Resources {
		if !resource.Supports("list", "") {
			continue
		}
		key := resource.Key()
		if c.prefer(resource, best[key]) {
			best[key] = resource
		}
	}
	return slices.Collect(maps.Values(best))
}

func (c Catalog) prefer(resource, best Resource) bool {
	preferred := c.Preferred[resource.Group]
	return best.Name == "" || resource.Version == preferred ||
		best.Version != preferred && apiVersion.CompareKubeAwareVersionStrings(resource.Version, best.Version) > 0
}
