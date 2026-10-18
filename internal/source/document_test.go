package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceSavePreservesDraftAndRejectsExternalEdits(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "notes.txt")
	os.WriteFile(file, []byte("before\n"), 0640)
	s, err := Load(context.Background(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Edit("notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(file, []byte("external\n"), 0640)
	if err = d.Save([]byte("draft\n")); err == nil {
		t.Fatal("overwrote external edit")
	}
	if string(d.Baseline) != "before\n" {
		t.Fatal("baseline changed after failed write")
	}
	os.WriteFile(file, []byte("before\n"), 0640)
	if err = d.Save([]byte("saved\n")); err != nil {
		t.Fatal(err)
	}
	bytes, _ := os.ReadFile(file)
	info, _ := os.Stat(file)
	if string(bytes) != "saved\n" || info.Mode().Perm() != 0640 {
		t.Fatal("save lost bytes or permissions")
	}
	if sourceTextFixture(t, s, "notes.txt") != "saved\n" {
		t.Fatal("selected file read did not see saved bytes")
	}
	if err = d.Save([]byte("again\n")); err != nil {
		t.Fatal(err)
	}
}
func TestSourceSaveRejectsReplacementAndSymlink(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "file.txt")
	os.WriteFile(name, []byte("before"), 0600)
	s, _ := Load(context.Background(), Config{Path: root})
	d, _ := s.Edit("file.txt")
	os.Rename(name, filepath.Join(root, "old.txt"))
	os.WriteFile(name, []byte("before"), 0600)
	if d.Save([]byte("after")) == nil {
		t.Fatal("accepted replaced inode")
	}
	os.Remove(name)
	os.Symlink(filepath.Join(root, "old.txt"), name)
	if _, err := s.Edit("file.txt"); err == nil {
		t.Fatal("accepted symbolic link")
	}
	if _, err := s.Edit("../file.txt"); err == nil {
		t.Fatal("accepted traversal")
	}
}
func TestSourceCopyIsExclusiveAndComplete(t *testing.T) {
	s := &Snapshot{Config: Config{Repository: "https://example.invalid/repo"}, Files: map[string][]byte{"dir/file.txt": []byte("snapshot")}}
	if _, err := s.Edit("dir/file.txt"); err == nil {
		t.Fatal("remote snapshot writable")
	}
	dst := filepath.Join(t.TempDir(), "copy")
	if err := s.Copy(dst); err != nil {
		t.Fatal(err)
	}
	if err := s.Copy(dst); err == nil {
		t.Fatal("overwrote existing directory")
	}
	data, _ := os.ReadFile(filepath.Join(dst, "dir/file.txt"))
	if string(data) != "snapshot" {
		t.Fatal("copy differs")
	}
	s.Gaps = []string{"omitted"}
	if err := s.Copy(filepath.Join(t.TempDir(), "partial")); err == nil || !strings.Contains(err.Error(), "omitted") {
		t.Fatal("copied partial snapshot")
	}
}

func TestSourceDocumentRejectsFileGrowth(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(context.Background(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Edit("notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Truncate(file, maxFile+1); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Edit("notes.txt"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("edit accepted enlarged file: %v", err)
	}
	if err = d.Save([]byte("draft")); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("save accepted enlarged file: %v", err)
	}
	info, err := os.Stat(file)
	if err != nil || info.Size() != maxFile+1 || string(d.Baseline) != "before" {
		t.Fatal("failed save changed the file or baseline")
	}
}
