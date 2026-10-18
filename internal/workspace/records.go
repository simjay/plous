package workspace

import (
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxActivityBytes = 8 << 20

func openActivityFile(path string, writable bool) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if writable {
		flags = unix.O_RDWR | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	}
	fd, err := unix.Open(path, flags, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("activity records must be regular files")
	}
	if writable && info.Mode().Perm() != 0600 {
		if err := f.Chmod(0600); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

// ActivityReferences captures revision identities without reading every body.
func (s *Store) ActivityReferences(w string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, ErrClosed
	}
	if !idPattern.MatchString(w) {
		return nil, errors.New("invalid workspace ID")
	}
	dir := filepath.Join(s.root, "workspaces", w, "revisions")
	for _, p := range []string{filepath.Dir(dir), dir} {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() {
			return nil, errors.New("activity storage must be a directory")
		}
	}
	entries, err := readDirBounded(dir, 100000)
	if err != nil {
		return nil, err
	}
	if len(entries) > 100000 {
		return nil, errors.New("activity exceeds the current search index limit")
	}
	var refs []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".pending-") {
			continue
		}
		if !e.IsDir() || strings.ContainsAny(e.Name(), `/\`) {
			return nil, errors.New("invalid activity entry")
		}
		refs = append(refs, e.Name())
	}
	return refs, nil
}

func (s *Store) ReadActivity(w, ref string) (Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return Change{}, ErrClosed
	}
	if !idPattern.MatchString(w) || strings.HasPrefix(ref, ".") || strings.ContainsAny(ref, `/\`) || len(ref) > 128 || ref == "" {
		return Change{}, errors.New("invalid activity reference")
	}
	dir := filepath.Join(s.root, "workspaces", w)
	for _, path := range []string{dir, filepath.Join(dir, "revisions"), filepath.Join(dir, "revisions", ref)} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() {
			return Change{}, errors.New("activity storage must be a directory")
		}
	}
	f, err := openActivityFile(filepath.Join(dir, "revisions", ref, "change.json"), false)
	if err != nil {
		return Change{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxActivityBytes+1))
	if err != nil || len(data) > maxActivityBytes {
		return Change{}, errors.New("activity record exceeds size limit")
	}
	var change Change
	if err := json.Unmarshal(data, &change); err != nil {
		return Change{}, err
	}
	if change.WorkspaceID != w || change.After.ID != w || revisionName(change) != ref {
		return Change{}, errors.New("activity identity does not match reference")
	}
	return change, nil
}

func readDirBounded(path string, limit int) ([]os.DirEntry, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	entries, err := f.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > limit {
		return nil, errors.New("stored history exceeds the current search index limit")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}
