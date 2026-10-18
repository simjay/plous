package monitor

import (
	"context"
	"encoding/base64"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestEveryCatalogResourceHasNativeViewsWithoutBorrowedSemantics(t *testing.T) {
	for identity := range resourceCatalog {
		name, group, _ := strings.Cut(identity, ".")
		r := kubecommon.Resource{Name: name, Group: group, Kind: ResourceTitle(kubecommon.Resource{Name: name, Group: group})}
		t.Run(identity, func(t *testing.T) {
			if !HasPresenter(r) {
				t.Fatal("catalog resource has no native columns")
			}
			object := summaryFixture(t, `{"metadata":{"name":"fixture","creationTimestamp":"2026-09-29T12:00:00Z"},"status":{"conditions":[{"type":"Example","status":"Unknown","lastTransitionTime":"2026-09-30T01:00:00Z"}]}}`)
			columns, rows := Table(r, Collection{Objects: []unstructured.Unstructured{object}}, time.Now())
			if len(columns) < 3 || len(rows) != 1 || len(rows[0].Cells) != len(columns) {
				t.Fatalf("invalid table: %v %#v", columns, rows)
			}
			if columns[0] != "Name" || !slices.Contains(columns, "Age") {
				t.Fatalf("identity/age missing: %v", columns)
			}
			text := summaryText(r, object)
			if strings.Contains(text, "Resource details\nYAML: Open YAML") {
				t.Fatal("catalog resource fell through to generic summary")
			}
			if strings.Contains(text, "map[") {
				t.Fatal("raw map in structured summary")
			}
			extension := r
			extension.Group = "unrelated.example"
			if HasPresenter(extension) {
				t.Fatal("unrelated extension inherited native presenter")
			}
		})
	}
	for identity := range nativeColumns {
		if _, ok := resourceCatalog[identity]; !ok {
			t.Errorf("presenter %s is absent from the catalog", identity)
		}
	}
}

func TestNativeTablesKeepAbsentStatusUnknownAndDistinctEventAPIs(t *testing.T) {
	deployment := summaryFixture(t, `{"metadata":{"name":"checkout"},"spec":{"replicas":3}}`)
	columns, rows := Table(kubecommon.Resource{Name: "deployments", Group: "apps"}, Collection{Objects: []unstructured.Unstructured{deployment}}, time.Now())
	for i, column := range columns {
		switch column {
		case "Ready":
			if rows[0].Cells[i] != "?/3" {
				t.Fatal(rows[0].Cells)
			}
		case "Available", "Updated", "Progressing":
			if rows[0].Cells[i] != "Not reported" {
				t.Fatal(rows[0].Cells)
			}
		}
	}
	event := summaryFixture(t, `{"metadata":{"name":"failed"},"type":"Warning","reason":"FailedMount","regarding":{"kind":"Pod","name":"checkout"},"note":"volume unavailable","series":{"count":4,"lastObservedTime":"2026-09-30T01:00:00Z"}}`)
	columns, rows = Table(kubecommon.Resource{Name: "events", Group: "events.k8s.io"}, Collection{Objects: []unstructured.Unstructured{event}}, time.Now())
	values := map[string]string{}
	for i, column := range columns {
		values[column] = rows[0].Cells[i]
	}
	if values["Regarding"] != "Pod/checkout" || values["Count"] != "4" || values["Note"] != "volume unavailable" {
		t.Fatal(values)
	}
}

func TestGatewayRoutesPreserveParentSpecificConditionsAndProtocolFields(t *testing.T) {
	route := summaryFixture(t, `{"kind":"GRPCRoute","metadata":{"name":"rpc"},"spec":{"hostnames":["rpc.example.com"],"parentRefs":[{"name":"edge","sectionName":"https"}],"rules":[{"matches":[{"method":{"type":"Exact","service":"checkout.Cart","method":"Get"}}],"backendRefs":[{"name":"cart","port":9000,"weight":2}]}]},"status":{"parents":[{"parentRef":{"name":"edge"},"controllerName":"example.net/controller","conditions":[{"type":"Accepted","status":"True"}]},{"parentRef":{"name":"internal"},"conditions":[{"type":"Accepted","status":"False","reason":"NotAllowedByListeners"}]}]}}`)
	text := summaryText(kubecommon.Resource{Name: "grpcroutes", Group: "gateway.networking.k8s.io", Kind: "GRPCRoute"}, route)
	for _, want := range []string{"checkout.Cart", "Get", "cart · 9000 · 2", "Parent conditions · edge", "Parent conditions · internal", "NotAllowedByListeners", "example.net/controller"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %s", want, text)
		}
	}
	if strings.Contains(text, "HTTP path") || strings.Contains(text, "Ready:") {
		t.Fatal("route inherited unsupported HTTP fields or synthetic global readiness")
	}
}

