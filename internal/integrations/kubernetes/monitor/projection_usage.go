package monitor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// projectedObjectReader exposes only the reads needed by ownership, logs and usage
// acquisition. It retains the projector's guarded transport and captured scope.
type projectedObjectReader struct {
	projector       *Projector
	bytes, requests int
}

func (r *projectedObjectReader) read(ctx context.Context, resource kubecommon.Resource, ns, name, cursor string) (map[string]any, error) {
	if ns != r.projector.scope.Namespace && resource.Namespaced || !HasUsage(resource) && resource.Group != "metrics.k8s.io" {
		return nil, errors.New("resource read outside captured target")
	}
	if !resource.Supports(map[bool]string{true: "get", false: "list"}[name != ""], "") {
		return nil, &observation.ReadFailure{Message: "Resource API does not support the required read"}
	}
	r.requests++
	if r.requests > 64 || r.bytes >= 2*maxAgentResponseBytes {
		return nil, &observation.ReadFailure{Message: "Resource acquisition limit reached"}
	}
	path := resourcePath(resource, ns)
	if name != "" {
		path += "/" + name
	}
	request := r.projector.client.Get().AbsPath(path)
	if name == "" {
		request.Param("limit", "50").Param("continue", cursor)
	}
	var object map[string]any
	if err := readAgentJSON(ctx, request, int64(min(maxAgentResponseBytes, 2*maxAgentResponseBytes-r.bytes)), &object); err != nil {
		return nil, err
	}
	r.bytes += encodedSize(object)
	return object, nil
}

func (r *projectedObjectReader) Get(ctx context.Context, resource kubecommon.Resource, ns, name string) (*unstructured.Unstructured, error) {
	o, err := r.read(ctx, resource, ns, name, "")
	if err != nil {
		return nil, err
	}
	object := &unstructured.Unstructured{Object: o}
	if object.GetName() != name || resource.Namespaced && object.GetNamespace() != ns {
		return nil, errors.New("measurement object identity mismatch")
	}
	if err := measurementIdentity(resource, *object); err != nil {
		return nil, err
	}
	setType(object, resource)
	return object, nil
}

func (r *projectedObjectReader) List(ctx context.Context, resource kubecommon.Resource, ns, selector string) (Collection, error) {
	result := Collection{}
	cursor := ""
	cursors, identities := map[string]bool{}, map[string]bool{}
	for {
		raw, err := r.read(ctx, resource, ns, "", cursor)
		if err != nil {
			return Collection{}, err
		}
		list := unstructured.UnstructuredList{Object: raw}
		items, ok := raw["items"].([]any)
		if !ok {
			return Collection{}, errors.New("invalid measurement collection")
		}
		if cursor != "" && result.Version != list.GetResourceVersion() {
			return Collection{}, &observation.ReadFailure{Message: "Measurement collection changed during pagination"}
		}
		result.Version = list.GetResourceVersion()
		for _, item := range items {
			value, ok := item.(map[string]any)
			if !ok {
				return Collection{}, errors.New("invalid measurement object")
			}
			object := unstructured.Unstructured{Object: value}
			if object.GetName() == "" || resource.Namespaced && object.GetNamespace() != ns || identities[UsageKey(object)] {
				return Collection{}, errors.New("measurement collection identity mismatch")
			}
			identities[UsageKey(object)] = true
			if err := measurementIdentity(resource, object); err != nil {
				return Collection{}, err
			}
			setType(&object, resource)
			result.Objects = append(result.Objects, object)
		}
		cursor = list.GetContinue()
		if len(result.Objects) > 1000 || len(result.Objects) == 1000 && cursor != "" {
			return Collection{}, &observation.ReadFailure{Message: "Measurement population exceeds 1000 objects"}
		}
		if cursor == "" {
			return result, nil
		}
		if len(cursor) > 4096 || cursors[cursor] {
			return Collection{}, errors.New("invalid measurement continuation")
		}
		cursors[cursor] = true
	}
}

func (r *projectedObjectReader) UsageList(ctx context.Context, resource kubecommon.Resource, ns string) (map[string]Usage, error) {
	collection, err := r.List(ctx, resource, ns, "")
	if err != nil {
		return nil, err
	}
	return usageSamples(collection.Objects, resource.Name == "nodes"), nil
}

