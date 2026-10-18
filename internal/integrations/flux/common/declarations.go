// Package common owns Flux API identities and source declarations.
package common

import (
	"slices"
	"strings"

	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const SourceGroup = "source.toolkit.fluxcd.io"
const KustomizeGroup = "kustomize.toolkit.fluxcd.io"
const HelmGroup = "helm.toolkit.fluxcd.io"

func Recognizes(group string) bool {
	return strings.HasSuffix(group, ".toolkit.fluxcd.io") || group == "source.extensions.fluxcd.io"
}
func Allows(ref gitops.Reference) bool {
	return ref.Group == KustomizeGroup && ref.Kind == "Kustomization" || ref.Group == HelmGroup && ref.Kind == "HelmRelease" || ref.Group == SourceGroup && slices.Contains([]string{"GitRepository", "OCIRepository", "HelmRepository", "HelmChart", "Bucket"}, ref.Kind)
}
func SourceReferences(m map[string]any, namespace string) []gitops.SourceReference {
	paths := [][]string{{"spec", "sourceRef"}, {"spec", "chartRef"}, {"spec", "chart", "spec", "sourceRef"}}
	seen := map[string]bool{}
	var refs []gitops.SourceReference
	for _, path := range paths {
		source, found, _ := unstructured.NestedMap(m, path...)
		if !found {
			continue
		}
		ref := gitops.Reference{Group: SourceGroup, Kind: gitops.Str(source, "kind"), Namespace: gitops.Fallback(gitops.Str(source, "namespace"), namespace), Name: gitops.Str(source, "name")}
		if version := gitops.Str(source, "apiVersion"); version != "" {
			if gv, err := schema.ParseGroupVersion(version); err == nil {
				ref.Group, ref.Version = gv.Group, gv.Version
			}
		}
		if ref.Name != "" && Allows(ref) && !seen[ref.Key()] {
			refs = append(refs, gitops.SourceReference{Ref: ref})
			seen[ref.Key()] = true
		}
	}
	return refs
}
