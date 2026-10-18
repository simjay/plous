package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestHelmProjectionExplainsRecordedFieldsAndChanges(t *testing.T) {
	client := &Client{Config: fixtureConfig(), Runner: runnerFunc(func(_ context.Context, _ Config, args []string, _ string) ([]byte, error) {
		revision := 2
		for n, arg := range args {
			if arg == "--revision" && n+1 < len(args) {
				revision, _ = strconv.Atoi(args[n+1])
			}
		}
		if args[0] == "get" && args[1] == "metadata" {
			return []byte(`{"name":"shop","namespace":"apps","revision":2,"status":"failed"}`), nil
		}
		if args[0] != "status" {
			return nil, fmt.Errorf("unnecessary acquisition: %v", args)
		}
		event := "pre-install"
		if revision == 2 {
			event = "pre-upgrade"
		}
		return json.Marshal(map[string]any{
			"name": "shop", "namespace": "apps", "version": revision,
			"chart":    map[string]any{"metadata": map[string]string{"name": "shop", "version": "1.0.0"}},
			"info":     map[string]string{"status": "failed", "description": "progress deadline exceeded", "notes": "do-not-retain-notes"},
			"manifest": fmt.Sprintf("apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: shop}\nspec:\n  replicas: %d\n  template:\n    spec:\n      containers: [{name: shop, image: shop:v%d, env: [{name: PASSWORD, value: do-not-retain-env}]}]\n", revision, revision),
			"hooks":    []any{map[string]any{"name": "migrate", "events": []string{event}, "manifest": "apiVersion: batch/v1\nkind: Job\nmetadata: {name: migrate}\nspec: {backoffLimit: 1}\n"}},
		})
	})}
	projector, err := NewProjector(client, "apps")
	if err != nil {
		t.Fatal(err)
	}
	for operation, want := range map[string]string{"status": "progress deadline exceeded", "manifest": "shop:v2", "hooks": "pre-upgrade", "compare": "/spec/replicas"} {
		r, err := projector.Execute(t.Context(), Command{Operation: operation, Name: "shop"})
		if err != nil {
			t.Fatal(operation, err)
		}
		body := r.SearchText()
		if !strings.Contains(body, want) || strings.Contains(body, "do-not-retain") {
			t.Fatal(operation, body)
		}
		if operation == "compare" {
			found, hook := false, false
			for _, row := range r.Rows {
				if strings.HasSuffix(row.Identity.Name, "/spec/replicas") {
					found = row.Facts["before"].String() == "1" && row.Facts["after"].String() == "2"
				}
				if strings.HasSuffix(row.Identity.Name, "/events/0") {
					hook = row.Facts["before"].String() == "pre-install" && row.Facts["after"].String() == "pre-upgrade"
				}
			}
			if !found || !hook {
				t.Fatal("revision values lost", body)
			}
		}
	}
}

func TestHelmValueAcquisitionPreservesIntegerPrecision(t *testing.T) {
	client := &Client{Config: fixtureConfig(), Runner: runnerFunc(func(_ context.Context, _ Config, args []string, _ string) ([]byte, error) {
		switch args[0] {
		case "status":
			return []byte(`{"name":"shop","namespace":"apps","version":1,"chart":{"metadata":{"name":"shop","version":"1.0.0"}},"info":{"status":"deployed"}}`), nil
		case "get":
			return []byte(`{"sequence":9007199254740993}`), nil
		case "history":
			return []byte(`[]`), nil
		}
		return nil, fmt.Errorf("unexpected acquisition: %v", args)
	})}
	projector, err := NewProjector(client, "apps")
	if err != nil {
		t.Fatal(err)
	}
	r, err := projector.Execute(t.Context(), Command{Operation: "values", Name: "shop", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 1 || r.Rows[0].Facts["value"].String() != "9007199254740993" {
		t.Fatal("lost integer precision", r)
	}
}
