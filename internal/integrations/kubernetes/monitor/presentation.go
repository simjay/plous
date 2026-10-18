package monitor

import (
	"fmt"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"go.yaml.in/yaml/v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func HasPresenter(r kubecommon.Resource) bool {
	_, ok := nativeColumns[r.Key()]
	return ok
}

type Row struct {
	Object  unstructured.Unstructured
	Cells   []string
	Numbers map[int]float64
}

func Table(r kubecommon.Resource, collection Collection, now time.Time) ([]string, []Row) {
	definition, native := nativeColumns[r.Key()]
	columns := []string{"Name", "Status", "Age"}
	if native {
		columns = []string{"Name"}
		for _, column := range definition {
			columns = append(columns, column.title)
		}
		columns = append(columns, "Age")
	} else if len(collection.Columns) > 0 {
		columns = append([]string(nil), collection.Columns...)
	}
	if r.Namespaced {
		columns = append(columns[:1], append([]string{"Namespace"}, columns[1:]...)...)
	}
	// Append fields to preserve saved sort indices from older sessions.
	if r.Key() == "pods" || r.Key() == "nodes" {
		columns = append(columns, "CPU", "Memory")
		if r.Key() == "pods" {
			columns = append(columns, "Owner", "QoS")
		}
	}
	rows := make([]Row, 0, len(collection.Objects))
	for _, object := range collection.Objects {
		numbers := map[int]float64{}
		cells := []string{object.GetName(), fallback(Value(object.Object, "status", "phase"), "Not reported"), Age(object.GetCreationTimestamp().Time, now)}
		if native {
			cells = []string{object.GetName()}
			for _, column := range definition {
				value := column.value(object.Object)
				if r.Key() == "services" && column.title == "Ready endpoints" {
					if readiness, found := collection.ServiceEndpoints[Identity(object)]; found {
						value = readiness.Cell()
					}
				}
				cells = append(cells, fallback(value, "Not reported"))
			}
			cells = append(cells, Age(object.GetCreationTimestamp().Time, now))
		} else if len(collection.Columns) > 0 {
			cells = append([]string(nil), collection.Cells[Identity(object)]...)
			if len(cells) != len(collection.Columns) {
				cells = make([]string, len(collection.Columns))
				for i := range cells {
					cells[i] = "Not reported"
				}
				cells[0] = object.GetName()
			}
		}
		if r.Namespaced {
			cells = append(cells[:1], append([]string{object.GetNamespace()}, cells[1:]...)...)
		}
		if r.Key() == "pods" || r.Key() == "nodes" {
			cpu, memory := "N/A", "N/A"
			if usage, found := collection.Usage[UsageKey(object)]; found && usage.Matches(object, now) && (r.Key() != "pods" || !terminalPod(object)) {
				cpu, memory = CPUUsage(usage.CPU), MemoryUsage(usage.Memory)
				numbers[len(cells)], numbers[len(cells)+1] = usage.CPU, usage.Memory
			}
			cells = append(cells, cpu, memory)
			if r.Key() == "pods" {
				cells = append(cells, controllerName(object), fallback(Value(object.Object, "status", "qosClass"), "Not reported"))
			}
		}
		rows = append(rows, Row{Object: object, Cells: cells, Numbers: numbers})
	}
	return columns, rows
}

func PodState(o map[string]any) (ready, total, restarts int, status string) {
	f := podFacts(o)
	return f.ContainersReady, f.ContainersTotal, f.Restarts, f.Status
}

func podDisplayState(o map[string]any) (ready, total, restarts int, status string) {
	total = len(Maps(o, "spec", "containers"))
	status = Value(o, "status", "phase")
	if status == "Succeeded" {
		status = "Completed"
	}
	for _, s := range Maps(o, "status", "containerStatuses") {
		if Value(s, "ready") == "true" {
			ready++
		}
		restarts += integer(s, "restartCount")
		if reason := Value(s, "state", "waiting", "reason"); reason != "" {
			status = reason
		} else if reason := Value(s, "state", "terminated", "reason"); reason != "" && reason != "Completed" {
			status = reason
		}
	}
	initStatus := map[string]map[string]any{}
	for _, s := range Maps(o, "status", "initContainerStatuses") {
		initStatus[Value(s, "name")] = s
		restarts += integer(s, "restartCount")
	}
	initializing := false
	initContainers := Maps(o, "spec", "initContainers")
	for i, container := range initContainers {
		s := initStatus[Value(container, "name")]
		sidecar := Value(container, "restartPolicy") == "Always"
		if sidecar {
			total++
			if Value(s, "ready") == "true" {
				ready++
			}
		}
		if initializing || (sidecar && Value(s, "started") == "true") || (!sidecar && Value(s, "state", "terminated", "exitCode") == "0") {
			continue
		}
		initializing = true
		status = fmt.Sprintf("Init:%d/%d", i, len(initContainers))
		if reason := Value(s, "state", "waiting", "reason"); reason != "" {
			status = "Init:" + reason
		} else if reason := Value(s, "state", "terminated", "reason"); reason != "" {
			status = "Init:" + reason
		} else if code := Value(s, "state", "terminated", "exitCode"); code != "" && code != "0" {
			status = "Init:ExitCode:" + code
		}
	}
	if Value(o, "metadata", "deletionTimestamp") != "" {
		status = "Terminating"
	}
	if status == "" {
		status = "Unknown"
	}
	return
}

func Containers(object unstructured.Unstructured) []string {
	var names []string
	for _, kind := range []string{"containers", "initContainers", "ephemeralContainers"} {
		for _, container := range Maps(object.Object, "spec", kind) {
			names = append(names, Value(container, "name"))
		}
	}
	return names
}

func YAML(object unstructured.Unstructured, reveal bool) string {
	copy := object.DeepCopy()
	unstructured.RemoveNestedField(copy.Object, "metadata", "managedFields")
	if copy.GetKind() == "Secret" && !reveal {
		for _, field := range []string{"data", "stringData"} {
			if values, ok, _ := unstructured.NestedMap(copy.Object, field); ok {
				for key := range values {
					values[key] = "<hidden>"
				}
				_ = unstructured.SetNestedMap(copy.Object, values, field)
			}
		}
		unstructured.RemoveNestedField(copy.Object, "metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration")
	}
	data, err := yaml.Marshal(copy.Object)
	if err != nil {
		return "Cannot render this object as YAML."
	}
	return string(data)
}

func Value(object map[string]any, fields ...string) string {
	v, found, _ := unstructured.NestedFieldNoCopy(object, fields...)
	if !found || v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func Maps(object map[string]any, fields ...string) []map[string]any {
	values, _, _ := unstructured.NestedSlice(object, fields...)
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if item, ok := value.(map[string]any); ok {
			result = append(result, item)
		}
	}
	return result
}
func integer(object map[string]any, fields ...string) int {
	v, _, _ := unstructured.NestedInt64(object, fields...)
	return int(v)
}
func Age(created, now time.Time) string {
	if created.IsZero() {
		return "N/A"
	}
	d := max(time.Duration(0), now.Sub(created))
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 48*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
