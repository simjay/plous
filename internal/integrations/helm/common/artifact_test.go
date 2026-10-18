package common

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestHelmArchiveChecksAllExpandedBytesAndActualLinks(t *testing.T) {
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink} {
		var data bytes.Buffer
		gz := gzip.NewWriter(&data)
		tw := tar.NewWriter(gz)
		_ = tw.WriteHeader(&tar.Header{Name: "demo/link", Typeflag: kind, Linkname: "/outside"})
		_ = tw.Close()
		_ = gz.Close()
		if _, err := ReadArtifact(data.Bytes()); err == nil {
			t.Fatal("archive link accepted")
		}
	}
	a := chartFixture(t)
	var expanded bytes.Buffer
	gz := gzip.NewWriter(&expanded)
	_, _ = gz.Write(bytes.Repeat([]byte{0}, (32<<20)+1))
	_ = gz.Close()
	if _, err := ReadArtifact(append(a.Archive, expanded.Bytes()...)); err == nil {
		t.Fatal("oversized trailing gzip stream escaped the archive limit")
	}
	corrupt := append([]byte(nil), a.Archive...)
	corrupt[len(corrupt)-5] ^= 1
	if _, err := ReadArtifact(corrupt); err == nil {
		t.Fatal("corrupt gzip checksum accepted")
	}
}

func TestHelmArchiveRejectsTraversalAndLinks(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute/file", "one/../escape"} {
		if _, err := ReadArtifact(chartArchive(t, map[string]string{name: "x"})); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
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
func chartFixture(t *testing.T) *Artifact {
	t.Helper()
	a, err := ReadArtifact(chartArchive(t, map[string]string{"demo/Chart.yaml": "apiVersion: v2\nname: demo\nversion: 1.0.0\n", "demo/values.yaml": "replicas: 1\n", "demo/templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: {{ .Release.Name }}\nspec:\n  replicas: {{ .Values.replicas }}\n  selector:\n    matchLabels:\n      app: demo\n  template:\n    metadata:\n      labels:\n        app: demo\n    spec:\n      containers:\n      - name: demo\n        image: example.invalid/demo:1\n"}))
	if err != nil {
		t.Fatal(err)
	}
	return a
}
