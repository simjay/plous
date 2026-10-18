package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
)

type operationError string

func (e operationError) Error() string { return string(e) }

func TargetChangedError() error {
	return operationError("This object was replaced. Reopen its details.")
}

type Access struct {
	Known, Allowed bool
	Reason         string
}

func (c *Client) CheckAccess(ctx context.Context, r kubecommon.Resource, ns, name, verb, subresource string) (Access, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	if !r.Namespaced {
		ns = ""
	}
	review := map[string]any{
		"apiVersion": "authorization.k8s.io/v1", "kind": "SelfSubjectAccessReview",
		"spec": map[string]any{"resourceAttributes": map[string]string{
			"group": r.Group, "version": r.Version, "resource": r.Name,
			"subresource": subresource, "namespace": ns, "name": name, "verb": verb,
		}},
	}
	body, err := json.Marshal(review)
	if err != nil {
		return Access{}, err
	}
	data, err := c.rest.Post().AbsPath("/apis/authorization.k8s.io/v1/selfsubjectaccessreviews").SetHeader("Content-Type", "application/json").Body(body).Do(ctx).Raw()
	if err != nil {
		return Access{Reason: "Permission could not be checked. The operation response remains authoritative."}, err
	}
	var result struct {
		Status struct {
			Allowed         *bool
			Denied          bool
			EvaluationError string
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return Access{Reason: "Permission check returned an unreadable response."}, err
	}
	allowed := result.Status.Allowed != nil && *result.Status.Allowed
	access := Access{Known: allowed || result.Status.Denied || result.Status.Allowed != nil && result.Status.EvaluationError == "", Allowed: allowed}
	switch {
	case access.Allowed:
		access.Reason = "Allowed by Kubernetes RBAC."
	case access.Known:
		access.Reason = "Denied by Kubernetes RBAC."
	default:
		access.Reason = "Permission was not decided. The operation response remains authoritative."
	}
	return access, nil
}

func validPathPart(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\?#%\x00\r\n")
}

func validateTarget(r kubecommon.Resource, object *unstructured.Unstructured) error {
	if object == nil || !validPathPart(object.GetName()) || !validPathPart(r.Name) || strings.Contains(r.Name, "/") {
		return operationError("The operation has no valid resource target.")
	}
	if r.Namespaced && !validPathPart(object.GetNamespace()) || !r.Namespaced && object.GetNamespace() != "" {
		return operationError("The resource namespace does not match its API scope.")
	}
	if expected := r.GVR().GroupVersion().String(); object.GetAPIVersion() != expected {
		return operationError(fmt.Sprintf("apiVersion must be %q for this resource. Restore that field before saving.", expected))
	}
	if object.GetKind() != r.Kind {
		return operationError(fmt.Sprintf("kind must be %q for this resource. Restore that field before saving.", r.Kind))
	}
	if object.GetUID() == "" || object.GetResourceVersion() == "" {
		return operationError("Refresh this object to capture its UID and resource version before changing it.")
	}
	return nil
}

func validateEdit(r kubecommon.Resource, original, proposed *unstructured.Unstructured) error {
	if err := validateTarget(r, original); err != nil {
		return err
	}
	if err := validateTarget(r, proposed); err != nil {
		return err
	}
	if proposed.GetName() != original.GetName() || proposed.GetNamespace() != original.GetNamespace() || proposed.GetUID() != original.GetUID() || proposed.GetResourceVersion() != original.GetResourceVersion() {
		return operationError("Name, namespace, UID, and resource version must stay unchanged. Reopen Edit to load a newer object.")
	}
	if !reflect.DeepEqual(original.Object["status"], proposed.Object["status"]) {
		return operationError("Status is reported by Kubernetes and cannot be edited here.")
	}
	if r.Key() == "secrets" {
		for _, field := range []string{"data", "stringData"} {
			values, _, _ := unstructured.NestedStringMap(proposed.Object, field)
			for _, value := range values {
				if value == "<hidden>" {
					return operationError("Masked Secret values cannot be applied. Reopen Edit with the full object.")
				}
			}
		}
	}
	if err := ValidateResourceEdit(r, original, proposed); err != nil {
		return operationError(err.Error())
	}
	return nil
}

func ParseManifest(r kubecommon.Resource, original *unstructured.Unstructured, text string) (*unstructured.Unstructured, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(text), 4096)
	var object unstructured.Unstructured
	if err := decoder.Decode(&object); err != nil || object.Object == nil {
		// The decoder fills the object before rejecting a missing kind.
		if object.Object != nil && object.GetKind() == "" {
			return nil, operationError(fmt.Sprintf("kind must be %q for this resource. Restore that field before saving.", r.Kind))
		}
		return nil, operationError("The manifest must contain one valid YAML or JSON object.")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, operationError("Edit accepts one resource object, without additional YAML documents.")
	}
	if err := validateEdit(r, original, &object); err != nil {
		return nil, err
	}
	return &object, nil
}

