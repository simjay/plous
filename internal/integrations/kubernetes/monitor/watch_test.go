package monitor

import (
	"context"
	"fmt"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"
)

func TestWatchInventoryPreservesPublishedSnapshotsAndReplacementUID(t *testing.T) {
	first := unstructured.Unstructured{Object: testPod("one", "old")}
	initial := Collection{Objects: []unstructured.Unstructured{first}, Version: "10"}
	w := newWatchedInventory(initial)
	published := w.snapshot()
	replacement := unstructured.Unstructured{Object: testPod("one", "new")}
	replacement.SetResourceVersion("11")
	if err := w.apply(watch.Modified, replacement); err != nil {
		t.Fatal(err)
	}
	if err := w.apply(watch.Deleted, first); err != nil {
		t.Fatal(err)
	}
	if len(w.collection.Objects) != 1 || string(w.collection.Objects[0].GetUID()) != "new" {
		t.Fatal("late deletion removed replacement")
	}
	if string(published.Objects[0].GetUID()) != "old" || string(initial.Objects[0].GetUID()) != "old" {
		t.Fatal("watch mutated a published collection")
	}
	second := unstructured.Unstructured{Object: testPod("two", "two")}
	w.apply(watch.Added, second)
	w.apply(watch.Deleted, replacement)
	w.apply(watch.Modified, second)
	if len(w.collection.Objects) != 1 || w.collection.Objects[0].GetName() != "two" {
		t.Fatal("delete broke positions")
	}
	w.collection.Objects = make([]unstructured.Unstructured, MaxObjects)
	if err := w.apply(watch.Added, first); err == nil {
		t.Fatal("watch exceeded object limit")
	}
}

func TestWatchCoalescesNativeEventsWithoutListCalls(t *testing.T) {
	client := &Client{} // Any unexpected list request fails this test.
	stream := watch.NewRaceFreeFake()
	defer stream.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for i := 0; i < 10; i++ {
		stream.Add(&unstructured.Unstructured{Object: testPod(fmt.Sprint(i), fmt.Sprint(i))})
	}
	published := 0
	err := client.consume(ctx, stream, kubecommon.Resource{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true}, "shop", "", Collection{}, func(snapshot Snapshot) {
		published++
		if len(snapshot.Collection.Objects) != 10 {
			t.Errorf("burst was not coalesced: %d", len(snapshot.Collection.Objects))
		}
		cancel()
	})
	if err != context.Canceled || published != 1 {
		t.Fatalf("err=%v published=%d", err, published)
	}
}
