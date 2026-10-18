package source

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

// Document binds a draft to the captured file, directory, and original bytes.
// Buffers are held in memory only. They never enter application session storage.
type Document struct {
	Directory, Path    string
	Baseline           []byte
	rootInfo, fileInfo fs.FileInfo
}

func regularPath(root *os.Root, name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) || name == "." {
		return nil, errors.New("invalid source file path")
	}
	var info fs.FileInfo
	current := ""
	for _, part := range strings.Split(name, "/") {
		current = path.Join(current, part)
		var err error
		info, err = root.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, errors.New("symbolic links are not editable source files")
		}
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("source file is not a regular file")
	}
	return info, nil
}

func readDocument(root *os.Root, name string) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err == nil && len(data) > maxFile {
		err = errors.New("source file exceeds 2 MiB")
	}
	return data, err
}

func (s *Snapshot) Edit(name string) (*Document, error) {
	if s.Config.Repository != "" {
		return nil, errors.New("remote snapshots are read-only; create a local copy before editing")
	}
	if !s.Has(name) {
		return nil, errors.New("file is unavailable in the captured source")
	}
	root, err := os.OpenRoot(s.Config.Path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	ri, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	fi, err := regularPath(root, name)
	if err != nil {
		return nil, err
	}
	current, err := readDocument(root, name)
	if err != nil {
		return nil, err
	}

	return &Document{Directory: s.Config.Path, Path: name, Baseline: bytes.Clone(current), rootInfo: ri, fileInfo: fi}, nil
}

func (d *Document) Save(data []byte) error {
	if len(data) > maxFile {
		return errors.New("document exceeds 2 MiB")
	}
	root, err := os.OpenRoot(d.Directory)
	if err != nil {
		return err
	}
	defer root.Close()
	ri, err := root.Stat(".")
	if err != nil || !os.SameFile(ri, d.rootInfo) {
		return errors.New("source directory changed; save stopped")
	}
	check := func() error {
		fi, err := regularPath(root, d.Path)
		if err != nil {
			return err
		}
		current, err := readDocument(root, d.Path)
		if err != nil {
			return err
		}
		if !os.SameFile(fi, d.fileInfo) || !bytes.Equal(current, d.Baseline) {
			return errors.New("file changed outside the editor; save stopped and draft retained")
		}
		return nil
	}
	if err = check(); err != nil {
		return err
	}
	temp := path.Join(path.Dir(d.Path), ".plous-save-"+rand.Text())
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Chmod(d.fileInfo.Mode().Perm())
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = check(); err != nil {
		return err
	}
	if err = root.Rename(temp, d.Path); err != nil {
		return err
	}
	d.Baseline = bytes.Clone(data)
	d.fileInfo, err = root.Stat(d.Path)
	if err != nil {
		return fmt.Errorf("file saved but verification failed: %w", err)
	}
	return nil
}

// Copy creates a new directory exclusively. It never overwrites a working tree.
func (s *Snapshot) Copy(directory string) error {
	if len(s.Gaps) != 0 {
		return errors.New("snapshot has omitted files; choose a complete source before making an editable copy")
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			os.RemoveAll(directory)
		}
	}()
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, name := range s.Paths() {
		if !fs.ValidPath(name) {
			return errors.New("invalid source path")
		}
		if err = root.MkdirAll(path.Dir(name), 0700); err != nil {
			return err
		}
		data, readErr := s.readBytes(name)
		if readErr != nil {
			return readErr
		}
		if err = root.WriteFile(name, data, 0600); err != nil {
			return err
		}
	}
	complete = true
	return nil
}
