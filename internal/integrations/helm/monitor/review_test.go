package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
)

func TestHelmListDoesNotFollowUnexpectedNamespace(t *testing.T) {
	calls := 0
	c := &Client{Config: fixtureConfig(), Runner: runnerFunc(func(_ context.Context, _ Config, _ []string, _ string) ([]byte, error) {
		calls++
		return []byte(`[{"name":"shop","namespace":"production","revision":"1"}]`), nil
	})}
	e, err := NewProjector(c, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Execute(context.Background(), Command{Operation: "list"}); err == nil || calls != 1 {
		t.Fatal("list followed an out-of-scope release", calls, err)
	}
	if _, err = e.Execute(context.Background(), Command{Operation: "notes", Name: "shop"}); err == nil || calls != 1 {
		t.Fatal("withheld notes fetched a release body")
	}
}

func TestHelmComparisonPreservesImplicitNamespace(t *testing.T) {
	before := "apiVersion: v1\nkind: Secret\nmetadata: {name: demo}\ndata: {password: before-credential}\n"
	after := strings.ReplaceAll(before, "before-credential", "after-credential")
	after = strings.ReplaceAll(after, "name: demo", "name: demo, namespace: apps")
	p := &Plan{Namespace: "apps", Before: before, Manifest: after}
	if summary := p.Summary(); strings.Contains(summary, "+ Add") || strings.Contains(summary, "- Remove") || !strings.Contains(summary, "~ Change") {
		t.Fatal("implicit namespace appeared to replace the object", summary)
	}
}

func TestHelmReadExecutorMasksValuesAndManifestAtCapturedRevision(t *testing.T) {
	snapshot := Snapshot{Name: "shop", Namespace: "apps", Version: 2, Chart: helmcommon.Chart{Metadata: map[string]any{"name": "demo", "version": "1.0.0"}}, Manifest: "apiVersion: v1\nkind: Secret\nmetadata: {name: demo}\ndata: {key: hidden-manifest}\n"}
	c := &Client{Config: fixtureConfig(), Runner: runnerFunc(func(_ context.Context, config Config, args []string, _ string) ([]byte, error) {
		if config.Context != "staging" || !slices.Contains(args, "apps") {
			t.Fatal("read escaped the captured context or namespace")
		}
		switch args[0] {
		case "status", "get":
			i := slices.Index(args, "--revision")
			if i < 0 || args[i+1] != "2" {
				t.Fatal("read escaped the captured revision", args)
			}
			if args[0] == "status" {
				return json.Marshal(snapshot)
			}
			return []byte(`{"password":"hidden-value","replicas":2}`), nil
		case "history":
			return []byte(`[]`), nil
		}
		return nil, errors.New("unexpected read")
	})}
	executor, err := NewProjector(c, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"values", "manifest"} {
		result, err := executor.Execute(context.Background(), Command{Operation: operation, Name: "shop", Revision: 2})
		if err != nil {
			t.Fatal(err)
		}
		text := manifestutil.JSON(result)
		if strings.Contains(text, "hidden-") || result.Connection != fixtureConfig().ConnectionID {
			t.Fatal("output lost scope or exposed sensitive fields", text)
		}
	}
}
