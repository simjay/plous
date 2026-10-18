package code

import (
	"fmt"
	"path"
	"sort"
	"strings"

	kustomizecommon "github.com/simjay/plous/internal/integrations/kustomize/common"
	"github.com/simjay/plous/internal/integrations/manifest"
)

// Inventory retains parsed configurations. Build inventories also own captured inputs.
type Inventory struct {
	Files   map[string][]byte
	Entries []kustomizecommon.Entry
	paths   []string
	read    func(string) ([]byte, error)
	inspect func(string) (bool, error)
}

func Discover(files map[string][]byte) *Inventory {
	result := &Inventory{Files: files}
	result.discover(nil)
	result.read = nil
	return result
}

// Inspect reads only kustomization candidates and retains their parsed declarations.
func Inspect(paths []string, read func(string) ([]byte, error), retain func(int) error) (*Inventory, error) {
	result := &Inventory{paths: paths, read: read}
	err := result.discover(retain)
	result.read = nil
	return result, err
}
func (s *Inventory) readFile(name string) ([]byte, error) {
	if s.read != nil {
		if s.paths != nil && !s.has(name) {
			return nil, fmt.Errorf("file absent from scanned inventory: %s", name)
		}
		return s.read(name)
	}
	data, ok := s.Files[name]
	if !ok {
		return nil, fmt.Errorf("file unavailable: %s", name)
	}
	return data, nil
}
func (s *Inventory) has(name string) bool {
	if s.paths != nil {
		i := sort.SearchStrings(s.paths, name)
		return i < len(s.paths) && s.paths[i] == name
	}
	_, ok := s.Files[name]
	return ok
}
func (s *Inventory) Paths() []string {
	if s.paths != nil {
		return s.paths
	}
	names := make([]string, 0, len(s.Files))
	for name := range s.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func (s *Inventory) Entry(name string) *kustomizecommon.Entry {
	for i := range s.Entries {
		if s.Entries[i].Path == name {
			return &s.Entries[i]
		}
	}
	return nil
}
func Document(name string, data []byte) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".yaml", ".yml", ".json":
		return kustomizecommon.MaskConfiguration(data)
	case ".md", ".txt":
		return manifest.SafeText(string(data))
	default:
		if isKustomization(path.Base(name)) {
			return kustomizecommon.MaskConfiguration(data)
		}
		return "File captured as build input. Preview is unavailable for this file type."
	}
}