func TestStorageAndAllocationSummariesRetainVersionSpecificFields(t *testing.T) {
	cases := []struct {
		key, fixture string
		want         []string
	}{
		{"csinodes.storage.k8s.io", `{"spec":{"drivers":[{"name":"disk.example.io","nodeID":"worker-19","topologyKeys":["zone"],"allocatable":{"count":12}}]}}`, []string{"disk.example.io · worker-19 · zone · 12"}},
		{"volumesnapshotcontents.snapshot.storage.k8s.io", `{"spec":{"driver":"disk.example.io","deletionPolicy":"Retain","volumeSnapshotRef":{"name":"backup","namespace":"shop","uid":"snapshot-1"},"source":{"snapshotHandle":"provider-id"}},"status":{"readyToUse":false,"error":{"message":"backend unavailable"}}}`, []string{"Retain", "snapshot-1", "provider-id", "backend unavailable", "Ready to use: false"}},
		{"resourceclaims.resource.k8s.io", `{"spec":{"devices":{"requests":[{"name":"gpu","exactly":{"deviceClassName":"accelerator","selectors":[{"cel":{"expression":"device.driver == 'gpu.example.io'"}}]}}]}},"status":{"allocation":{"devices":{"results":[{"request":"gpu","driver":"gpu.example.io","pool":"worker","device":"gpu-1"}]}},"reservedFor":[{"resource":"pods","name":"trainer","uid":"pod-1"}]}}`, []string{"accelerator", "device.driver", "gpu.example.io · worker · gpu-1", "trainer · pod-1"}},
		{"leasecandidates.coordination.k8s.io", `{"spec":{"leaseName":"scheduler","binaryVersion":"1.37.0","emulationVersion":"1.36.0","strategy":"OldestEmulationVersion"}}`, []string{"Lease: scheduler", "Strategy: OldestEmulationVersion"}},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			name, group, _ := strings.Cut(tc.key, ".")
			text := summaryText(kubecommon.Resource{Name: name, Group: group}, summaryFixture(t, tc.fixture))
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q in %s", want, text)
				}
			}
			if strings.Contains(text, "map[") {
				t.Fatal(text)
			}
		})
	}
}

func TestQuotaQuantitiesAndConfigKeySizesAreNotRawCounts(t *testing.T) {
	quota := summaryFixture(t, `{"spec":{"hard":{"requests.cpu":"2","requests.memory":"1Gi"}},"status":{"hard":{"requests.cpu":"2","requests.memory":"1Gi"},"used":{"requests.cpu":"2500m","requests.memory":"256Mi"}}}`)
	text := summaryText(kubecommon.Resource{Name: "resourcequotas"}, quota)
	for _, want := range []string{"requests.cpu · -500m", "requests.memory · 768Mi"} {
		if !strings.Contains(text, want) {
			t.Error(text)
		}
	}
	if quotaExceeded(quota.Object) != "1" {
		t.Fatal(quotaExceeded(quota.Object))
	}
	delete(quota.Object["status"].(map[string]any)["used"].(map[string]any), "requests.cpu")
	if quotaExceeded(quota.Object) != "Not reported" {
		t.Fatal("missing usage treated as zero")
	}
	secret := summaryFixture(t, `{"kind":"Secret","data":{"token":"`+base64.StdEncoding.EncodeToString([]byte("private"))+`"}}`)
	text = summaryText(kubecommon.Resource{Name: "secrets"}, secret)
	if strings.Contains(text, "private") || !strings.Contains(text, "token · data · 7") {
		t.Fatal(text)
	}
}

func TestIngressRelationshipsUseBackendAndTLSIdentities(t *testing.T) {
	ingress := summaryFixture(t, `{"kind":"Ingress","metadata":{"name":"checkout","namespace":"shop"},"spec":{"ingressClassName":"public","rules":[{"http":{"paths":[{"backend":{"service":{"name":"checkout","port":{"number":80}}}}]}}],"tls":[{"secretName":"checkout-tls"}]}}`)
	resources := []kubecommon.Resource{{Name: "services", Kind: "Service", Namespaced: true}, {Name: "secrets", Kind: "Secret", Namespaced: true}, {Name: "ingressclasses", Group: "networking.k8s.io", Kind: "IngressClass"}}
	var client *Client
	refs, warning := client.Related(context.Background(), kubecommon.Resource{Name: "ingresses", Group: "networking.k8s.io", Kind: "Ingress", Namespaced: true}, ingress, resources)
	if warning != "" || len(refs) != 3 {
		t.Fatalf("refs=%#v warnings=%s", refs, warning)
	}
	identities := map[string]string{}
	for _, ref := range refs {
		identities[ref.Resource.Key()+"/"+ref.Name] = ref.Namespace
	}
	if identities["services/checkout"] != "shop" || identities["secrets/checkout-tls"] != "shop" {
		t.Fatal(identities)
	}
}

