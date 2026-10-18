// Package code inspects Flux declarations in captured repository documents.
package code

import (
	"fmt"
	"path"
	"strings"

	"github.com/simjay/plous/internal/integrations/catalog"
	"github.com/simjay/plous/internal/integrations/flux/common"
)

func Discover(docs []catalog.Document) []catalog.Item {
	var result []catalog.Item
	add := func(item catalog.Item) { result = append(result, item) }
	for _, d := range docs {
		api, kind := catalog.Text(d.Object, "apiVersion"), catalog.Text(d.Object, "kind")
		group, _, _ := strings.Cut(api, "/")
		name := catalog.Text(d.Object, "metadata", "name")
		if name == "" || !common.Recognizes(group) {
			continue
		}
		id := fmt.Sprintf("flux:%s:%d", d.File, d.Index)
		c := catalog.Item{ID: id, Technology: "flux", Group: path.Dir(d.File), Name: name, Kind: kind, File: d.File}
		spec := catalog.Object(d.Object, "spec")
		ref := catalog.Object(spec, "sourceRef")
		c.Detail = strings.TrimSpace(catalog.Text(ref, "kind") + " " + catalog.Text(ref, "name"))
		if p := catalog.Text(spec, "path"); p != "" {
			c.Detail += " · " + p
		}
		if kind == "HelmRelease" {
			chart := catalog.Object(spec, "chart", "spec")
			chartName := catalog.Text(chart, "chart")
			chartRef := catalog.Object(spec, "chartRef")
			if chartName == "" {
				chartName = catalog.Text(chartRef, "name")
			}
			if chartName != "" {
				sourceRef := catalog.Object(chart, "sourceRef")
				if len(sourceRef) == 0 {
					sourceRef = chartRef
				}
				location := strings.TrimSpace(catalog.Text(sourceRef, "kind") + " " + catalog.Text(sourceRef, "name"))
				namespace := catalog.Text(sourceRef, "namespace")
				if namespace == "" {
					namespace = catalog.Text(d.Object, "metadata", "namespace")
				}
				for _, other := range docs {
					if group, _, _ := strings.Cut(catalog.Text(other.Object, "apiVersion"), "/"); common.Recognizes(group) && catalog.Text(other.Object, "kind") == catalog.Text(sourceRef, "kind") && catalog.Text(other.Object, "metadata", "name") == catalog.Text(sourceRef, "name") && catalog.Text(other.Object, "metadata", "namespace") == namespace {
						if url := catalog.Text(other.Object, "spec", "url"); url != "" {
							location += " · " + url
						}
					}
				}
				add(catalog.Item{ID: id + "/chart", Technology: "helm", Group: path.Dir(d.File), Name: chartName, Kind: "Helm chart", File: d.File, Version: catalog.Text(chart, "version"), Location: location, Detail: "Declared by Flux HelmRelease " + name, External: true})
				c.Detail = "Chart: " + chartName
			}
		}
		add(c)
	}
	return result
}
