package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func editableDeployment() (kubecommon.Resource, *unstructured.Unstructured) {
	r := kubecommon.Resource{Group: "apps", Version: "v1", Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: []string{"get", "list", "update", "patch", "delete"}, Subresources: map[string][]string{"scale": {"get", "update"}}}
	o := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "checkout", "namespace": "shop", "uid": "original", "resourceVersion": "10"},
		"spec":     map[string]any{"replicas": int64(2), "template": map[string]any{"metadata": map[string]any{"annotations": map[string]any{"existing": "keep"}}}},
		"status":   map[string]any{"readyReplicas": int64(2)},
	}}
	return r, o
}

func TestManifestRejectsRetargetingStatusAndMaskedSecrets(t *testing.T) {
	r, original := editableDeployment()
	for _, change := range []func(*unstructured.Unstructured){
		func(o *unstructured.Unstructured) { o.SetUID("replacement") },
		func(o *unstructured.Unstructured) { o.SetName("another") },
		func(o *unstructured.Unstructured) { o.SetNamespace("other") },
		func(o *unstructured.Unstructured) { o.SetResourceVersion("11") },
		func(o *unstructured.Unstructured) { o.SetKind("StatefulSet") },
		func(o *unstructured.Unstructured) { o.Object["status"] = map[string]any{"readyReplicas": int64(9)} },
	} {
		changed := original.DeepCopy()
		change(changed)
		if _, err := ParseManifest(r, original, YAML(*changed, true)); err == nil {
			t.Fatalf("unsafe edit accepted: %s", YAML(*changed, true))
		}
	}
	changed := original.DeepCopy()
	_ = unstructured.SetNestedField(changed.Object, int64(3), "spec", "replicas")
	if _, err := ParseManifest(r, original, YAML(*changed, true)); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseManifest(r, original, YAML(*changed, true)+"---\nkind: Pod\n"); err == nil {
		t.Fatal("multiple manifests accepted")
	}
	secret := original.DeepCopy()
	secret.SetAPIVersion("v1")
	secret.SetKind("Secret")
	secret.Object["data"] = map[string]any{"token": "c2VjcmV0"}
	sr := kubecommon.Resource{Version: "v1", Name: "secrets", Kind: "Secret", Namespaced: true}
	if _, err := ParseManifest(sr, secret, YAML(*secret, false)); err == nil {
		t.Fatal("masked Secret accepted")
	}
}

func TestManifestAPIMismatchIdentifiesFieldAndRequiredValue(t *testing.T) {
	r, original := editableDeployment()
	for _, tc := range []struct{ field, value, expected string }{
		{"apiVersion", "v1", `apiVersion must be "apps/v1"`},
		{"apiVersion", "", `apiVersion must be "apps/v1"`},
		{"kind", "StatefulSet", `kind must be "Deployment"`},
		{"kind", "", `kind must be "Deployment"`},
	} {
		t.Run(tc.field+"/"+tc.value, func(t *testing.T) {
			proposed := original.DeepCopy()
			proposed.Object[tc.field] = tc.value
			_, err := ParseManifest(r, original, YAML(*proposed, true))
			if err == nil || !strings.Contains(err.Error(), tc.expected) {
				t.Fatalf("missing field-specific recovery: %v", err)
			}
		})
	}
}

func TestEditValidatesBeforeApplyAndPreservesOptimisticIdentity(t *testing.T) {
	r, original := editableDeployment()
	var modes []string
	client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
		if request.Method != "PUT" || request.URL.Path != "/apis/apps/v1/namespaces/shop/deployments/checkout" {
			t.Errorf("unexpected request %s %s", request.Method, request.URL)
		}
		var body unstructured.Unstructured
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.GetUID() != original.GetUID() || body.GetResourceVersion() != "10" || request.URL.Query().Get("fieldValidation") != "Strict" {
			t.Errorf("missing edit protection: %#v %s", body, request.URL)
		}
		modes = append(modes, request.URL.Query().Get("dryRun"))
		reply(w, body.Object)
	})
	proposed := original.DeepCopy()
	proposed.SetLabels(map[string]string{"reviewed": "yes"})
	if _, err := client.ValidateEdit(context.Background(), r, original, proposed); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ApplyEdit(context.Background(), r, original, proposed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(modes, []string{"All", ""}) || original.GetLabels()["reviewed"] != "" {
		t.Fatalf("unexpected requests or modified original: %v", modes)
	}
}

func TestDeleteUsesUIDPreconditionWithoutForce(t *testing.T) {
	r, original := editableDeployment()
	client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
		var options metav1.DeleteOptions
		if err := json.NewDecoder(request.Body).Decode(&options); err != nil {
			t.Error(err)
		}
		if request.Method != "DELETE" || options.Preconditions == nil || options.Preconditions.UID == nil || *options.Preconditions.UID != "original" || options.GracePeriodSeconds != nil {
			t.Errorf("delete lost graceful exact-target semantics: %#v", options)
		}
		reply(w, metav1.Status{Status: "Success"})
	})
	if err := client.Delete(context.Background(), r, original); err != nil {
		t.Fatal(err)
	}
}

