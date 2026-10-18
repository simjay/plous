package monitor

import (
	"context"
	"errors"
	"testing"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type overviewMetadata struct {
	t     *testing.T
	items []metav1.PartialObjectMetadata
	err   error
}

func (f overviewMetadata) ListMetadata(_ context.Context, r kubecommon.Resource, ns, selector string) ([]metav1.PartialObjectMetadata, bool, error) {
	if r.Name != "secrets" || ns != "" || selector != "owner=helm" {
		f.t.Fatalf("unexpected read: %+v %q %q", r, ns, selector)
	}
	return f.items, false, f.err
}
func TestOverviewMetadataKeepsLatestRevisionAndUnknownTime(t *testing.T) {
	makeItem := func(version, status string) metav1.PartialObjectMetadata {
		return metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Labels: map[string]string{"owner": "helm", "name": "web", "version": version, "status": status}}}
	}
	result := ReadOverview(context.Background(), overviewMetadata{t: t, items: []metav1.PartialObjectMetadata{makeItem("3", "failed"), makeItem("1", "deployed"), makeItem("2", "deployed")}}, "secret")
	if len(result.Releases) != 1 || result.Releases[0].Revision != 3 || result.Releases[0].Status != "failed" || !result.Releases[0].Updated.IsZero() {
		t.Fatal(result)
	}
	result = ReadOverview(context.Background(), overviewMetadata{t: t, items: []metav1.PartialObjectMetadata{makeItem("broken", "failed")}, err: errors.New("denied")}, "secret")
	if result.Error == "" || !result.Partial || len(result.Releases) != 0 {
		t.Fatal("invalid metadata reported as a complete empty inventory", result)
	}
}
