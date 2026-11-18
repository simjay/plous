package monitor

import (
	"context"
	"errors"
	"fmt"
	"sync"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type CheckTarget struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type ResourceCheck struct {
	CheckTarget
	UID, Inactive string
	Signals       []AlertSignal
	CompleteRules []AlertRule
}

type CheckResult struct {
	Objects  []ResourceCheck
	Coverage map[string]string
}

var checkResources = map[string]kubecommon.Resource{
	"Deployment": {Name: "deployments", Group: "apps", Version: "v1", Kind: "Deployment", Namespaced: true},
	"DaemonSet":  {Name: "daemonsets", Group: "apps", Version: "v1", Kind: "DaemonSet", Namespaced: true},
	"ReplicaSet": {Name: "replicasets", Group: "apps", Version: "v1", Kind: "ReplicaSet", Namespaced: true},
	"Pod":        {Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true},
	"Service":    {Name: "services", Version: "v1", Kind: "Service", Namespaced: true},
}

func CheckTargetKinds() []string {
	return []string{"Deployment", "DaemonSet", "Service"}
}

func (c *Client) Check(ctx context.Context, targets []CheckTarget) (CheckResult, error) {
	namespace := c.Namespace
	if namespace == "" {
		return CheckResult{}, errors.New("a monitor check requires a namespace")
	}
	if err := c.checkTarget(); err != nil {
		return CheckResult{}, fmt.Errorf("verify pinned health target: %w", err)
	}
	selected := make(map[CheckTarget]bool, len(targets))
	kinds := map[string]bool{}
	for _, t := range targets {
		selected[t] = true
		kinds[t.Kind] = true
	}
	if len(targets) == 0 {
		kinds["Deployment"] = true
	}
	if kinds["Deployment"] {
		kinds["ReplicaSet"] = true
	}
	if kinds["Deployment"] || kinds["DaemonSet"] {
		kinds["Pod"] = true
	}

	type read struct {
		kind string
		data Collection
		err  error
	}
	results := make(chan read, len(kinds))
	for kind := range kinds {
		go func() {
			data, err := c.List(ctx, checkResources[kind], namespace, "")
			results <- read{kind, data, err}
		}()
	}
	check := CheckResult{Coverage: map[string]string{}}
	inventory := make(map[string][]unstructured.Unstructured, len(kinds))
	for range kinds {
		r := <-results
		switch {
		case r.err != nil:
			check.Coverage[r.kind] = ErrorText(r.err)
		case r.data.Truncated:
			check.Coverage[r.kind] = "Inventory is truncated."
		default:
			inventory[r.kind] = r.data.Objects
			for _, o := range r.data.Objects {
				if o.GetNamespace() != namespace || o.GetUID() == "" {
					delete(inventory, r.kind)
					check.Coverage[r.kind] = "Inventory contains an object without a verified identity."
					break
				}
			}
		}
	}

	replicaOwners := map[string]string{}
	for _, rs := range inventory["ReplicaSet"] {
		replicaOwners[string(rs.GetUID())] = controllerUID(rs, "Deployment")
	}
	pods := map[string][]unstructured.Unstructured{}
	for _, pod := range inventory["Pod"] {
		if pod.GetDeletionTimestamp() != nil || terminalPod(pod) {
			continue
		}
		uid := controllerUID(pod, "DaemonSet")
		if uid == "" {
			rs := controllerUID(pod, "ReplicaSet")
			uid = replicaOwners[rs]
			if rs != "" && kinds["Deployment"] {
				if _, found := replicaOwners[rs]; !found {
					check.Coverage["Deployment Pods"] = "A Pod's owning ReplicaSet was absent from the check."
				}
			}
		}
		if uid != "" {
			pods[uid] = append(pods[uid], pod)
		}
	}

	var services []unstructured.Unstructured
	for _, service := range inventory["Service"] {
		if selected[CheckTarget{"Service", service.GetName()}] {
			services = append(services, service)
		}
	}
	readiness := c.checkServiceReadiness(ctx, namespace, services)
	found := map[CheckTarget]bool{}
	for _, kind := range CheckTargetKinds() {
		for _, o := range inventory[kind] {
			target := CheckTarget{kind, o.GetName()}
			if len(selected) != 0 && !selected[target] {
				continue
			}
			found[target] = true
			obj := ResourceCheck{CheckTarget: target, UID: string(o.GetUID())}
			podsComplete := check.Coverage["Pod"] == "" &&
				(kind == "DaemonSet" || check.Coverage["ReplicaSet"] == "" && check.Coverage["Deployment Pods"] == "")
			evaluateAlertRules(&check, &obj, o, readiness[obj.UID], pods[obj.UID], podsComplete)
			check.Objects = append(check.Objects, obj)
		}
	}
	for target := range selected {
		if !found[target] && check.Coverage[target.Kind] == "" {
			check.Coverage[target.Kind+"/"+target.Name] = "Selected resource is absent."
		}
	}
	if err := c.checkTarget(); err != nil {
		return CheckResult{}, fmt.Errorf("verify pinned health target: %w", err)
	}
	return check, nil
}

func (c *Client) checkServiceReadiness(ctx context.Context, namespace string, services []unstructured.Unstructured) map[string]EndpointReadiness {
	result := make(map[string]EndpointReadiness, len(services))
	var workers sync.WaitGroup
	var mu sync.Mutex
	for _, service := range services {
		workers.Go(func() {
			resource := kubecommon.Resource{Name: "endpointslices", Group: "discovery.k8s.io", Version: "v1", Namespaced: true}
			r := c.serviceEndpointReadiness(ctx, []unstructured.Unstructured{service}, resource, true, namespace, "kubernetes.io/service-name="+service.GetName())
			mu.Lock()
			result[string(service.GetUID())] = r[string(service.GetUID())]
			mu.Unlock()
		})
	}
	workers.Wait()
	return result
}

func controllerUID(object unstructured.Unstructured, kind string) string {
	for _, owner := range object.GetOwnerReferences() {
		if owner.Kind == kind && groupOf(owner.APIVersion) == "apps" && owner.Controller != nil && *owner.Controller {
			return string(owner.UID)
		}
	}
	return ""
}
