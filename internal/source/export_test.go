package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
)

func TestExportMasksAllResourcesAndNeverOverwrites(t *testing.T) {
	resources, err := manifest.Resources("apiVersion: v1\nkind: Secret\nmetadata: {name: credentials}\nstringData: {password: private-export-value}\n---\napiVersion: v1\nkind: Service\nmetadata: {name: api}\nspec: {ports: [{port: 80}]}\n")
	if err != nil {
		t.Fatal(err)
	}
	r := &Result{BuildResult: &BuildResult{Resources: resources}}
	file := filepath.Join(t.TempDir(), "render.yaml")
	if got, err := Export(r, file); err != nil || got != file {
		t.Fatal(got, err)
	}
	data, err := os.ReadFile(file)
	if err != nil || strings.Contains(string(data), "private-export-value") {
		t.Fatal("unsafe or unreadable export", err)
	}
	docs, err := manifest.Resources(string(data))
	if err != nil || len(docs) != 2 {
		t.Fatal("export did not retain all documents", err)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("export must be private", err)
	}
	if _, err := Export(r, file); err == nil {
		t.Fatal("overwrote existing file")
	}
	after, _ := os.ReadFile(file)
	if string(after) != string(data) {
		t.Fatal("existing file changed")
	}
	link := filepath.Join(t.TempDir(), "link.yaml")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(r, link); err == nil {
		t.Fatal("followed existing symlink")
	}
	for _, invalid := range []*Result{nil, {}, {BuildResult: &BuildResult{}}} {
		if _, err := Export(invalid, file); err == nil {
			t.Fatal("exported empty result")
		}
	}
	if _, err := Export(r, ""); err == nil {
		t.Fatal("accepted empty destination")
	}
}
