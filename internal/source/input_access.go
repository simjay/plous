package source

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// inputKind inspects an exact dependency, independent of scanning exclusions.
func (s *Snapshot) inputKind(name string) (bool, error) {
	defer runtime.KeepAlive(s)
	if !inputPath(name) {
		return false, errors.New("input leaves the selected source")
	}
	if s.Files != nil {
		if _, ok := s.Files[name]; ok {
			return false, nil
		}
		for file := range s.Files {
			if name == "." || strings.HasPrefix(file, name+"/") {
				return true, nil
			}
		}
		return false, fmt.Errorf("%s: %w", name, os.ErrNotExist)
	}
	if s.git != nil {
		g := s.git
		g.store.mu.RLock()
		defer g.store.mu.RUnlock()
		if g.store.closed {
			return false, errors.New("acquired Git source has closed")
		}
		full := path.Join(g.prefix, name)
		if full == "." {
			return true, nil
		}
		entry, err := g.tree.FindEntry(full)
		if errors.Is(err, object.ErrEntryNotFound) {
			return false, fmt.Errorf("%s: %w", name, os.ErrNotExist)
		}
		if err != nil {
			return false, err
		}
		if entry.Mode == filemode.Dir {
			return true, nil
		}
		if entry.Mode != filemode.Regular && entry.Mode != filemode.Executable {
			return false, fmt.Errorf("%s: symlinks and submodules are unavailable", name)
		}
		return false, nil
	}
	root, err := openSourceRoot(s.Config.Path)
	if err != nil {
		return false, err
	}
	defer root.Close()
	f, err := openTreePath(root, name, false)
	if err != nil {
		return false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

// inputDirectory enumerates one selected directory, not a discovery inventory.
func (s *Snapshot) inputDirectory(name string, include func(string, bool) (bool, error)) ([]sourceEntry, error) {
	defer runtime.KeepAlive(s)
	if name == "" {
		name = "."
	}
	if !inputPath(name) {
		return nil, errors.New("invalid input directory")
	}
	var entries []sourceEntry
	if s.Files != nil {
		children := map[string]sourceEntry{}
		prefix := ""
		if name != "." {
			prefix = name + "/"
		}
		for file, data := range s.Files {
			if !strings.HasPrefix(file, prefix) {
				continue
			}
			part, _, dir := strings.Cut(strings.TrimPrefix(file, prefix), "/")
			key := path.Join(name, part)
			children[key] = sourceEntry{Path: key, Directory: dir, Size: int64(len(data)), Modified: s.Observed}
		}
		if len(children) == 0 && name != "." {
			return nil, os.ErrNotExist
		}
		for _, e := range children {
			if include != nil {
				ok, err := include(e.Path, e.Directory)
				if err != nil {
					return nil, err
				}
				if !ok {
					continue
				}
			}
			entries = append(entries, e)
		}
	} else if s.git != nil {
		g := s.git
		g.store.mu.RLock()
		if g.store.closed {
			g.store.mu.RUnlock()
			return nil, errors.New("acquired Git source has closed")
		}
		tree := g.tree
		full := path.Join(g.prefix, name)
		if full != "." {
			var err error
			tree, err = tree.Tree(full)
			if err != nil {
				g.store.mu.RUnlock()
				return nil, err
			}
		}
		g.store.mu.RUnlock()
		if len(tree.Entries) > sourcePageEntries {
			return nil, errors.New("source directory exceeds entry limit")
		}
		for _, e := range tree.Entries {
			item := sourceEntry{Path: path.Join(name, e.Name), Directory: e.Mode == filemode.Dir, Modified: s.Observed}
			if e.Mode != filemode.Dir && e.Mode != filemode.Regular && e.Mode != filemode.Executable {
				item.Mode = fs.ModeSymlink
			}
			if include != nil {
				ok, err := include(item.Path, item.Directory)
				if err != nil {
					return nil, err
				}
				if !ok {
					continue
				}
			}
			if e.Mode == filemode.Regular || e.Mode == filemode.Executable {
				g.store.mu.RLock()
				if g.store.closed {
					g.store.mu.RUnlock()
					return nil, errors.New("acquired Git source has closed")
				}
				file, err := tree.File(e.Name)
				g.store.mu.RUnlock()
				if err != nil {
					return nil, err
				}
				item.Size = file.Size
			}
			entries = append(entries, item)
		}
	} else {
		root, err := openSourceRoot(s.Config.Path)
		if err != nil {
			return nil, err
		}
		defer root.Close()
		return treeDirectory(root, name, include)
	}

	if len(entries) > sourcePageEntries {
		return nil, errors.New("source directory exceeds 20,000 entries; select a smaller directory")
	}
	slices.SortFunc(entries, func(a, b sourceEntry) int { return strings.Compare(a.Path, b.Path) })
	return entries, nil
}

func inputPath(name string) bool {
	if !fs.ValidPath(name) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".git" {
			return false
		}
	}
	return true
}

// treeDirectory applies name/type exclusions before opening any child path.
func treeDirectory(root *os.File, name string, include func(string, bool) (bool, error)) ([]sourceEntry, error) {
	f, err := openTreePath(root, name, true)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	children, err := f.ReadDir(sourcePageEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(children) > sourcePageEntries {
		return nil, errors.New("source directory exceeds entry limit")
	}
	entries := []sourceEntry{}
	for _, e := range children {
		item := sourceEntry{Path: path.Join(name, e.Name()), Directory: e.IsDir(), Mode: e.Type()}
		if include != nil {
			ok, err := include(item.Path, item.Directory)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
		if e.Type()&fs.ModeSymlink == 0 && (e.IsDir() || e.Type().IsRegular()) {
			child, err := openTreePath(root, item.Path, e.IsDir())
			if err != nil {
				return nil, err
			}
			info, err := child.Stat()
			child.Close()
			if err != nil {
				return nil, err
			}
			item.Size, item.Modified, item.Mode = info.Size(), info.ModTime().UTC(), info.Mode()
		}
		entries = append(entries, item)
	}
	slices.SortFunc(entries, func(a, b sourceEntry) int { return strings.Compare(a.Path, b.Path) })
	return entries, nil
}
