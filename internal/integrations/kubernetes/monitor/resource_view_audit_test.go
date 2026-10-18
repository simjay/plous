package monitor

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNativeSummaryRetainsEmptySubresourceAndNestedMetadata(t *testing.T) {
	cases := []struct {
		identity, fixture string
		want              []string
	}{
		{"resourceclaimtemplates.resource.k8s.io", `{"metadata":{"labels":{"outer":"object"}},"spec":{"metadata":{"labels":{"generated":"claim"},"annotations":{"purpose":"training"}},"spec":{"devices":{"requests":[]}}}}`, []string{"Generated claim metadata", "labels / generated · claim", "annotations / purpose · training"}},
		{"podsecuritypolicies.policy", `{"spec":{"seLinux":{"rule":"MustRunAs","seLinuxOptions":{"user":"system_u","role":"system_r","type":"container_t","level":"s0:c0,c1"}}}}`, []string{"SELinux options", "User: system_u", "Role: system_r", "Type: container_t", "Level: s0:c0,c1"}},
		{"customresourcedefinitions.apiextensions.k8s.io", `{"spec":{"versions":[{"name":"v1","subresources":{"status":{},"scale":{"specReplicasPath":".spec.replicas","statusReplicasPath":".status.replicas"}}},{"name":"v2","subresources":{}}]}}`, []string{"Subresources · v1", "status: Enabled", "scale: Enabled", "scale / specReplicasPath · .spec.replicas"}},
	}
	for _, tc := range cases {
		t.Run(tc.identity, func(t *testing.T) {
			name, group, _ := strings.Cut(tc.identity, ".")
			text := summaryText(kubecommon.Resource{Name: name, Group: group}, summaryFixture(t, tc.fixture))
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q in %s", want, text)
				}
			}
		})
	}
}

func TestPersistentVolumeRelationRequiresMatchingClaimUID(t *testing.T) {
	for _, tc := range []struct {
		name, claimUID  string
		denied, matches bool
	}{
		{"matching", "claim-1", false, true}, {"replaced claim", "claim-2", false, false}, {"missing UID", "", false, false}, {"denied", "claim-1", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/persistentvolumes/disk" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				if tc.denied {
					apiError(w, http.StatusForbidden, "Forbidden")
					return
				}
				volume := summaryFixture(t, `{"apiVersion":"v1","kind":"PersistentVolume","metadata":{"name":"disk","uid":"volume-1"},"spec":{"claimRef":{"namespace":"shop","name":"data","uid":"`+tc.claimUID+`"}}}`)
				reply(w, volume.Object)
			})
			claim := summaryFixture(t, `{"metadata":{"name":"data","namespace":"shop","uid":"claim-1"},"spec":{"volumeName":"disk"}}`)
			refs, warning := client.Related(context.Background(), kubecommon.Resource{Name: "persistentvolumeclaims", Kind: "PersistentVolumeClaim", Namespaced: true}, claim, []kubecommon.Resource{{Name: "persistentvolumes", Kind: "PersistentVolume", Version: "v1"}})
			if tc.matches {
				if len(refs) != 1 || refs[0].UID != "volume-1" || !strings.Contains(refs[0].Relation, "verified claim UID") || warning != "" {
					t.Fatalf("refs=%#v warning=%s", refs, warning)
				}
			} else if tc.denied {
				if len(refs) != 1 || !strings.Contains(refs[0].Relation, "not verified") || !strings.Contains(strings.ToLower(warning), "denied") {
					t.Fatalf("refs=%#v warning=%s", refs, warning)
				}
			} else if len(refs) != 0 || !strings.Contains(warning, "does not report a binding") {
				t.Fatalf("refs=%#v warning=%s", refs, warning)
			}
		})
	}
}

func TestEndpointSliceTableIncludesPortProtocol(t *testing.T) {
	object := summaryFixture(t, `{"metadata":{"name":"checkout"},"ports":[{"name":"https","port":8443,"protocol":"TCP"}]}`)
	columns, rows := Table(kubecommon.Resource{Name: "endpointslices", Group: "discovery.k8s.io"}, Collection{Objects: []unstructured.Unstructured{object}}, time.Now())
	for i, column := range columns {
		if column == "Ports" {
			if !strings.Contains(rows[0].Cells[i], "8443") || !strings.Contains(rows[0].Cells[i], "TCP") {
				t.Fatal(rows[0].Cells[i])
			}
			return
		}
	}
	t.Fatal("missing Ports column")
}