func TestEventRelationshipsCarryUIDAndDoNotGuessUserGroupResources(t *testing.T) {
	event := summaryFixture(t, `{"metadata":{"name":"event","namespace":"shop"},"regarding":{"kind":"Pod","apiVersion":"v1","name":"checkout","namespace":"shop","uid":"old-pod"}}`)
	var client *Client
	refs, warning := client.Related(context.Background(), kubecommon.Resource{Name: "events", Group: "events.k8s.io", Namespaced: true}, event, []kubecommon.Resource{{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true}})
	if warning != "" || len(refs) != 1 || refs[0].UID != "old-pod" {
		t.Fatalf("refs=%#v warning=%s", refs, warning)
	}
	binding := summaryFixture(t, `{"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"reader"},"subjects":[{"kind":"User","name":"jae"},{"kind":"Group","name":"team"},{"kind":"ServiceAccount","name":"agent","namespace":"shop"}],"metadata":{"namespace":"shop"}}`)
	refs, warning = client.Related(context.Background(), kubecommon.Resource{Name: "rolebindings", Group: "rbac.authorization.k8s.io", Namespaced: true}, binding, []kubecommon.Resource{{Name: "roles", Group: "rbac.authorization.k8s.io", Kind: "Role", Namespaced: true}, {Name: "serviceaccounts", Kind: "ServiceAccount", Namespaced: true}})
	if warning != "" || len(refs) != 2 {
		t.Fatalf("refs=%#v warning=%s", refs, warning)
	}
}

func TestServiceSelectorRelationsAreNotReportedAsOwnership(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/shop/pods" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		reply(w, objectList("Pod", "1", map[string]any{"metadata": map[string]any{"name": "selected", "namespace": "shop", "uid": "pod-1", "labels": map[string]any{"app": "checkout"}}}, map[string]any{"metadata": map[string]any{"name": "other", "namespace": "shop", "labels": map[string]any{"app": "different"}}}))
	})
	service := summaryFixture(t, `{"metadata":{"name":"checkout","namespace":"shop"},"spec":{"selector":{"app":"checkout"}}}`)
	refs, warning := client.Related(context.Background(), kubecommon.Resource{Name: "services", Kind: "Service", Namespaced: true}, service, []kubecommon.Resource{{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true}})
	if len(refs) != 1 || refs[0].Name != "selected" || !strings.Contains(refs[0].Relation, "selector match") || strings.Contains(refs[0].Relation, "Owned") {
		t.Fatalf("refs=%#v warning=%s", refs, warning)
	}
	if !strings.Contains(warning, "endpointslices.discovery.k8s.io API unavailable") {
		t.Fatal(warning)
	}
}

func TestListRelationshipsPreserveScopeAndDiscoveredIdentity(t *testing.T) {
	resources := []kubecommon.Resource{{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true}, {Name: "nodes", Version: "v1", Kind: "Node"}, {Name: "widgets", Group: "example.io", Version: "v1", Kind: "Widget", Namespaced: true}}
	namespace := summaryFixture(t, `{"metadata":{"name":"shop"}}`)
	refs := resourceListReferences(kubecommon.Resource{Name: "namespaces"}, namespace, resources)
	if len(refs) != 2 {
		t.Fatal(refs)
	}
	for _, ref := range refs {
		if !ref.List || ref.Name != "" || ref.Namespace != "shop" || !ref.Resource.Namespaced {
			t.Fatal(ref)
		}
	}
	crd := summaryFixture(t, `{"spec":{"group":"example.io","names":{"plural":"widgets"}}}`)
	refs = resourceListReferences(kubecommon.Resource{Name: "customresourcedefinitions", Group: "apiextensions.k8s.io"}, crd, resources)
	if len(refs) != 1 || !refs[0].List || refs[0].Resource.Key() != "widgets.example.io" {
		t.Fatal(refs)
	}
	crd.Object["spec"].(map[string]any)["group"] = "unrelated.example"
	if refs := resourceListReferences(kubecommon.Resource{Name: "customresourcedefinitions", Group: "apiextensions.k8s.io"}, crd, resources); len(refs) != 0 {
		t.Fatal("unknown API name borrowed a served resource")
	}
}

func TestWorkloadTemplatesExposeConfigurationVolumesMountsAndProbes(t *testing.T) {
	object := summaryFixture(t, `{"kind":"Deployment","spec":{"template":{"metadata":{"labels":{"app":"checkout"}},"spec":{"containers":[{"name":"app","image":"checkout:v2","envFrom":[{"configMapRef":{"name":"settings"}}],"readinessProbe":{"httpGet":{"path":"/ready","port":8080}},"volumeMounts":[{"name":"config","mountPath":"/etc/app","readOnly":true}]}],"volumes":[{"name":"config","configMap":{"name":"settings"}}]}}}}`)
	text := summaryText(kubecommon.Resource{Name: "deployments", Group: "apps", Kind: "Deployment"}, object)
	for _, want := range []string{"checkout:v2", "Environment references · app", "configMapRef · settings", "Pod template volumes", "config · configMap · settings", "Template mounts · app", "config · /etc/app · true", "Readiness: HTTP Pod IP:8080/ready", "app · checkout"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %s", want, text)
		}
	}
}
