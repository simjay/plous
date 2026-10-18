package gitops_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	argomonitor "github.com/simjay/plous/internal/integrations/argo/monitor"
	fluxmonitor "github.com/simjay/plous/internal/integrations/flux/monitor"
	. "github.com/simjay/plous/internal/integrations/gitops/common"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utiljson "k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/rest"
)

var kustomizations = kubecommon.Resource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Name: "kustomizations", Kind: "Kustomization", Namespaced: true, Verbs: []string{"list", "get"}}
var helmreleases = kubecommon.Resource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Name: "helmreleases", Kind: "HelmRelease", Namespaced: true, Verbs: []string{"list", "get"}}

type fixtureReader struct {
	catalog   []kubecommon.Resource
	warning   string
	discovery error
	objects   map[string]kubernetes.Collection
	failures  map[string]error
	mu        sync.Mutex
	reads     []string
}

func (f *fixtureReader) Discover(context.Context) ([]kubecommon.Resource, string, error) {
	return f.catalog, f.warning, f.discovery
}

func (f *fixtureReader) List(_ context.Context, r kubecommon.Resource, namespace, selector string) (kubernetes.Collection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, namespace+"/"+r.Key()+"?"+selector)
	return f.objects[r.Key()], f.failures[r.Key()]
}

func (f *fixtureReader) Get(_ context.Context, r kubecommon.Resource, namespace, name string) (*unstructured.Unstructured, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, namespace+"/"+r.Key()+"/"+name)
	return nil, errors.New("unexpected object read")
}

func TestSnapshotDistinguishesAccessFromEmptyAndPreservesPartialFlux(t *testing.T) {
	denied := apierrors.NewForbidden(schema.GroupResource{Group: kustomizations.Group, Resource: kustomizations.Name}, "", errors.New("private server details"))
	tests := []struct {
		name    string
		reader  fixtureReader
		state   AccessState
		items   int
		partial bool
	}{
		{name: "missing APIs", state: APIUnavailable},
		{name: "empty", reader: fixtureReader{catalog: []kubecommon.Resource{kustomizations, helmreleases}}, state: Empty},
		{name: "read denied", reader: fixtureReader{catalog: []kubecommon.Resource{kustomizations}, failures: map[string]error{kustomizations.Key(): denied}}, state: ReadDenied},
		{name: "connection unavailable", reader: fixtureReader{discovery: errors.New("private network detail")}, state: ConnectionUnavailable},
		{name: "partial kind installed", reader: fixtureReader{catalog: []kubecommon.Resource{kustomizations}, objects: map[string]kubernetes.Collection{kustomizations.Key(): {Objects: []unstructured.Unstructured{fixture(t, `{"metadata":{"name":"platform","namespace":"flux-system"}}`)}}}}, state: Partial, items: 1, partial: true},
		{name: "partial permission", reader: fixtureReader{catalog: []kubecommon.Resource{kustomizations, helmreleases}, failures: map[string]error{helmreleases.Key(): denied}}, state: Partial, partial: true},
		{name: "incomplete discovery", reader: fixtureReader{catalog: []kubecommon.Resource{kustomizations, helmreleases}, warning: "Some API groups could not be discovered."}, state: Partial, partial: true},
	}
	for i := range tests {
		test := &tests[i]
		t.Run(test.name, func(t *testing.T) {
			snapshot := fluxmonitor.New(&test.reader).Load(context.Background(), "flux-system")
			if snapshot.State != test.state || len(snapshot.Items) != test.items || snapshot.ReadAt.IsZero() {
				t.Fatalf("snapshot=%#v", snapshot)
			}
			if test.partial && len(snapshot.Issues) == 0 {
				t.Fatal("partial results lost their explanation")
			}
			if strings.Contains(snapshot.Message, "private") {
				t.Fatal("raw request errors leaked")
			}
			for _, read := range test.reader.reads {
				if !strings.HasPrefix(read, "flux-system/") {
					t.Fatalf("namespace scope lost: %q", read)
				}
			}
		})
	}
}

func TestReferenceReadRejectsSecretsAndArbitraryKindsBeforeDiscovery(t *testing.T) {
	reader := &fixtureReader{discovery: errors.New("must not discover")}
	for _, ref := range []Reference{
		{Kind: "Secret", Namespace: "argocd", Name: "cluster"},
		{Group: "example.io", Kind: "GitRepository", Namespace: "default", Name: "source"},
		{Group: "source.toolkit.fluxcd.io", Kind: "GitRepository", Name: "source"},
	} {
		_, err := fluxmonitor.New(reader).Get(context.Background(), ref)
		if err == nil || err.Error() != "This GitOps reference is not supported." || len(reader.reads) != 0 {
			t.Fatalf("reference was allowed: %#v %v", ref, err)
		}
	}
}