func TestScaleCapturesScaleVersionAndRejectsReplacement(t *testing.T) {
	r, original := editableDeployment()
	var puts atomic.Int32
	scale := original.DeepCopy()
	scale.SetAPIVersion("autoscaling/v1")
	scale.SetKind("Scale")
	scale.SetResourceVersion("11")
	client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
		if !strings.HasSuffix(request.URL.Path, "/deployments/checkout/scale") {
			t.Errorf("wrong scale URL: %s", request.URL)
		}
		if request.Method == "PUT" {
			puts.Add(1)
			var body unstructured.Unstructured
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body.GetResourceVersion() != "11" || Value(body.Object, "spec", "replicas") != "4" || body.GetUID() != "original" {
				t.Errorf("bad Scale update: %#v", body)
			}
		}
		reply(w, scale.Object)
	})
	current, err := client.GetScale(context.Background(), r, original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Scale(context.Background(), r, original, current, 4); err != nil {
		t.Fatal(err)
	}
	current.SetUID("replacement")
	if _, err := client.Scale(context.Background(), r, original, current, 5); err == nil || puts.Load() != 1 {
		t.Fatal("scale retargeted a replacement")
	}
	if _, err := client.Scale(context.Background(), r, original, scale, -1); err == nil {
		t.Fatal("negative replicas accepted")
	}
}

func TestRestartUsesUIDAndVersionTestsAndPreservesAnnotations(t *testing.T) {
	r, original := editableDeployment()
	var requests atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		var patch []map[string]any
		_ = json.NewDecoder(request.Body).Decode(&patch)
		if request.Method != "PATCH" || request.Header.Get("Content-Type") != "application/json-patch+json" || len(patch) != 3 || patch[0]["value"] != "original" || patch[1]["value"] != "10" {
			t.Errorf("restart missing guards: %#v", patch)
		}
		annotations := patch[2]["value"].(map[string]any)
		if annotations["existing"] != "keep" || annotations["kubectl.kubernetes.io/restartedAt"] == "" {
			t.Errorf("restart lost annotations: %#v", patch)
		}
		reply(w, original.Object)
	})
	if _, err := client.Restart(context.Background(), r, original); err != nil {
		t.Fatal(err)
	}
	for _, resource := range []kubecommon.Resource{{Name: "pods"}, {Group: "example.io", Name: "deployments"}, {Group: "apps", Name: "replicasets"}} {
		if _, err := client.Restart(context.Background(), resource, original); err == nil {
			t.Fatalf("unsupported restart enabled: %#v", resource)
		}
	}
	_ = unstructured.SetNestedField(original.Object, true, "spec", "paused")
	if _, err := client.Restart(context.Background(), r, original); err == nil || requests.Load() != 1 {
		t.Fatal("paused Deployment restart was submitted")
	}
}

func TestAccessDistinguishesDeniedUnknownAndTransportFailure(t *testing.T) {
	r, original := editableDeployment()
	for _, scenario := range []string{"allowed", "denied", "no-opinion", "unknown", "evaluation-error", "unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
				var review map[string]any
				_ = json.NewDecoder(request.Body).Decode(&review)
				attributes := review["spec"].(map[string]any)["resourceAttributes"].(map[string]any)
				if attributes["namespace"] != "shop" || attributes["subresource"] != "scale" || attributes["name"] != "checkout" || attributes["verb"] != "update" {
					t.Errorf("wrong permission target: %#v", review)
				}
				if scenario == "unavailable" {
					apiError(w, http.StatusForbidden, "Forbidden")
					return
				}
				status := map[string]any{"allowed": scenario == "allowed", "denied": scenario == "denied"}
				if scenario == "unknown" {
					delete(status, "allowed")
				}
				if scenario == "evaluation-error" {
					status["evaluationError"] = "server-private-value"
				}
				reply(w, map[string]any{"status": status})
			})
			access, err := client.CheckAccess(context.Background(), r, original.GetNamespace(), original.GetName(), "update", "scale")
			if access.Allowed != (scenario == "allowed") || access.Known != (scenario == "allowed" || scenario == "denied" || scenario == "no-opinion") || (err != nil) != (scenario == "unavailable") || strings.Contains(access.Reason, "server-private-value") {
				t.Fatalf("access=%#v err=%v", access, err)
			}
		})
	}
}

func TestOperationErrorsDoNotExposeServerValues(t *testing.T) {
	r, original := editableDeployment()
	for _, scenario := range []struct {
		code   int
		reason string
	}{{409, "Conflict"}, {422, "Invalid"}, {403, "Forbidden"}, {400, "BadRequest"}} {
		client := testClient(t, func(w http.ResponseWriter, request *http.Request) { apiError(w, scenario.code, scenario.reason) })
		_, err := client.ApplyEdit(context.Background(), r, original, original.DeepCopy())
		message := OperationErrorText(err)
		if err == nil || message == "" || strings.Contains(message, "server-private-value") {
			t.Fatalf("unsafe error: %q %v", message, err)
		}
	}
}

func TestRestartStaleJSONPatchExplainsConflictWithoutResubmitting(t *testing.T) {
	r, original := editableDeployment()
	var patches atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
		if request.Method == "PATCH" {
			patches.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			reply(w, map[string]any{"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "BadRequest", "code": 400})
			return
		}
		current := original.DeepCopy()
		current.SetResourceVersion("11")
		reply(w, current.Object)
	})
	_, err := client.Restart(context.Background(), r, original)
	if !apierrors.IsConflict(err) || patches.Load() != 1 {
		t.Fatalf("stale patch: %v, writes=%d", err, patches.Load())
	}
}
