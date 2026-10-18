// Package monitor reads flux controller observations through a captured Kubernetes reader.
package monitor

import (
	"slices"

	"github.com/simjay/plous/internal/integrations/flux/common"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	transport "github.com/simjay/plous/internal/integrations/gitops/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

func roots() []gitops.Reference {
	return []gitops.Reference{{Group: common.KustomizeGroup, Kind: "Kustomization"}, {Group: common.HelmGroup, Kind: "HelmRelease"}}
}
func sourceRoots() []gitops.Reference {
	var refs []gitops.Reference
	for _, kind := range []string{"GitRepository", "OCIRepository", "HelmRepository", "HelmChart", "Bucket"} {
		refs = append(refs, gitops.Reference{Group: common.SourceGroup, Kind: kind})
	}
	return refs
}
func New(reader transport.Reader, sections ...gitops.Section) *transport.Client {
	refs, optional := roots(), false
	if len(sections) > 0 {
		switch gitops.NormalizeSection(gitops.Flux, sections[0]) {
		case gitops.Kustomizations:
			refs = refs[:1]
		case gitops.HelmReleases:
			refs = refs[1:]
		case gitops.Sources:
			refs, optional = sourceRoots(), true
		}
	}
	return transport.New(reader, transport.Definition{Roots: refs, OptionalRoots: optional, Allows: func(ref gitops.Reference) bool { return common.Allows(ref) }, SupportsVersion: SupportsVersion, Parse: ParseFlux})
}
func Supports(catalog []kubecommon.Resource) bool {
	return transport.Supports(catalog, append(roots(), sourceRoots()...))
}
func SupportsVersion(r kubecommon.Resource) bool {
	switch r.Group {
	case common.KustomizeGroup, common.SourceGroup:
		return slices.Contains([]string{"v1", "v1beta2", "v1beta1"}, r.Version)
	case common.HelmGroup:
		return slices.Contains([]string{"v2", "v2beta2", "v2beta1"}, r.Version)
	}
	return false
}