func TestToolInterfacesRejectOtherControllerReferences(t *testing.T) {
	reader := &fixtureReader{discovery: errors.New("must not discover")}
	fluxRef := Reference{Group: "kustomize.toolkit.fluxcd.io", Kind: "Kustomization", Namespace: "apps", Name: "app"}
	argoRef := Reference{Group: "argoproj.io", Kind: "Application", Namespace: "apps", Name: "app"}
	_, argoErr := argomonitor.New(reader).Get(context.Background(), fluxRef)
	_, fluxErr := fluxmonitor.New(reader).Get(context.Background(), argoRef)
	for _, err := range []error{argoErr, fluxErr} {
		if err == nil || err.Error() != "This GitOps reference is not supported." {
			t.Fatalf("tool read crossed its controller boundary: %v", err)
		}
	}
}

func TestUnknownGitOpsVersionsReportUnsupportedWithoutReadingObjects(t *testing.T) {
	unknown := kustomizations
	unknown.Version = "v99"
	reader := &fixtureReader{catalog: []kubecommon.Resource{unknown}}
	snapshot := fluxmonitor.New(reader).Load(context.Background(), "flux-system")
	if snapshot.State != APIUnavailable || len(reader.reads) != 0 || !strings.Contains(snapshot.Issues[0].Message, "version v99 is not supported") {
		t.Fatalf("unknown schema was read: %#v, reads %#v", snapshot, reader.reads)
	}
	_, err := fluxmonitor.New(reader).Get(context.Background(), Reference{Group: unknown.Group, Kind: unknown.Kind, Namespace: "flux-system", Name: "platform"})
	if err == nil || !strings.Contains(err.Error(), "version v99 is not supported") || len(reader.reads) != 0 {
		t.Fatalf("unknown detail schema was read: %v, reads %#v", err, reader.reads)
	}
	if !fluxmonitor.Supports(reader.catalog) {
		t.Fatal("installed but unsupported API must remain discoverable for its explanation")
	}
	deployment := kubecommon.Resource{Group: "apps", Version: "v9", Kind: "Deployment", Name: "deployments"}
	if _, ok := FindResource([]kubecommon.Resource{deployment}, Reference{Group: "apps", Kind: "Deployment"}); !ok {
		t.Fatal("generic Kubernetes resource routing was constrained by GitOps schema support")
	}
}

func TestKnownGitOpsVersionsRemainReadable(t *testing.T) {
	for _, resource := range []kubecommon.Resource{
		{Group: "argoproj.io", Kind: "Application", Version: "v1alpha1"},
		{Group: "kustomize.toolkit.fluxcd.io", Kind: "Kustomization", Version: "v1beta1"},
		{Group: "kustomize.toolkit.fluxcd.io", Kind: "Kustomization", Version: "v1beta2"},
		{Group: "kustomize.toolkit.fluxcd.io", Kind: "Kustomization", Version: "v1"},
		{Group: "helm.toolkit.fluxcd.io", Kind: "HelmRelease", Version: "v2beta1"},
		{Group: "helm.toolkit.fluxcd.io", Kind: "HelmRelease", Version: "v2beta2"},
		{Group: "helm.toolkit.fluxcd.io", Kind: "HelmRelease", Version: "v2"},
		{Group: "source.toolkit.fluxcd.io", Kind: "GitRepository", Version: "v1"},
	} {
		if !argomonitor.SupportsVersion(resource) && !fluxmonitor.SupportsVersion(resource) {
			t.Fatalf("known schema blocked: %#v: %v", resource, "unsupported")
		}
	}
}

