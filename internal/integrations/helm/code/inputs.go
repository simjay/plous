package code

import (
	"bytes"
	"io/fs"
	"time"

	"helm.sh/helm/v3/pkg/ignore"
)

// ChartIncludes uses the same root-relative rules and defaults as Helm's directory loader.
func ChartIncludes(data []byte) (func(string, bool) bool, error) {
	rules, err := ignore.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	rules.AddDefaults()
	return func(name string, directory bool) bool {
		return !rules.Ignore(name, chartPathInfo{directory: directory})
	}, nil
}

// Helm's matcher consults only IsDir; paths are supplied separately.
type chartPathInfo struct{ directory bool }

func (i chartPathInfo) Name() string { return "" }
func (i chartPathInfo) Size() int64  { return 0 }
func (i chartPathInfo) Mode() fs.FileMode {
	if i.directory {
		return fs.ModeDir
	}
	return 0
}
func (i chartPathInfo) ModTime() time.Time { return time.Time{} }
func (i chartPathInfo) IsDir() bool        { return i.directory }
func (i chartPathInfo) Sys() any           { return nil }
