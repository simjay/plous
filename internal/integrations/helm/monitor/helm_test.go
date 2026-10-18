package monitor

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
)

type runnerFunc func(context.Context, Config, []string, string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, c Config, args []string, input string) ([]byte, error) {
	return f(ctx, c, args, input)
}
func chartArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(data))
	}
	_ = tw.Close()
	_ = gz.Close()
	return b.Bytes()
}
func chartFixture(t *testing.T) *helmcommon.Artifact {
	t.Helper()
	a, err := helmcommon.ReadArtifact(chartArchive(t, map[string]string{"demo/Chart.yaml": "apiVersion: v2\nname: demo\nversion: 1.0.0\n", "demo/values.yaml": "replicas: 1\n", "demo/templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: {{ .Release.Name }}\nspec:\n  replicas: {{ .Values.replicas }}\n  selector:\n    matchLabels:\n      app: demo\n  template:\n    metadata:\n      labels:\n        app: demo\n    spec:\n      containers:\n      - name: demo\n        image: example.invalid/demo:1\n"}))
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func fixtureConfig() Config {
	return Config{ConnectionID: "connection", Kubeconfig: "/explicit/kubeconfig", Context: "staging", Namespace: "apps", Driver: "secret"}
}

func TestHelmPreviewIdentifiesScopeHooksAndChangedWorkloadFields(t *testing.T) {
	before := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: demo\nspec:\n  replicas: 1\n"
	p := &Plan{Operation: "preview", Before: before, Manifest: strings.ReplaceAll(before, "replicas: 1", "replicas: 2") + "---\napiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRole\nmetadata:\n  name: demo\n  annotations:\n    helm.sh/hook: pre-install\n"}
	summary := p.Summary()
	for _, want := range []string{"Changed spec.replicas", "Cluster-scoped resource", "Helm hook: pre-install"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("missing %s in %s", want, summary)
		}
	}
	if outcome, _ := outcome(context.Background(), helmcommon.ErrOutputLimit); outcome != "unknown" {
		t.Fatalf("bounded output claimed %s", outcome)
	}
}
func TestHelmPolicyRejectsWritesAndEscapesBeforeDispatch(t *testing.T) {
	calls := 0
	c := &Client{Config: fixtureConfig(), Runner: runnerFunc(func(context.Context, Config, []string, string) ([]byte, error) { calls++; return nil, nil })}
	e, err := NewProjector(c, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`{"operation":"rollback","name":"shop"}`, `{"operation":"install","name":"shop"}`, `{"operation":"test","name":"shop"}`, `{"operation":"list","namespace":"other"}`, `{"operation":"values","name":"--post-renderer=evil"}`, `{"operation":"list","flags":["--all-namespaces"]}`, `{"operation":"list","operation":"install"}`, `{"Operation":"list"}`, `{"operation":"list","name":"shop"}`, `{"operation":"history","name":"shop","revision":1}`} {
		cmd, err := DecodeCommand(input)
		if err == nil {
			_, err = e.Execute(context.Background(), cmd)
		}
		if err == nil {
			t.Errorf("accepted %s", input)
		}
	}
	if calls != 0 {
		t.Fatal("denied command reached runner")
	}
	c.Config.Namespace = "other"
	if e.config.Namespace != "apps" || e.client.Config.Namespace != "apps" {
		t.Fatal("captured scope was mutable")
	}
}
func TestHelmListUsesMetadataNotHyphenSplitting(t *testing.T) {
	c := &Client{Config: fixtureConfig(), Runner: runnerFunc(func(_ context.Context, _ Config, args []string, _ string) ([]byte, error) {
		switch args[0] {
		case "list":
			return []byte(`[{"name":"shop","namespace":"apps","revision":"7","chart":"ambiguous-name-1.0.0","status":"failed"}]`), nil
		case "get":
			return []byte(`{"name":"shop","namespace":"apps","revision":7,"chart":"different-name","version":"1.0.0-beta.2","status":"failed"}`), nil
		}
		return nil, errors.New("unexpected")
	})}
	rows, err := c.List(context.Background(), "apps")
	if err != nil || len(rows) != 1 || rows[0].Chart != "different-name" || rows[0].Status != "failed" {
		t.Fatalf("%+v %v", rows, err)
	}
}

func TestHelmListKeepsEarlierPagesWhenLaterReadFails(t *testing.T) {
	var firstPage []map[string]string
	for i := 0; i < 100; i++ {
		firstPage = append(firstPage, map[string]string{"name": fmt.Sprintf("shop-%d", i), "namespace": "apps", "revision": "1"})
	}
	c := &Client{Config: fixtureConfig(), Runner: runnerFunc(func(_ context.Context, _ Config, args []string, _ string) ([]byte, error) {
		if args[0] == "list" {
			if args[slices.Index(args, "--offset")+1] != "0" {
				return nil, errors.New("later page unavailable")
			}
			return json.Marshal(firstPage)
		}
		return json.Marshal(Release{Name: args[2], Namespace: "apps", Revision: 1, Chart: "demo", Version: "1.0.0"})
	})}
	rows, err := c.List(context.Background(), "apps")
	if err == nil || len(rows) != 100 || rows[99].Name != "shop-99" || rows[99].Chart != "demo" {
		t.Fatalf("lost partial inventory: %d rows, %v", len(rows), err)
	}
}
func TestHelmChartPreviewCannotExecute(t *testing.T) {
	calls := []string{}
	c := &Client{Config: fixtureConfig(), Runner: runnerFunc(func(_ context.Context, _ Config, args []string, _ string) ([]byte, error) {
		calls = append(calls, args[0])
		switch args[0] {
		case "lint":
			return []byte("lint passed"), nil
		case "template":
			return []byte("apiVersion: v1\nkind: ConfigMap\nmetadata: {name: demo}\n"), nil
		}
		t.Fatalf("chart preview invoked %s", args[0])
		return nil, nil
	})}
	for _, baseline := range []*Snapshot{nil, {Name: "demo", Namespace: "apps", Version: 2}} {
		p, err := c.PrepareChart(t.Context(), chartFixture(t), "demo", "apps", "{}", baseline)
		if err != nil || p.Operation != "preview" {
			t.Fatalf("preview: %v %v", p, err)
		}
		before := len(calls)
		if status, err := c.Execute(t.Context(), p); status != "not submitted" || err == nil || len(calls) != before {
			t.Fatal("preview reached execution", status, err, calls)
		}
	}
	for _, operation := range []string{"install", "search"} {
		if _, err := (SDKRunner{}).Run(t.Context(), Config{}, []string{operation, "demo"}, ""); err == nil || !strings.Contains(err.Error(), "unsupported Helm operation") {
			t.Fatal("retired operation reached the SDK", operation, err)
		}
	}
}

