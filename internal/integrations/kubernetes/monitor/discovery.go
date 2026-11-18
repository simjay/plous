package monitor

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	apidiscovery "k8s.io/api/apidiscovery/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/discovery"
)

type discoveryRead func(ctx context.Context, path, accept string, target any) error

// Discover reads either the full catalog or the group of one qualified resource.
func discoverCatalog(ctx context.Context, read discoveryRead, identity string) (kubecommon.Catalog, error) {
	catalog := kubecommon.Catalog{Preferred: map[string]string{"": "v1"}}
	versions := []schema.GroupVersion{{Version: "v1"}}
	aggregated := map[schema.GroupVersion]*metav1.APIResourceList{}
	var groups []metav1.APIGroup
	_, group, _ := strings.Cut(identity, ".")
	switch {
	case identity == "":
		var response struct {
			metav1.TypeMeta
			Groups []metav1.APIGroup                `json:"groups"`
			Items  []apidiscovery.APIGroupDiscovery `json:"items"`
		}
		err := read(ctx, "/apis", discovery.AcceptV2+","+discovery.AcceptV1, &response)
		switch {
		case err != nil:
			catalog.Missing = append(catalog.Missing, "/apis")
		case response.Kind != "APIGroupDiscoveryList":
			groups = response.Groups
		case response.APIVersion != apidiscovery.SchemeGroupVersion.String():
			return catalog, errors.New("unsupported Kubernetes discovery response version")
		default:
			list, resources, failed := discovery.SplitGroupsAndResources(apidiscovery.APIGroupDiscoveryList{Items: response.Items})
			groups, aggregated = list.Groups, resources
			for gv := range failed {
				catalog.Missing = append(catalog.Missing, gv.String())
			}
		}
	case group != "":
		var apiGroup metav1.APIGroup
		if err := read(ctx, "/apis/"+group, "", &apiGroup); err != nil {
			return catalog, err
		}
		// Older servers can omit the group name in the group endpoint.
		if apiGroup.Name == "" {
			apiGroup.Name = group
		}
		if apiGroup.Name != group {
			return catalog, errors.New("Kubernetes returned invalid API discovery")
		}
		groups, versions = []metav1.APIGroup{apiGroup}, nil
	}
	for _, apiGroup := range groups {
		if apiGroup.Name == "" {
			apiGroup.Name, _, _ = strings.Cut(apiGroup.PreferredVersion.GroupVersion, "/")
		}
		preferred, err := discoveryVersion(apiGroup.Name, apiGroup.PreferredVersion.GroupVersion)
		if err != nil {
			if identity != "" {
				return catalog, err
			}
			catalog.Missing = append(catalog.Missing, apiGroup.Name)
		} else {
			catalog.Preferred[preferred.Group] = preferred.Version
			versions = append(versions, preferred)
		}
		for _, advertised := range apiGroup.Versions {
			gv, err := discoveryVersion(apiGroup.Name, advertised.GroupVersion)
			if err == nil && !slices.Contains(versions, gv) {
				versions = append(versions, gv)
			}
		}
	}
	type result struct {
		list *metav1.APIResourceList
		err  error
	}
	results := make([]result, len(versions))
	var workers sync.WaitGroup
	slots := make(chan struct{}, 6)
	for i, gv := range versions {
		if list := aggregated[gv]; list != nil {
			results[i].list = list
			continue
		}
		workers.Go(func() {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-slots }()
			path := "/api/" + gv.Version
			if gv.Group != "" {
				path = "/apis/" + gv.String()
			}
			list := new(metav1.APIResourceList)
			results[i] = result{list, read(ctx, path, "", list)}
		})
	}
	workers.Wait()
	if ctx.Err() != nil {
		return kubecommon.Catalog{}, ctx.Err()
	}
	for i, result := range results {
		gv := versions[i]
		if result.err != nil {
			if identity != "" && len(versions) == 1 {
				return catalog, result.err
			}
			catalog.Missing = append(catalog.Missing, gv.String())
			continue
		}
		catalog.Resources = append(catalog.Resources, resourcesForVersion(gv, result.list)...)
	}
	slices.Sort(catalog.Missing)
	catalog.Missing = slices.Compact(catalog.Missing)
	return catalog, nil
}

func discoveryVersion(group, value string) (schema.GroupVersion, error) {
	gv, err := schema.ParseGroupVersion(value)
	if err != nil || gv.Group != group || len(validation.IsDNS1123Label(gv.Version)) != 0 ||
		group != "" && len(validation.IsDNS1123Subdomain(group)) != 0 {
		return schema.GroupVersion{}, errors.New("Kubernetes returned invalid API discovery")
	}
	return gv, nil
}

func resourcesForVersion(gv schema.GroupVersion, list *metav1.APIResourceList) []kubecommon.Resource {
	subresources := map[string]map[string][]string{}
	for _, item := range list.APIResources {
		parent, subresource, ok := strings.Cut(item.Name, "/")
		if ok {
			if subresources[parent] == nil {
				subresources[parent] = map[string][]string{}
			}
			subresources[parent][subresource] = slices.Clone(item.Verbs)
		}
	}
	var resources []kubecommon.Resource
	for _, item := range list.APIResources {
		if len(validation.IsDNS1123Label(item.Name)) != 0 {
			continue
		}
		resources = append(resources, kubecommon.Resource{
			Group: gv.Group, Version: gv.Version, Name: item.Name, Kind: item.Kind, Namespaced: item.Namespaced,
			Verbs: item.Verbs, ShortNames: item.ShortNames, Subresources: subresources[item.Name],
		})
	}
	return resources
}