func (e *Projector) usage(ctx context.Context, c Command, result observation.Result) (observation.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	r, err := e.discoverResource(ctx, c.Resource)
	if err != nil {
		return result, err
	}
	if !HasUsage(r) || r.Version != "v1" {
		return result, &observation.ReadFailure{Message: "Usage requires a supported Pod, Node or workload API"}
	}
	ns := c.Namespace
	if !r.Namespaced {
		ns, result.Namespace = "", ""
	}
	result.Selection["api_version"] = r.GVR().GroupVersion().String()
	reader := &projectedObjectReader{projector: e}
	object, err := reader.Get(ctx, r, ns, c.Name)
	if err != nil {
		return result, err
	}
	keys := []string{"pods.metrics.k8s.io"}
	if r.Key() == "nodes" {
		keys[0] = "nodes.metrics.k8s.io"
	} else if r.Key() != "pods" {
		keys = append(keys, "pods")
		if r.Key() == "deployments.apps" {
			keys = append(keys, "replicasets.apps")
		}
		if r.Key() == "cronjobs.batch" {
			keys = append(keys, "jobs.batch")
		}
	}
	resources := []kubecommon.Resource{r}
	for _, key := range keys {
		found, err := e.discoverResource(ctx, key)
		if err != nil {
			return result, err
		}
		resources = append(resources, found)
	}
	sample, err := objectUsage(ctx, reader, r, object, resources, result.ObservedAt)
	result.Rows = []observation.Row{{Identity: usageIdentity(r, *object), Facts: usageFacts(sample)}}
	result.Coverage.Observed = 1
	if err != nil {
		var local operationError
		if errors.As(err, &local) {
			return result, &observation.ReadFailure{Message: string(local)}
		}
		return result, err
	}
	projectUsageBudgets(r, *object, sample, result.Rows[0].Facts)
	result.Complete()
	return result, result.Validate()
}

func usageFacts(u Usage) map[string]observation.Value {
	facts := map[string]observation.Value{
		"cpu_nanocores": observation.Unknown(), "memory_bytes": observation.Unknown(),
		"measurements_observed": observation.Number(int64(u.Coverage.Observed)),
		"measurements_complete": observation.Boolean(u.Coverage.Complete),
		"aggregated":            observation.Boolean(u.Aggregated),
	}
	if u.Aggregated {
		facts["active_pods"] = observation.Number(int64(u.Expected))
	}
	if u.Coverage.Complete && !u.Timestamp.IsZero() {
		projectMeasurementValues(u, facts)
		facts["source_start"], facts["source_end"] = observation.Text(u.Timestamp.UTC().Format(time.RFC3339Nano)), observation.Text(u.SourceEnd.UTC().Format(time.RFC3339Nano))
		facts["collected_at"] = observation.Text(u.Collected.UTC().Format(time.RFC3339Nano))
		facts["interval_start"] = observation.Text(u.IntervalStart.UTC().Format(time.RFC3339Nano))
		if !u.Aggregated {
			facts["measurement_window_ns"] = observation.Number(int64(u.Window))
		}
		if u.Snapshot != "" {
			facts["snapshot"] = observation.Text(u.Snapshot)
		}

	}
	return facts
}

func projectMeasurementValues(u Usage, facts map[string]observation.Value) {
	facts["cpu_nanocores"], facts["memory_bytes"] = measuredNumber(u.CPU, 1e9), measuredNumber(u.Memory, 1)
	for _, container := range u.Breakdown {
		facts[fmt.Sprintf("container.%s.cpu_nanocores", container.Name)] = measuredNumber(container.CPU, 1e9)
		facts[fmt.Sprintf("container.%s.memory_bytes", container.Name)] = measuredNumber(container.Memory, 1)
	}
}

func measuredNumber(value, unit float64) observation.Value {
	n := math.Round(value * unit)
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n >= math.MaxInt64 {
		return observation.Unknown()
	}
	return observation.Number(int64(n))
}

func measurementIdentity(r kubecommon.Resource, o unstructured.Unstructured) error {
	if o.GetKind() != "" && o.GetKind() != r.Kind || o.GetAPIVersion() != "" && o.GetAPIVersion() != r.GVR().GroupVersion().String() {
		return errors.New("measurement API identity mismatch")
	}
	return nil
}
