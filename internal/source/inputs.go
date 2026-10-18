package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	helmcode "github.com/simjay/plous/internal/integrations/helm/code"
	kustomize "github.com/simjay/plous/internal/integrations/kustomize/code"
)

// captureInputs owns the only source-body collection used by a build. Local
// files are read now; an acquired Git source remains pinned to its commit.
func captureInputs(ctx context.Context, source *Snapshot, entry Entry, in Inputs) (*Inventory, error) {
	current := source
	captured := &Snapshot{Config: source.Config, Revision: source.Revision, Observed: time.Now().UTC(), Files: map[string][]byte{}}
	total := 0
	read := func(name string) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !fs.ValidPath(name) {
			return nil, errors.New("build input leaves the selected source")
		}
		if data, ok := captured.Files[name]; ok {
			return data, nil
		}
		data, err := current.readBytes(name)
		if err != nil {
			return nil, err
		}
		if len(captured.Files) >= maxFiles || total+len(data) > maxSource {
			return nil, errors.New("selected build inputs exceed 4,096 files or 32 MiB")
		}
		captured.Files[name] = data
		total += len(data)
		return data, nil
	}
	chart := func(root string) error {
		var rules []byte
		ignorePath := path.Join(root, ".helmignore")
		if _, err := current.inputKind(ignorePath); err == nil {
			rules, err = read(ignorePath)
			if err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		includes, err := helmcode.ChartIncludes(rules)
		if err != nil {
			return fmt.Errorf("%s: %w", ignorePath, err)
		}
		var visit func(string) error
		visit = func(directory string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries, err := current.inputDirectory(directory, func(name string, dir bool) (bool, error) {
				relative := name
				if root != "." {
					relative = strings.TrimPrefix(name, root+"/")
				}
				return inputPath(name) && includes(relative, dir), nil
			})
			if err != nil {
				return err
			}
			for _, file := range entries {
				if file.Mode&fs.ModeSymlink != 0 || (!file.Directory && !file.Mode.IsRegular()) {
					return fmt.Errorf("chart contains unsupported input: %s", file.Path)
				}
				if file.Directory {
					if err := visit(file.Path); err != nil {
						return err
					}
				} else if _, err := read(file.Path); err != nil {
					return err
				}
			}
			return nil
		}
		return visit(root)
	}
	switch entry.Kind {
	case "plain":
		if _, err := read(entry.File); err != nil {
			return nil, err
		}
	case "helm":
		if err := chart(entry.Path); err != nil {
			return nil, err
		}
		for _, name := range in.Values {
			if _, err := read(name); err != nil {
				return nil, err
			}
		}
	case "kustomize", "kustomize+helm":
		selected, err := kustomize.InspectTarget(entry.Path, read, current.inputKind)
		if err != nil {
			return nil, err
		}
		// Chart declarations belong to the Kustomize integration; composition owns acquisition.
		partial := &Inventory{Source: current, Kustomize: selected}
		for _, recipe := range selected.Entries {
			partial.Entries = append(partial.Entries, Entry{ID: "kustomize:" + recipe.Path, Kind: "kustomize", Path: recipe.Path, File: recipe.File, References: recipe.References})
		}
		for _, recipe := range partial.Entries {
			for _, declaration := range partial.declaredHelmCharts(recipe.ID) {
				if declaration.Status == "invalid" || declaration.Status == "out-of-scope" {
					return nil, errors.New(declaration.Error)
				}
				if declaration.Path != "" {
					_, err := current.inputKind(path.Join(declaration.Path, "Chart.yaml"))
					if errors.Is(err, os.ErrNotExist) && declaration.Status == "remote" {
						continue
					}
					if err != nil {
						return nil, err
					}
					if err := chart(declaration.Path); err != nil {
						return nil, err
					}
				}
			}
		}
	default:
		return nil, errors.New("rendering is unavailable for this entry type")
	}
	if current.git == nil && current.Files == nil {
		for name, data := range captured.Files {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			verified, err := current.readBytes(name)
			if err != nil || !bytes.Equal(data, verified) {
				return nil, fmt.Errorf("build input changed during capture: %s; retry the render", name)
			}
		}
	}
	captured.identify()
	return DiscoverContext(ctx, captured)
}
