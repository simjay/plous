package monitor

import (
	"context"
	"net/http"
	"strings"
	"testing"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

func TestStorageDeleteNoticeReportsVerifiedReclaimAndConsumers(t *testing.T) {
	resources := []kubecommon.Resource{{Name: "persistentvolumes", Kind: "PersistentVolume", Version: "v1"}, {Name: "persistentvolumeclaims", Kind: "PersistentVolumeClaim", Version: "v1", Namespaced: true}, {Name: "pods", Kind: "Pod", Version: "v1", Namespaced: true}}
	claim := summaryFixture(t, `{"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"name":"data","namespace":"shop","uid":"claim-1"},"spec":{"volumeName":"disk"}}`)
	volume := summaryFixture(t, `{"apiVersion":"v1","kind":"PersistentVolume","metadata":{"name":"disk","uid":"volume-1"},"spec":{"persistentVolumeReclaimPolicy":"Delete","claimRef":{"name":"data","namespace":"shop","uid":"claim-1"}}}`)
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/persistentvolumes/disk":
			reply(w, volume.Object)
		case "/api/v1/namespaces/shop/persistentvolumeclaims/data":
			reply(w, claim.Object)
		case "/api/v1/namespaces/shop/pods":
			consumer := summaryFixture(t, `{"metadata":{"name":"checkout","namespace":"shop"},"spec":{"volumes":[{"persistentVolumeClaim":{"claimName":"data"}}]},"status":{"phase":"Running"}}`)
			other := summaryFixture(t, `{"metadata":{"name":"unrelated","namespace":"shop"},"spec":{"volumes":[{"persistentVolumeClaim":{"claimName":"elsewhere"}}]}}`)
			reply(w, objectList("Pod", "1", consumer.Object, other.Object))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			apiError(w, 404, "NotFound")
		}
	})
	for _, r := range resources[:2] {
		object := &claim
		if r.Name == "persistentvolumes" {
			object = &volume
		}
		notice := client.StorageDeleteNotice(context.Background(), r, object, resources)
		for _, want := range []string{"reclaim policy: Delete", "verified", "Consumers: 1 Pod volume references", "checkout (Running)"} {
			if !strings.Contains(notice, want) {
				t.Fatalf("missing %s: %s", want, notice)
			}
		}
		if strings.Contains(notice, "unrelated") {
			t.Fatal(notice)
		}
	}
}

func TestStorageDeleteNoticeDoesNotBorrowPolicyFromReplacedBinding(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/persistentvolumes/disk" {
			volume := summaryFixture(t, `{"apiVersion":"v1","kind":"PersistentVolume","metadata":{"name":"disk"},"spec":{"persistentVolumeReclaimPolicy":"Delete","claimRef":{"name":"data","namespace":"shop","uid":"old-claim"}}}`)
			reply(w, volume.Object)
		} else {
			apiError(w, 403, "Forbidden")
		}
	})
	claim := summaryFixture(t, `{"metadata":{"name":"data","namespace":"shop","uid":"new-claim"},"spec":{"volumeName":"disk"}}`)
	r := kubecommon.Resource{Name: "persistentvolumeclaims", Kind: "PersistentVolumeClaim", Version: "v1", Namespaced: true}
	notice := client.StorageDeleteNotice(context.Background(), r, &claim, []kubecommon.Resource{{Name: "persistentvolumes", Kind: "PersistentVolume", Version: "v1"}, {Name: "pods", Kind: "Pod", Version: "v1", Namespaced: true}})
	if strings.Contains(notice, "reclaim policy: Delete") || !strings.Contains(notice, "does not report a binding") || !strings.Contains(notice, "Consumers: unknown") || !strings.Contains(strings.ToLower(notice), "denied") {
		t.Fatal(notice)
	}
}

func TestSnapshotDeleteNoticeRequiresReciprocalBinding(t *testing.T) {
	for _, tc := range []struct {
		name, refName, refNamespace, refUID, contentName string
		denied, valid                                    bool
	}{
		{"matching", "backup", "shop", "snapshot-1", "content-1", false, true},
		{"replaced snapshot", "backup", "shop", "snapshot-old", "content-1", false, false},
		{"wrong namespace", "backup", "other", "snapshot-1", "content-1", false, false},
		{"wrong name", "other", "shop", "snapshot-1", "content-1", false, false},
		{"missing UID", "backup", "shop", "", "content-1", false, false},
		{"nonreciprocal content", "backup", "shop", "snapshot-1", "content-other", false, false},
		{"denied", "backup", "shop", "snapshot-1", "content-1", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/apis/snapshot.storage.k8s.io/v1/volumesnapshotcontents/content-1" {
					t.Errorf("unexpected request %s", r.URL.Path)
				}
				if tc.denied {
					apiError(w, 403, "Forbidden")
					return
				}
				content := summaryFixture(t, `{"apiVersion":"snapshot.storage.k8s.io/v1","kind":"VolumeSnapshotContent","metadata":{"name":"`+tc.contentName+`"},"spec":{"deletionPolicy":"Delete","volumeSnapshotRef":{"apiVersion":"snapshot.storage.k8s.io/v1","kind":"VolumeSnapshot","name":"`+tc.refName+`","namespace":"`+tc.refNamespace+`","uid":"`+tc.refUID+`"}}}`)
				reply(w, content.Object)
			})
			snapshot := summaryFixture(t, `{"metadata":{"name":"backup","namespace":"shop","uid":"snapshot-1"},"status":{"boundVolumeSnapshotContentName":"content-1"}}`)
			r := kubecommon.Resource{Name: "volumesnapshots", Group: "snapshot.storage.k8s.io", Version: "v1", Namespaced: true}
			notice := client.StorageDeleteNotice(context.Background(), r, &snapshot, []kubecommon.Resource{{Name: "volumesnapshotcontents", Group: "snapshot.storage.k8s.io", Version: "v1"}})
			if tc.valid {
				if !strings.Contains(notice, "verified snapshot UID") || !strings.Contains(notice, "deletion policy: Delete") {
					t.Fatal(notice)
				}
			} else if strings.Contains(notice, "deletion policy: Delete") || !strings.Contains(notice, "unknown") {
				t.Fatal(notice)
			}
			if tc.denied && !strings.Contains(strings.ToLower(notice), "denied") {
				t.Fatal(notice)
			}
		})
	}
}

func TestSnapshotContentDeleteNoticeUsesOwnPolicyWithoutClaimingVerifiedReference(t *testing.T) {
	content := summaryFixture(t, `{"metadata":{"name":"content-1"},"spec":{"deletionPolicy":"Retain","volumeSnapshotRef":{"name":"backup","namespace":"shop","uid":"snapshot-1"}}}`)
	var client *Client
	notice := client.StorageDeleteNotice(context.Background(), kubecommon.Resource{Name: "volumesnapshotcontents", Group: "snapshot.storage.k8s.io"}, &content, nil)
	for _, want := range []string{"deletion policy: Retain", "shop/backup", "snapshot-1", "has not been verified"} {
		if !strings.Contains(notice, want) {
			t.Fatal(notice)
		}
	}
	snapshot := summaryFixture(t, `{"metadata":{"name":"backup","namespace":"shop","uid":"snapshot-1"},"spec":{"source":{"volumeSnapshotContentName":"content-1"}}}`)
	notice = client.StorageDeleteNotice(context.Background(), kubecommon.Resource{Name: "volumesnapshots", Group: "snapshot.storage.k8s.io"}, &snapshot, nil)
	if !strings.Contains(notice, "unknown; no bound snapshot content is reported") {
		t.Fatal(notice)
	}
}
