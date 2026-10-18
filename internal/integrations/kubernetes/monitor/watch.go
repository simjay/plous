package monitor

import (
	"context"
	"fmt"
	"slices"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"
)

// Native resource watches update local objects. Server printer columns and
// truncated inventories retain full relists to preserve server semantics.
func (c *Client) Observe(ctx context.Context, r kubecommon.Resource, ns, selector string, emit func(Snapshot)) {
	backoff := time.Second
	for ctx.Err() == nil {
		collection, err := c.List(ctx, r, ns, selector)
		if err != nil {
			emit(Snapshot{Err: err, Status: "Disconnected; retrying"})
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) || apierrors.IsNotFound(err) {
				return
			}
			if !pause(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
		status := "Live"
		if !slices.Contains(r.Verbs, "watch") {
			status = "Refreshing every 5s"
		}
		emit(Snapshot{Collection: &collection, Status: status})
		if !slices.Contains(r.Verbs, "watch") {
			if !pause(ctx, 5*time.Second) {
				return
			}
			continue
		}
		seconds := int64(300)
		stream, err := c.resource(r, ns).Watch(ctx, metav1.ListOptions{LabelSelector: selector, ResourceVersion: collection.Version, TimeoutSeconds: &seconds, AllowWatchBookmarks: true})
		if err != nil {
			if apierrors.IsForbidden(err) {
				r.Verbs = slices.DeleteFunc(slices.Clone(r.Verbs), func(v string) bool { return v == "watch" })
				emit(Snapshot{Status: "Watch denied; refreshing every 5s"})
				if !pause(ctx, 5*time.Second) {
					return
				}
				continue
			}
			emit(Snapshot{Err: err, Status: "Watch disconnected; retrying"})
			if !pause(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		err = c.consume(ctx, stream, r, ns, selector, collection, emit)
		stream.Stop()
		if ctx.Err() != nil {
			return
		}
		if err != nil && !apierrors.IsResourceExpired(err) && !apierrors.IsGone(err) {
			emit(Snapshot{Err: err, Status: "Watch disconnected; retrying"})
		}
		if !pause(ctx, time.Second) {
			return
		}
	}
}

func (c *Client) consume(ctx context.Context, stream watch.Interface, r kubecommon.Resource, ns, selector string, initial Collection, emit func(Snapshot)) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	incremental := HasPresenter(r) && !initial.Truncated
	inventory := newWatchedInventory(initial)
	dirty := false
	lastRead := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-stream.ResultChan():
			if !ok {
				return nil
			}
			if event.Type == watch.Error {
				return apierrors.FromObject(event.Object)
			}
			if event.Type == watch.Bookmark {
				continue
			}
			if incremental {
				object, ok := event.Object.(*unstructured.Unstructured)
				if !ok {
					return fmt.Errorf("unexpected watch object for %s", r.Name)
				}
				object = object.DeepCopy()
				setType(object, r)
				if err := inventory.apply(event.Type, *object); err != nil {
					return err
				}
			}
			dirty = true
		case <-ticker.C:
			endpointsDue := r.Key() == "services" && time.Since(lastRead) >= 5*time.Second
			if !dirty && !endpointsDue {
				continue
			}
			dirty = false
			if !incremental {
				collection, err := c.List(ctx, r, ns, selector)
				if err != nil {
					return err
				}
				lastRead = time.Now()
				emit(Snapshot{Collection: &collection, Status: "Live"})
				continue
			}
			if endpointsDue {
				c.catalogMu.RLock()
				target, available := findResourceIdentity(c.catalog, "endpointslices.discovery.k8s.io")
				c.catalogMu.RUnlock()
				inventory.collection.ServiceEndpoints = c.serviceEndpointReadiness(ctx, inventory.collection.Objects, target, available, ns, "")
				lastRead = time.Now()
			}
			collection := inventory.snapshot()
			emit(Snapshot{Collection: &collection, Status: "Live"})
		}
	}
}

type watchedInventory struct {
	collection Collection
	positions  map[string]int
}

func newWatchedInventory(collection Collection) *watchedInventory {
	collection.Objects = slices.Clone(collection.Objects)
	inventory := &watchedInventory{collection: collection, positions: make(map[string]int, len(collection.Objects))}
	for i, object := range collection.Objects {
		inventory.positions[object.GetNamespace()+"/"+object.GetName()] = i
	}
	return inventory
}

func (w *watchedInventory) apply(kind watch.EventType, object unstructured.Unstructured) error {
	key := object.GetNamespace() + "/" + object.GetName()
	index, exists := w.positions[key]
	switch kind {
	case watch.Added, watch.Modified:
		if exists {
			w.collection.Objects[index] = object
		} else {
			if len(w.collection.Objects) >= MaxObjects {
				return fmt.Errorf("watch inventory reached %d objects; relisting", MaxObjects)
			}
			w.positions[key] = len(w.collection.Objects)
			w.collection.Objects = append(w.collection.Objects, object)
		}
	case watch.Deleted:
		// A delayed delete for a replaced object must not remove its successor.
		if exists && w.collection.Objects[index].GetUID() == object.GetUID() {
			last := len(w.collection.Objects) - 1
			moved := w.collection.Objects[last]
			w.collection.Objects[index] = moved
			w.positions[moved.GetNamespace()+"/"+moved.GetName()] = index
			w.collection.Objects[last] = unstructured.Unstructured{}
			w.collection.Objects = w.collection.Objects[:last]
			delete(w.positions, key)
		}
	default:
		return fmt.Errorf("unexpected watch event %q", kind)
	}
	w.collection.Version = object.GetResourceVersion()
	return nil
}

func (w *watchedInventory) snapshot() Collection {
	result := w.collection
	// Published slices are immutable while the watcher continues processing.
	result.Objects = slices.Clone(w.collection.Objects)
	return result
}