func TestHelmCapturedTargetDetectsKubeconfigEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	config := "apiVersion: v1\nkind: Config\ncontexts:\n- name: staging\n  context: {cluster: fixture}\nclusters:\n- name: fixture\n  cluster: {server: 'https://first.invalid'}\n"
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{ConnectionID: "captured", Kubeconfig: path, Context: "staging"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.captureTarget()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte(strings.ReplaceAll(config, "first.invalid", "second.invalid")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.captureTarget(); err == nil {
		t.Fatal("target check accepted a replaced kubeconfig")
	}
}
func TestHelmRollbackRechecksRetainedAndCurrentRevisions(t *testing.T) {
	latest, writes := 3, 0
	corrupt := false
	runner := runnerFunc(func(_ context.Context, _ Config, args []string, _ string) ([]byte, error) {
		rev := latest
		if i := slices.Index(args, "--revision"); i >= 0 {
			fmt.Sscan(args[i+1], &rev)
		}
		switch args[0] {
		case "get":
			if args[1] == "metadata" {
				return []byte(fmt.Sprintf(`{"name":"shop","namespace":"apps","revision":%d,"chart":"demo","version":"1.0.0","status":"deployed"}`, rev)), nil
			}
			return []byte(`{"replicas":1}`), nil
		case "status":
			s := Snapshot{Name: "shop", Namespace: "apps", Version: rev, Chart: helmcommon.Chart{Metadata: map[string]any{"name": "demo", "version": "1.0.0"}}, Manifest: fmt.Sprintf("apiVersion: v1\nkind: ConfigMap\nmetadata: {name: revision-%d}\n", rev)}
			s.Info.Status = "deployed"
			if corrupt && rev == 1 {
				s.Manifest += "# replaced\n"
			}
			b, _ := json.Marshal(s)
			return b, nil
		case "history":
			return []byte(`[{"revision":1,"status":"superseded"},{"revision":3,"status":"deployed"}]`), nil
		case "rollback":
			writes++
			return []byte("ok"), nil
		}
		return nil, errors.New("unexpected")
	})
	c := &Client{Config: fixtureConfig(), Runner: runner}
	p, err := c.PrepareRollback(context.Background(), "shop", "apps", 1)
	if err != nil {
		t.Fatal(err)
	}
	p.Namespace = "other"
	if status, err := c.Execute(t.Context(), p); status != "not submitted" || err == nil || writes != 0 {
		t.Fatal("modified rollback target reached execution")
	}
	p.Namespace = "apps"
	targetIdentity := "captured"
	c.targetIdentity = func() (string, error) { return targetIdentity, nil }
	p.targetConfigIdentity = targetIdentity
	targetIdentity = "changed"
	if status, err := c.Execute(t.Context(), p); status != "not submitted" || err == nil || writes != 0 {
		t.Fatal("changed kubeconfig reached rollback")
	}
	targetIdentity = "captured"
	latest = 4
	if status, e := c.Execute(context.Background(), p); status != "not submitted" || e == nil {
		t.Fatal("new current revision accepted")
	}
	latest = 3
	corrupt = true
	if _, e := c.Execute(context.Background(), p); e == nil {
		t.Fatal("replaced target accepted")
	}
	corrupt = false
	if status, e := c.Execute(context.Background(), p); e != nil || status != "completed" || writes != 1 {
		t.Fatalf("%s %v %d", status, e, writes)
	}
}
func TestHelmValuesRejectInvalidYAML(t *testing.T) {
	if err := ValidateValues("items: ["); err == nil {
		t.Fatal("invalid values accepted")
	}
	if err := ValidateValues("x: 1\nx: 2\n"); err == nil {
		t.Fatal("duplicate key accepted")
	}
}

func TestHelmRealLocalChartPreview(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	a := chartFixture(t)
	file := filepath.Join(dir, "chart.tgz")
	if err := os.WriteFile(file, a.Archive, 0600); err != nil {
		t.Fatal(err)
	}
	c := &Client{Config: Config{ConnectionID: "synthetic", Context: "synthetic", Namespace: "apps", Kubeconfig: filepath.Join(dir, "nonexistent"), Driver: "secret"}, Runner: SDKRunner{}}
	acquired, err := c.Acquire(context.Background(), file, "", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.PrepareChart(context.Background(), acquired, "fixture", "apps", "replicas: 2\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Manifest, "replicas: 2") || !strings.Contains(p.Manifest, "name: fixture") {
		t.Fatal(p.Manifest)
	}
}
