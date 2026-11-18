package monitor

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
)

func (e *Projector) projectCatalog(ctx context.Context, query string, result observation.Result) (observation.Result, error) {
	result.Schema = "kubernetes.resource_types.v1"
	result.Namespace = ""
	result.Selection = map[string]string{"query": query}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	catalog, err := discoverCatalog(ctx, e.readDiscovery, "")
	if err != nil {
		return result, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	slices.SortFunc(catalog.Resources, func(a, b kubecommon.Resource) int {
		if order := cmp.Compare(a.Key(), b.Key()); order != 0 {
			return order
		}
		return cmp.Compare(a.Version, b.Version)
	})
	for _, resource := range catalog.Resources {
		key := resource.Key()
		if resource.Name == "secret" || resource.Name == "secrets" {
			continue
		}
		get, list := resource.Supports("get", ""), resource.Supports("list", "")
		if !get && !list || !strings.Contains(strings.ToLower(key+" "+resource.Kind), query) {
			continue
		}
		if len(result.Rows) == 10000 {
			result.Partial("discovery row limit reached")
			break
		}
		result.Rows = append(result.Rows, observation.Row{
			Identity: observation.Identity{API: resource.GVR().GroupVersion().String(), Kind: resource.Kind, Name: key},
			Facts: map[string]observation.Value{
				"api_version": observation.Text(resource.Version), "namespaced": observation.Boolean(resource.Namespaced),
				"get": observation.Boolean(get), "list": observation.Boolean(list),
			},
		})
	}
	result.Coverage.Observed = len(result.Rows)
	if len(catalog.Missing) > 0 {
		result.Partial("some API groups or versions could not be discovered")
	}
	if result.Outcome != "partial" {
		result.Complete()
	}
	return result, nil
}