func TestSourceMetadataUsesOnlyKubernetesGETsAndServedVersion(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			t.Errorf("unexpected mutation %s", r.Method)
		}
		var response any
		switch r.URL.Path {
		case "/apis":
			response = map[string]any{"groups": []any{map[string]any{"name": "source.toolkit.fluxcd.io", "preferredVersion": map[string]string{"groupVersion": "source.toolkit.fluxcd.io/v1", "version": "v1"}}}}
		case "/api/v1":
			response = map[string]any{"resources": []any{map[string]any{"name": "pods", "kind": "Pod", "namespaced": true, "verbs": []string{"get", "list"}}}}
		case "/apis/source.toolkit.fluxcd.io/v1":
			response = map[string]any{"resources": []any{map[string]any{"name": "gitrepositories", "kind": "GitRepository", "namespaced": true, "verbs": []string{"get", "list"}}}}
		case "/apis/source.toolkit.fluxcd.io/v1/namespaces/flux-system/gitrepositories/config":
			response = map[string]any{"apiVersion": "source.toolkit.fluxcd.io/v1", "kind": "GitRepository", "metadata": map[string]any{"name": "config", "namespace": "flux-system", "generation": 2}, "spec": map[string]any{"url": "https://repo.example/config", "secretRef": map[string]string{"name": "credentials"}}, "status": map[string]any{"artifact": map[string]string{"url": "http://forbidden-artifact.invalid/content", "revision": "main@sha1:old"}, "conditions": []any{map[string]any{"type": "Ready", "status": "False", "observedGeneration": 2, "message": "fetch failed"}}}}
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
			response = metav1.Status{Status: "Failure", Reason: metav1.StatusReasonNotFound, Code: 404}
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(server.Close)
	kube, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	item, err := fluxmonitor.New(kube).Get(context.Background(), Reference{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Kind: "GitRepository", Namespace: "flux-system", Name: "config"})
	if err != nil {
		t.Fatal(err)
	}
	if item.Ref.Version != "v1" || item.Status != "Not ready" || field(item, "Fetched revision") != "main@sha1:old" || item.ReadAt.IsZero() {
		t.Fatalf("bad source read: %#v", item)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 4 {
		t.Fatalf("unexpected reads: %#v", requests)
	}
	for _, request := range requests {
		if strings.Contains(request, "secret") || strings.Contains(request, "artifact") {
			t.Fatalf("read outside metadata scope: %s", request)
		}
	}
}

func TestProblemSortIsDeterministicAndTruncationIsKept(t *testing.T) {
	reader := &fixtureReader{objects: map[string]kubernetes.Collection{kustomizations.Key(): {
		Truncated: true,
		Objects: []unstructured.Unstructured{
			fixture(t, `{"metadata":{"name":"ready","namespace":"flux-system"},"status":{"conditions":[{"type":"Ready","status":"True"}]}}`),
			fixture(t, `{"metadata":{"name":"failed","namespace":"flux-system"},"status":{"conditions":[{"type":"Ready","status":"False"}]}}`),
		},
	}}}
	snapshot := fluxmonitor.New(reader).LoadWithCatalog(context.Background(), "", []kubecommon.Resource{kustomizations, helmreleases}, "")
	if !snapshot.Truncated || len(snapshot.Items) != 2 || snapshot.Items[0].Ref.Name != "failed" || snapshot.Items[0].Ref.Kind != "Kustomization" {
		t.Fatalf("sort/type/truncation lost: %#v", snapshot)
	}
	if !fluxmonitor.Supports([]kubecommon.Resource{kustomizations}) || argomonitor.Supports([]kubecommon.Resource{kustomizations}) {
		t.Fatal("tool availability does not match discovered kinds")
	}
}

func fixture(t *testing.T, data string) unstructured.Unstructured {
	t.Helper()
	var object unstructured.Unstructured
	if err := utiljson.Unmarshal([]byte(data), &object.Object); err != nil {
		t.Fatal(err)
	}
	return object
}
func field(item Item, label string) string {
	for _, f := range item.Fields {
		if f.Label == label {
			return f.Value
		}
	}
	return ""
}

func TestFluxCategoriesReadOnlyTheirOwnKinds(t *testing.T) {
	source := kubecommon.Resource{Group: "source.toolkit.fluxcd.io", Version: "v1", Kind: "GitRepository", Name: "gitrepositories", Namespaced: true, Verbs: []string{"get", "list"}}
	for _, test := range []struct {
		section  Section
		resource kubecommon.Resource
	}{{Kustomizations, kustomizations}, {HelmReleases, helmreleases}, {Sources, source}} {
		reader := &fixtureReader{catalog: []kubecommon.Resource{kustomizations, helmreleases, source}}
		snapshot := fluxmonitor.New(reader, test.section).Load(context.Background(), "controllers")
		if snapshot.State != Empty || len(reader.reads) != 1 || reader.reads[0] != "controllers/"+test.resource.Key()+"?" {
			t.Fatalf("%s crossed category or reported absent optional sources as failure: %#v %#v", test.section, snapshot, reader.reads)
		}
	}
	denied := apierrors.NewForbidden(schema.GroupResource{Resource: source.Name}, "", errors.New("denied"))
	reader := &fixtureReader{catalog: []kubecommon.Resource{kustomizations, helmreleases, source}, failures: map[string]error{helmreleases.Key(): denied}}
	if s := fluxmonitor.New(reader, Kustomizations).Load(context.Background(), ""); s.State != Empty {
		t.Fatal("unrelated HelmRelease permission affected Kustomizations")
	}
	if s := fluxmonitor.New(reader, HelmReleases).Load(context.Background(), ""); s.State != ReadDenied {
		t.Fatal("HelmRelease denial hidden")
	}
	reader.warning = "Source API discovery was incomplete"
	if s := fluxmonitor.New(reader, Sources).Load(context.Background(), ""); s.State != Partial || len(s.Issues) == 0 {
		t.Fatal("source discovery warning hidden")
	}
}

func TestArgoCategoryApplicationsAreScopedAndDeniedCountsStayUnknown(t *testing.T) {
	apps := kubecommon.Resource{Group: "argoproj.io", Version: "v1alpha1", Kind: "Application", Name: "applications", Namespaced: true, Verbs: []string{"get", "list"}}
	sets := kubecommon.Resource{Group: apps.Group, Version: apps.Version, Kind: "ApplicationSet", Name: "applicationsets", Namespaced: true, Verbs: apps.Verbs}
	projects := kubecommon.Resource{Group: apps.Group, Version: apps.Version, Kind: "AppProject", Name: "appprojects", Namespaced: true, Verbs: apps.Verbs}
	reader := &fixtureReader{catalog: []kubecommon.Resource{apps, sets, projects}, objects: map[string]kubernetes.Collection{
		apps.Key():     {Objects: []unstructured.Unstructured{fixture(t, `{"metadata":{"name":"payments","namespace":"argocd","ownerReferences":[{"apiVersion":"argoproj.io/v1alpha1","kind":"ApplicationSet","name":"fleet","uid":"set-id"}]},"spec":{"project":"shop"},"status":{"sync":{"status":"Synced"},"health":{"status":"Degraded"}}}`)}},
		sets.Key():     {Objects: []unstructured.Unstructured{fixture(t, `{"metadata":{"name":"fleet","namespace":"argocd","uid":"set-id"},"spec":{"generators":[{"list":{}}]}}`)}},
		projects.Key(): {Objects: []unstructured.Unstructured{fixture(t, `{"metadata":{"name":"shop","namespace":"argocd"},"spec":{"destinations":[{"namespace":"shop","server":"https://kubernetes.default.svc"}],"sourceRepos":["https://user:secret@git.example/repo?token=private"]}}`)}},
	}}
	for _, section := range []Section{ApplicationSets, Projects} {
		reader.reads = nil
		snapshot := argomonitor.New(reader, section).Load(context.Background(), "argocd")
		if snapshot.State != Available || len(snapshot.Items) != 1 {
			t.Fatalf("%s: %#v", section, snapshot)
		}
		item := snapshot.Items[0]
		if field(item, "Applications") != "1" || len(item.Related) != 1 || item.Related[0].Health != "Degraded" || !strings.Contains(item.RelatedMessage, "Other namespaces are not included") {
			t.Fatalf("related app semantics lost: %#v", item)
		}
		for _, read := range reader.reads {
			if !strings.HasPrefix(read, "argocd/") {
				t.Fatal("association escaped namespace", read)
			}
		}
		if section == Projects && (field(item, "Repositories") != "1" || strings.Contains(field(item, "Allowed repository"), "secret") || item.Status != "") {
			t.Fatal("project leaked credentials or invented health")
		}
	}
	reader.failures = map[string]error{apps.Key(): apierrors.NewForbidden(schema.GroupResource{Resource: "applications"}, "", errors.New("denied"))}
	snapshot := argomonitor.New(reader, Projects).Load(context.Background(), "argocd")
	if len(snapshot.Items) != 1 || field(snapshot.Items[0], "Applications") != "" || !strings.Contains(snapshot.Items[0].RelatedMessage, "Read denied") {
		t.Fatal("unreadable applications were counted as zero")
	}
}
