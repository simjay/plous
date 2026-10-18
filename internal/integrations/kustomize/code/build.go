package code

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	kustomizecommon "github.com/simjay/plous/internal/integrations/kustomize/common"
	manifest "github.com/simjay/plous/internal/integrations/manifest"
)

type BuildResult struct {
	Path, Engine string
	Built        time.Time
	Resources    []manifest.Resource
	Observation  *kustomizecommon.Observation
}
type Builder func(context.Context, string) ([]byte, string, error)

func Build(ctx context.Context, source *Inventory, name string, builder Builder) (*BuildResult, error) {
	return build(ctx, source, name, builder, false)
}

// BuildHelm uses captured charts and values validated by the Code renderer.
func BuildHelm(ctx context.Context, source *Inventory, name string, builder Builder) (*BuildResult, error) {
	return build(ctx, source, name, builder, true)
}
func build(ctx context.Context, source *Inventory, name string, builder Builder, helm bool) (*BuildResult, error) {
	if source == nil {
		return nil, errors.New("load a source before building")
	}
	if err := source.validateBuildOptions(name, map[string]bool{}, map[string]bool{}, helm); err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp("", "plous-kustomize-build-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	for _, name := range source.Paths() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		target := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, source.Files[name], 0600); err != nil {
			return nil, err
		}
	}
	var observed *observer
	if builder == nil {
		observed = newObserver(directory)
		builder = func(ctx context.Context, selected string) ([]byte, string, error) {
			return observedBuild(ctx, selected, func() error {
				if helm {
					return prepareHelm(ctx, source, name, directory)
				}
				return nil
			}, observed)
		}
	}

	output, engine, err := builder(ctx, filepath.Join(directory, filepath.FromSlash(name)))
	if err != nil {
		return nil, err
	}
	if len(output) > 16<<20 {
		return nil, errors.New("rendered output exceeds 16 MiB")
	}
	resources, err := manifest.Resources(string(output))
	if err != nil {
		return nil, err
	}
	if len(resources) > 5000 {
		return nil, errors.New("build exceeds 5,000 rendered resources")
	}
	identities := map[string]bool{}
	for _, r := range resources {
		if r.Name == "" {
			return nil, errors.New("rendered resource is missing metadata.name")
		}
		if identities[r.Key()] {
			return nil, fmt.Errorf("duplicate rendered resource: %s", r.Key())
		}
		identities[r.Key()] = true
	}
	result := &BuildResult{Path: name, Engine: engine, Built: time.Now().UTC(), Resources: resources}
	if observed != nil {
		result.Observation = observed.observation
	}
	return result, nil
}
