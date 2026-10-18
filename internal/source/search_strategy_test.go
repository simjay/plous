package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLargeSourceDiscoveryKeepsMetadataAndSearchContinuesPastWorkLimit(t *testing.T) {
	scope, root := readScope(t)
	body := strings.Repeat("x", 8192) + "\n"
	for n := range 4200 {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("file-%04d.txt", n)), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "z-target.txt"), []byte("unique-target\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	snapshot, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	inventory := Discover(snapshot)
	if snapshot.Files != nil || snapshot.FileCount() != 4201 || len(inventory.Entries) != 0 {
		t.Fatal("discovery retained source bodies or lost metadata")
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	discoveryHeap := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	peak := after.HeapAlloc
	scope.View = CaptureReadView(inventory, nil, nil, nil)
	request := ReadRequest{Operation: "search", Query: "unique-target"}
	pages, matches := 0, 0
	for {
		page, err := readSourcePage(t.Context(), scope, request)
		if err != nil {
			t.Fatal(err)
		}
		runtime.ReadMemStats(&after)
		peak = max(peak, after.HeapAlloc)
		pages++
		matches += len(page.Data.Matches)
		if pages > 50 {
			t.Fatal("search did not advance")
		}
		token, _ := page.Continuation["cursor"].(string)
		if token == "" {
			if !page.Complete {
				t.Fatal("incomplete source without a continuation", page)
			}
			break
		}
		request.Cursor = token
	}
	if pages < 2 || matches != 1 || len(scope.Session.searches) != 0 {
		t.Fatal("search failed to cross the former whole-source limit", pages, matches)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(inventory)
	t.Logf("retained heap after discovery: %d bytes; after search: %d bytes; maximum heap sampled at page boundaries: %d bytes above baseline", discoveryHeap, int64(after.HeapAlloc)-int64(before.HeapAlloc), int64(peak)-int64(before.HeapAlloc))
	t.Logf("4201 files, %.1f MiB source text; %d pages; elapsed %s; no source bodies retained after discovery", float64(len(body)*4200)/(1<<20), pages, time.Since(started))
}

func TestSearchSelectionAndContinuationAreBoundToQueryAndContents(t *testing.T) {
	scope, root := readScope(t)
	if err := os.Mkdir(filepath.Join(root, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	for file, text := range map[string]string{"config/app.yaml": "TIMEOUT: 10\nTIMEOUT: 20\n", "notes.txt": "TIMEOUT: elsewhere", ".gitignore": "notes.txt\n"} {
		if err := os.WriteFile(filepath.Join(root, file), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	insensitive := false
	request := ReadRequest{Operation: "search", Path: "config", Glob: "*.yaml", Regex: true, CaseSensitive: &insensitive, Query: "timeout: [0-9]+", Limit: 1}
	first, err := readSourcePage(t.Context(), scope, request)
	if err != nil || len(first.Data.Matches) != 1 {
		t.Fatal(first, err)
	}
	request.Cursor = first.Continuation["cursor"].(string)
	wrong := request
	wrong.Query = "elsewhere"
	if _, err := readSourcePage(t.Context(), scope, wrong); err == nil {
		t.Fatal("cursor accepted another query")
	}
	if err := os.WriteFile(filepath.Join(root, "config/app.yaml"), []byte("TIMEOUT: 30\nTIMEOUT: 40\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSourcePage(t.Context(), scope, request); !errors.Is(err, ErrReadDenied) {
		t.Fatal("changed-file search did not give corrective feedback", err)
	}
	if len(scope.Session.searches) != 0 {
		t.Fatal("failed cursor retained traversal state")
	}
	if _, err := readSourcePage(t.Context(), scope, ReadRequest{Operation: "read", Path: first.Data.Matches[0].Path, Identity: first.Data.Matches[0].Identity}); err == nil {
		t.Fatal("read accepted stale content identity")
	}
	exact := readObservation(t, scope, ReadRequest{Operation: "search", Path: "config/app.yaml", Query: "TIMEOUT: 30"})
	if len(exact.Rows) != 1 || exact.Facts["traversal_finished"].String() != "true" {
		t.Fatal("exact-file search failed", exact)
	}
	all := readObservation(t, scope, ReadRequest{Operation: "search", Query: "elsewhere"})
	if len(all.Rows) != 0 {
		t.Fatal("ignored file was searched")
	}
	for _, bad := range []ReadRequest{{Operation: "search", Query: "[", Regex: true}, {Operation: "search", Query: "a", Glob: "["}, {Operation: "catalog", Regex: true}} {
		if _, err := GuardedRead(t.Context(), scope, bad); err == nil {
			t.Fatal("invalid options accepted", bad)
		}
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := readSourcePage(cancelled, scope, ReadRequest{Operation: "search", Query: "TIMEOUT"}); err == nil {
		t.Fatal("cancelled search succeeded")
	}
	scope.Session.Close()
	if _, err := readSourcePage(t.Context(), scope, ReadRequest{Operation: "read", Path: "config/app.yaml"}); err == nil {
		t.Fatal("closed session accepted work")
	}
}

func TestRenderCapturesCurrentTransitiveInputsWithoutUnrelatedSourceBodies(t *testing.T) {
	root := sourceFiles(t, map[string]string{
		"base/kustomization.yaml": "resources: [app.yaml]\n",
		"base/app.yaml":           "apiVersion: v1\nkind: Service\nmetadata: {name: original}\nspec: {ports: [{port: 80}]}\n",
		"prod/kustomization.yaml": "resources: [../base]\nconfigMapGenerator:\n- name: settings\n  files: [config.txt]\n",
		"prod/config.txt":         "setting=old\n", "unrelated.bin": strings.Repeat("x", 3<<20),
	})
	snapshot, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	inventory := Discover(snapshot)
	first, err := (Renderer{}).Render(t.Context(), inventory, "kustomize:prod", Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.CapturedSource().Source.Files) != 4 || first.CapturedSource().Source.Has("unrelated.bin") {
		t.Fatal("captured unrelated files")
	}
	if err := os.WriteFile(filepath.Join(root, "prod/config.txt"), []byte("setting=new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := (Renderer{}).Render(t.Context(), inventory, "kustomize:prod", Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	if first.OperationID == second.OperationID || first.ContentIdentity == second.ContentIdentity {
		t.Fatal("changed dependency did not change render identity")
	}
	if string(first.CapturedSource().Source.Files["prod/config.txt"]) != "setting=old\n" {
		t.Fatal("previous inputs changed")
	}
	if err := os.WriteFile(filepath.Join(root, "prod/kustomization.yaml"), []byte("resources: [missing.yaml]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Renderer{}).Render(t.Context(), inventory, "kustomize:prod", Inputs{}); err == nil {
		t.Fatal("render reused a stale configuration graph")
	}
	if snapshot.Files != nil {
		t.Fatal("render polluted discovery with captured bytes")
	}
}

func TestHelmCapturesFilesAndValuesOutsideChart(t *testing.T) {
	root := sourceFiles(t, map[string]string{
		"chart/Chart.yaml":            "apiVersion: v2\nname: demo\nversion: 1.0.0\n",
		"chart/templates/config.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: example}\ndata:\n  text: {{ .Files.Get \"assets/text.txt\" | quote }}\n  mode: {{ .Values.mode | quote }}\n",
		"chart/assets/text.txt":       "captured asset", "values/prod.yaml": "mode: production\n", "unrelated.txt": "not an input",
	})
	snapshot, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Renderer{}).Render(t.Context(), Discover(snapshot), "helm:chart", Inputs{Values: []string{"values/prod.yaml"}})
	if err != nil {
		t.Fatal(err)
	}
	data := result.Resources[0].Object["data"].(map[string]any)
	if data["text"] != "captured asset" || data["mode"] != "production" || len(result.CapturedSource().Source.Files) != 4 {
		t.Fatal("selected Helm inputs incomplete", data)
	}
}
