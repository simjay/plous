package common

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxOutput = 16 << 20

var ErrOutputLimit = errors.New("Helm output exceeded 16 MiB; narrow the scope")

func ExpandPaths(paths string) (string, error) {
	parts := filepath.SplitList(paths)
	for i, p := range parts {
		if strings.HasPrefix(p, "~/") {
			h, e := os.UserHomeDir()
			if e != nil {
				return "", e
			}
			parts[i] = filepath.Join(h, p[2:])
		}
	}
	return strings.Join(parts, string(os.PathListSeparator)), nil
}

func CopyRegular(src, dst string, limit int64) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return errors.New("Helm input must be a bounded regular file")
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return errors.New("Helm input exceeds limit")
	}
	return os.WriteFile(dst, data, 0600)
}

type ChartFile struct {
	Name string `json:"name"`
	Data []byte `json:"data"`
}

type Chart struct {
	Metadata  map[string]any `json:"metadata"`
	Values    map[string]any `json:"values"`
	Schema    []byte         `json:"schema"`
	Templates []ChartFile    `json:"templates"`
	Files     []ChartFile    `json:"files"`
}
