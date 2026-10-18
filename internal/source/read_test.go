package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisteredSourceCannotBypassRecallThroughProfileRoot(t *testing.T) {
	scope, _ := readScope(t)
	profile, err := filepath.EvalSymlinks(scope.ProfileRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{profile, filepath.Dir(profile), filepath.Join(profile, "agent", "run")} {
		for _, operation := range []string{"read", "list", "search"} {
			scope.Root = root
			args := map[string]any{"operation": operation}
			if operation == "read" {
				args["path"] = "conversation.2026-10-03.jsonl"
			}
			if operation == "search" {
				args["query"] = "saved user message"
			}
			if _, err := readFixture(scope, t.Context(), string(sourceJSON(args))); err == nil || !strings.Contains(err.Error(), "only through recall") {
				t.Fatalf("source root %q operation %s bypassed history boundary: %v", root, operation, err)
			}
		}
	}
}

func TestSourceSearchReportsLateMatchRange(t *testing.T) {
	for _, query := range []string{"needle", strings.Repeat("n", 1024)} {
		t.Run(fmt.Sprint(len(query)), func(t *testing.T) {
			scope, root := readScope(t)
			text := "first line\n" + strings.Repeat("日", 1000) + query + strings.Repeat("本", 1000)
			if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			value, err := readFixture(scope, t.Context(), string(sourceJSON(map[string]string{"operation": "search", "query": query})))
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Data struct {
					Matches []sourceMatch `json:"matches"`
				} `json:"data"`
			}
			if err := json.Unmarshal(sourceJSON(value), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Data.Matches) != 1 {
				t.Fatal(value)
			}
			match := result.Data.Matches[0]
			if !strings.Contains(match.Text, query) || !match.Truncated || match.Offset <= len("first line\n") || text[match.Offset:match.Offset+len(match.Text)] != match.Text {
				t.Fatal("search snippet lost match or byte provenance", match)
			}
		})
	}
}

func TestSourceChunksRejectChangedIdentity(t *testing.T) {
	scope, root := readScope(t)
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("exact-original"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := readFixture(scope, context.Background(), `{"operation":"read","path":"main.go","max_bytes":5}`)
	if err != nil {
		t.Fatal(err)
	}
	body := result.(map[string]any)
	data := body["data"].(map[string]any)
	identity := data["identity"].(string)
	if data["text"] != "exact" || body["complete"] != false {
		t.Fatal(body)
	}
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	input := string(sourceJSON(map[string]any{"operation": "read", "path": "main.go", "identity": identity, "offset": 5}))
	if _, err = readFixture(scope, context.Background(), input); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatal(err)
	}
}

func TestSourceSearchContinuation(t *testing.T) {
	scope, root := readScope(t)
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("target one\ntarget two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := readFixture(scope, context.Background(), `{"operation":"search","query":"target","limit":1}`)
	if err != nil {
		t.Fatal(err)
	}
	result := r.(map[string]any)
	cursor := result["continuation"].(map[string]any)["cursor"]
	r, err = readFixture(scope, context.Background(), string(sourceJSON(map[string]any{"operation": "search", "query": "target", "limit": 1, "cursor": cursor})))
	if err != nil || !strings.Contains(string(sourceJSON(r)), "target two") {
		t.Fatal(r, err)
	}
	if _, err = readFixture(scope, context.Background(), string(sourceJSON(map[string]any{"operation": "search", "query": "other", "cursor": cursor}))); err == nil {
		t.Fatal("cursor crossed query")
	}
}

func TestSourceSearchRemainsIncompleteAfterSkippedFile(t *testing.T) {
	scope, root := readScope(t)
	large, err := os.Create(filepath.Join(root, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err = large.Truncate(sourceFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	large.Close()
	for _, name := range []string{"b.go", "c.go"} {
		if err = os.WriteFile(filepath.Join(root, name), []byte("target"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	first, err := readFixture(scope, t.Context(), `{"operation":"search","query":"target","limit":1}`)
	if err != nil {
		t.Fatal(err)
	}
	value := first.(map[string]any)
	if value["complete"] != false || value["notice"] == nil {
		t.Fatal(value)
	}
	cursor := value["continuation"].(map[string]any)["cursor"]
	second, err := readFixture(scope, t.Context(), string(sourceJSON(map[string]any{"operation": "search", "query": "target", "cursor": cursor, "limit": 1})))
	if err != nil {
		t.Fatal(err)
	}
	value = second.(map[string]any)
	if value["complete"] != false || value["notice"] == nil || value["continuation"] != nil {
		t.Fatal("forgot omitted coverage on final page", value)
	}
}

func TestSourceRejectsParsedCredentialsWithoutExposingContent(t *testing.T) {
	for name, body := range map[string]string{
		"json-spaced":   `{"kind": "Secret", "data":{"value":"private-731"}}`,
		"json-escaped":  `{"k\u0069nd": "Secret", "data":{"value":"private-731"}}`,
		"quoted-yaml":   "kind: \"Secret\"\r\ndata: {value: private-731}\r\n",
		"multidoc":      "kind: Service\n---\nkind: 'Secret'\ndata: {value: private-731}\n",
		"nested-json":   `{"items":[{"kind":"Secret","data":{"value":"private-731"}}]}`,
		"alias-yaml":    "label: &kind Secret\nkind: *kind\ndata: {value: private-731}\n",
		"key-alias":     "label: &key kind\n*key: Secret\ndata: {value: private-731}\n",
		"kubeconfig":    "kind: Config\nclusters: [{name: local}]\nusers: [{name: user}]\n",
		"escaped-token": `{"access\u005ftoken":"private-731"}`,
		"env":           "env: [{name: PASSWORD, value: private-731}]\n",
	} {
		t.Run(name, func(t *testing.T) {
			scope, root := readScope(t)
			if err := os.WriteFile(filepath.Join(root, "resource.yaml"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := readFixture(scope, t.Context(), `{"operation":"read","path":"resource.yaml"}`)
			if err == nil || strings.Contains(err.Error(), "private-731") || result != nil {
				t.Fatal("credential content reached result", result, err)
			}
		})
	}
	scope, root := readScope(t)
	body := "apiVersion: apps/v1\nkind: Deployment\nspec: {replicas: 2}\n"
	if err := os.WriteFile(filepath.Join(root, "resource.yaml"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := readFixture(scope, t.Context(), `{"operation":"read","path":"resource.yaml"}`)
	if err != nil || result.(map[string]any)["data"].(map[string]any)["text"] != body {
		t.Fatal("permitted bytes changed", result, err)
	}
}

func readScope(t *testing.T) (ReadScope, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	profile, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session := NewReadSession()
	t.Cleanup(session.Close)
	return ReadScope{Session: session, Root: root, ProfileRoot: profile, ConnectionID: "src", ConnectionName: "source"}, root
}

func readFixture(scope ReadScope, ctx context.Context, input string) (any, error) {
	var request ReadRequest
	if err := json.Unmarshal([]byte(input), &request); err != nil {
		return nil, err
	}
	page, err := readSourcePage(ctx, scope, request)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	err = json.Unmarshal(sourceJSON(page), &result)
	return result, err
}
