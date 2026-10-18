// Package common owns Argo API identities and source declarations.
package common

import (
	"slices"

	"github.com/simjay/plous/internal/integrations/gitops/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const Group = "argoproj.io"

func Recognizes(group, kind string) bool {
	return group == Group && slices.Contains([]string{"Application", "ApplicationSet", "AppProject"}, kind)
}
func Sources(spec map[string]any) []common.SourceReference {
	sources := common.Maps(spec, "sources")
	if len(sources) == 0 {
		if source, found, _ := unstructured.NestedMap(spec, "source"); found {
			sources = append(sources, source)
		}
	}
	var result []common.SourceReference
	for _, source := range sources {
		result = append(result, common.SourceReference{URL: common.SafeURL(common.Str(source, "repoURL")), Path: common.Str(source, "path"), Revision: common.Str(source, "targetRevision"), Chart: common.Str(source, "chart")})
	}
	return result
}
