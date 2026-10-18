// Package code inspects Argo declarations in captured repository documents.
package code

import (
	"fmt"
	"path"
	"strings"

	"github.com/simjay/plous/internal/integrations/argo/common"
	"github.com/simjay/plous/internal/integrations/catalog"
)

func Discover(docs []catalog.Document) []catalog.Item {
	var result []catalog.Item
	for _, d := range docs {
		api, kind := catalog.Text(d.Object, "apiVersion"), catalog.Text(d.Object, "kind")
		group, _, _ := strings.Cut(api, "/")
		name := catalog.Text(d.Object, "metadata", "name")
		if name == "" || !common.Recognizes(group, kind) {
			continue
		}
		id := fmt.Sprintf("argo:%s:%d", d.File, d.Index)
		item := catalog.Item{ID: id, Technology: "argo", Group: path.Dir(d.File), Name: name, Kind: kind, File: d.File}
		spec := catalog.Object(d.Object, "spec")
		if kind == "ApplicationSet" {
			spec = catalog.Object(spec, "template", "spec")
		}
		for n, source := range common.Sources(spec) {
			item.Detail = strings.TrimSpace(item.Detail + " " + source.Path)
			if source.Chart != "" {
				result = append(result, catalog.Item{ID: fmt.Sprintf("%s:chart:%d", id, n), Technology: "helm", Group: path.Dir(d.File), Name: source.Chart, Kind: "Helm chart", File: d.File, Version: source.Revision, Location: source.URL, Detail: "Declared by Argo " + kind + " " + name, External: true})
			}
		}
		result = append(result, item)
	}
	return result
}
