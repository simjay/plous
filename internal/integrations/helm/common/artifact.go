package common

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

type Artifact struct {
	Reference, Repository, Version, Digest string
	Archive                                []byte
	Files                                  map[string][]byte
	Metadata                               map[string]any
}

func ReadArtifact(data []byte) (*Artifact, error) {
	if len(data) > MaxOutput {
		return nil, errors.New("chart archive exceeds limit")
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("chart must be a gzip archive")
	}
	defer gz.Close()
	expanded, err := io.ReadAll(io.LimitReader(gz, (32<<20)+1))
	if err != nil {
		return nil, errors.New("invalid chart compression")
	}
	if len(expanded) > 32<<20 {
		return nil, errors.New("expanded chart exceeds 32 MiB")
	}
	reader := tar.NewReader(bytes.NewReader(expanded))
	a := &Artifact{Archive: append([]byte(nil), data...), Digest: fmt.Sprintf("%x", sha256.Sum256(data)), Files: map[string][]byte{}}
	root := ""
	total := int64(0)
	for count := 0; ; count++ {
		h, e := reader.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, errors.New("invalid chart archive")
		}
		if count >= 10000 {
			return nil, errors.New("chart archive has too many entries")
		}
		name := path.Clean(h.Name)
		if strings.Contains("/"+h.Name+"/", "/../") || path.IsAbs(name) || strings.Contains(name, "\\") {
			return nil, errors.New("invalid chart file path")
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg {
			return nil, errors.New("chart archive contains a link or special file")
		}
		top, rest, ok := strings.Cut(name, "/")
		if !ok || top == "." || top == ".." || path.IsAbs(name) || strings.Contains(name, "\\") || rest == "" {
			return nil, errors.New("invalid chart file path")
		}
		if root == "" {
			root = top
		}
		if root != top {
			return nil, errors.New("chart archive has multiple roots")
		}
		if _, exists := a.Files[rest]; exists {
			return nil, errors.New("duplicate chart file")
		}
		total += h.Size
		if h.Size < 0 || total > 32<<20 || len(a.Files) >= 10000 {
			return nil, errors.New("expanded chart exceeds limit")
		}
		contents, e := io.ReadAll(reader)
		if e != nil {
			return nil, e
		}
		a.Files[rest] = contents
	}
	if err = yaml.Unmarshal(a.Files["Chart.yaml"], &a.Metadata); err != nil {
		return nil, errors.New("invalid chart metadata")
	}
	name, _ := a.Metadata["name"].(string)
	version, _ := a.Metadata["version"].(string)
	if name == "" || version == "" {
		return nil, errors.New("chart metadata is missing or invalid")
	}
	a.Version = version
	return a, nil
}

func (a *Artifact) File(name string) string { return string(a.Files[name]) }

func (a *Artifact) Intact() bool {
	return a != nil && a.Digest == fmt.Sprintf("%x", sha256.Sum256(a.Archive))
}

func (a *Artifact) Materialize() (string, func(), error) {
	if !a.Intact() {
		return "", func() {}, errors.New("chart content changed; prepare a new preview")
	}
	dir, err := os.MkdirTemp("", "plous-preview-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	filename := filepath.Join(dir, "chart.tgz")
	if err = os.WriteFile(filename, a.Archive, 0600); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return filename, cleanup, nil
}

func CheckChartDirectory(root string) error {
	var total int64
	count := 0
	return filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("chart directories must not contain symbolic links")
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("chart directory contains a special file")
		}
		total += info.Size()
		count++
		if total > 32<<20 || count > 10000 {
			return errors.New("chart directory exceeds preview limits")
		}
		return nil
	})
}
