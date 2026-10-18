package code

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	common "github.com/simjay/plous/internal/integrations/kustomize/common"
	"sigs.k8s.io/yaml"
)

// InspectTarget follows declared inputs without requiring a repository listing.
// Source owns exact reads, scope restrictions and input limits.
func InspectTarget(target string, read func(string) ([]byte, error), inspect func(string) (bool, error)) (*Inventory, error) {
	inventory := &Inventory{read: read, inspect: inspect}
	seen, visiting := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if visiting[name] {
			return fmt.Errorf("dependency cycle at %s", name)
		}
		if seen[name] {
			return nil
		}
		entry := common.Entry{Path: name}
		for _, candidate := range []string{"kustomization.yaml", "kustomization.yml", "Kustomization"} {
			file := path.Join(name, candidate)
			dir, err := inspect(file)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			if dir {
				return fmt.Errorf("kustomization must be a file: %s", file)
			}
			if entry.File != "" {
				return fmt.Errorf("ambiguous kustomization: %s", name)
			}
			entry.File = file
		}
		if entry.File == "" {
			return fmt.Errorf("missing kustomization: %s", name)
		}
		data, err := read(entry.File)
		if err != nil {
			return err
		}
		if err := yaml.UnmarshalStrict(data, &entry.Configuration); err != nil {
			return fmt.Errorf("invalid kustomization: %s", entry.File)
		}
		if api := text(entry.Configuration["apiVersion"]); api != "" && !strings.HasPrefix(api, "kustomize.config.k8s.io/") {
			return fmt.Errorf("not a build recipe: %s", entry.File)
		}
		entry.References = inventory.references(entry)
		visiting[name] = true
		for _, ref := range entry.References {
			if ref.Error != "" {
				return fmt.Errorf("%s: %s: %s", entry.File, ref.Path, ref.Error)
			}
			if ref.Entry {
				err = visit(ref.Path)
			} else {
				_, err = read(ref.Path)
			}
			if err != nil {
				return fmt.Errorf("%s: dependency %s: %w", entry.File, ref.Path, err)
			}
		}
		inventory.Entries = append(inventory.Entries, entry)
		delete(visiting, name)
		seen[name] = true
		return nil
	}
	if err := visit(target); err != nil {
		return nil, err
	}
	inventory.read = nil
	inventory.inspect = nil
	return inventory, nil
}