func (c *Client) ValidateEdit(ctx context.Context, r kubecommon.Resource, original, proposed *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.update(ctx, r, original, proposed, true)
}

func (c *Client) ApplyEdit(ctx context.Context, r kubecommon.Resource, original, proposed *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.update(ctx, r, original, proposed, false)
}

func (c *Client) update(ctx context.Context, r kubecommon.Resource, original, proposed *unstructured.Unstructured, dryRun bool) (*unstructured.Unstructured, error) {
	if err := c.checkTarget(); err != nil {
		return nil, err
	}
	if !r.Supports("update", "") {
		return nil, operationError("This API does not advertise resource updates.")
	}
	if err := validateEdit(r, original, proposed); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	options := metav1.UpdateOptions{FieldManager: "plous", FieldValidation: "Strict"}
	if dryRun {
		options.DryRun = []string{metav1.DryRunAll}
	}
	return c.resource(r, original.GetNamespace()).Update(ctx, proposed.DeepCopy(), options)
}

func (c *Client) Delete(ctx context.Context, r kubecommon.Resource, object *unstructured.Unstructured) error {
	if err := c.checkTarget(); err != nil {
		return err
	}
	if !r.Supports("delete", "") {
		return operationError("This API does not advertise resource deletion.")
	}
	if err := validateTarget(r, object); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	uid := object.GetUID()
	return c.resource(r, object.GetNamespace()).Delete(ctx, object.GetName(), metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
}

func (c *Client) GetScale(ctx context.Context, r kubecommon.Resource, object *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if !r.Supports("get", "scale") || !r.Supports("update", "scale") {
		return nil, operationError("This API does not advertise a readable, writable scale subresource.")
	}
	if err := validateTarget(r, object); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	scale, err := c.resource(r, object.GetNamespace()).Get(ctx, object.GetName(), metav1.GetOptions{}, "scale")
	if err == nil {
		err = validateScale(object, scale)
	}
	return scale, err
}

func validateScale(object, scale *unstructured.Unstructured) error {
	if scale == nil || scale.GetUID() != object.GetUID() || scale.GetName() != object.GetName() || scale.GetNamespace() != object.GetNamespace() || scale.GetResourceVersion() == "" {
		return operationError("The scale target changed or has no resource version. Reopen Scale to refresh it.")
	}
	return nil
}

func (c *Client) Scale(ctx context.Context, r kubecommon.Resource, object, currentScale *unstructured.Unstructured, replicas int64) (*unstructured.Unstructured, error) {
	if err := c.checkTarget(); err != nil {
		return nil, err
	}
	if !r.Supports("update", "scale") {
		return nil, operationError("This API does not advertise a writable scale subresource.")
	}
	if err := validateTarget(r, object); err != nil {
		return nil, err
	}
	if err := validateScale(object, currentScale); err != nil {
		return nil, err
	}
	if replicas < 0 || replicas > 2147483647 {
		return nil, operationError("Replicas must be a whole number between 0 and 2147483647.")
	}
	scale := currentScale.DeepCopy()
	if err := unstructured.SetNestedField(scale.Object, replicas, "spec", "replicas"); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return c.resource(r, object.GetNamespace()).Update(ctx, scale, metav1.UpdateOptions{FieldManager: "plous"}, "scale")
}

func (c *Client) Restart(ctx context.Context, r kubecommon.Resource, object *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if err := c.checkTarget(); err != nil {
		return nil, err
	}
	if r.Key() != "deployments.apps" && r.Key() != "statefulsets.apps" && r.Key() != "daemonsets.apps" {
		return nil, operationError("Restart rollout is available only for Deployments, StatefulSets, and DaemonSets.")
	}
	if !r.Supports("patch", "") {
		return nil, operationError("This API does not advertise patch support.")
	}
	if err := validateTarget(r, object); err != nil {
		return nil, err
	}
	if r.Key() == "deployments.apps" {
		paused, _, _ := unstructured.NestedBool(object.Object, "spec", "paused")
		if paused {
			return nil, operationError("This Deployment is paused. Resume it before requesting a restart rollout.")
		}
	}
	metadata, exists, err := unstructured.NestedMap(object.Object, "spec", "template", "metadata")
	if err != nil {
		return nil, operationError("The workload Pod template metadata is not valid.")
	}
	annotations, _, _ := unstructured.NestedStringMap(metadata, "annotations")
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	patch := []map[string]any{
		{"op": "test", "path": "/metadata/uid", "value": string(object.GetUID())},
		{"op": "test", "path": "/metadata/resourceVersion", "value": object.GetResourceVersion()},
	}
	if !exists {
		patch = append(patch, map[string]any{"op": "add", "path": "/spec/template/metadata", "value": map[string]any{}})
	}
	patch = append(patch, map[string]any{"op": "add", "path": "/spec/template/metadata/annotations", "value": annotations})
	data, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	updated, err := c.resource(r, object.GetNamespace()).Patch(ctx, object.GetName(), types.JSONPatchType, data, metav1.PatchOptions{FieldManager: "plous"})
	// JSON Patch test failures are returned as BadRequest, including a version
	// changed by the controller between confirmation and submission.
	if apierrors.IsBadRequest(err) {
		current, readErr := c.Get(ctx, r, object.GetNamespace(), object.GetName())
		if readErr == nil {
			if current.GetUID() != object.GetUID() {
				return nil, TargetChangedError()
			}
			if current.GetResourceVersion() != object.GetResourceVersion() {
				return nil, apierrors.NewConflict(r.GVR().GroupResource(), object.GetName(), errors.New("the confirmed resource version changed"))
			}
		}
	}
	return updated, err
}

func OperationErrorText(err error) string {
	var local operationError
	if errors.As(err, &local) {
		return string(local)
	}
	switch {
	case apierrors.IsConflict(err):
		return "The resource changed since it was opened. Refresh and review the change again."
	case apierrors.IsInvalid(err):
		return "Kubernetes rejected this change. Check immutable fields, value types, and required fields in the manifest."
	case apierrors.IsBadRequest(err):
		return "Kubernetes rejected the operation. Check the manifest or operation settings and refresh the target."
	case apierrors.IsForbidden(err):
		return "Kubernetes denied this operation. Check RBAC permissions and admission policies."
	case apierrors.IsMethodNotSupported(err):
		return "This operation is not supported by the selected API. Refresh discovery."
	default:
		return ErrorText(err)
	}
}

func RestartNotice(r kubecommon.Resource, object unstructured.Unstructured) string {
	if r.Key() == "statefulsets.apps" || r.Key() == "daemonsets.apps" {
		strategy, _, _ := unstructured.NestedString(object.Object, "spec", "updateStrategy", "type")
		if strategy == "OnDelete" {
			return "OnDelete updates the Pod template only. Existing Pods are not automatically replaced."
		}
		if r.Key() == "statefulsets.apps" {
			partition, found, _ := unstructured.NestedInt64(object.Object, "spec", "updateStrategy", "rollingUpdate", "partition")
			if found && partition > 0 {
				return fmt.Sprintf("RollingUpdate partition %d limits which Pods are replaced. Acceptance does not mean the rollout has completed.", partition)
			}
		}
	}
	return "Updates the Pod template to request a rollout. Kubernetes follows the workload update strategy; acceptance does not mean Pods are ready."
}
