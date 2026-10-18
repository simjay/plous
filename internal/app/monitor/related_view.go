package monitor

import (
	"cmp"
	"slices"
	"strings"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

var RelatedGroups = []string{"Owner", "Managed resources", "Referenced resources", "Selector matches"}

func RelatedGroup(ref kubernetes.Reference) int {
	if ref.Relation == "Owner" {
		return 0
	}
	if strings.Contains(strings.ToLower(ref.Relation), "selector match") {
		return 3
	}
	// A claim or node used by an owned Pod is a reference, not an owned object.
	if strings.HasPrefix(ref.Relation, "Owned ") && !strings.HasPrefix(ref.Relation, "Owned Pod ") {
		return 1
	}
	return 2
}

func SortedReferences(refs []kubernetes.Reference) []kubernetes.Reference {
	refs = slices.Clone(refs)
	slices.SortStableFunc(refs, func(a, b kubernetes.Reference) int {
		if n := cmp.Compare(RelatedGroup(a), RelatedGroup(b)); n != 0 {
			return n
		}
		return cmp.Compare(a.Resource.Kind+"/"+a.Namespace+"/"+a.Name, b.Resource.Kind+"/"+b.Namespace+"/"+b.Name)
	})
	return refs
}
