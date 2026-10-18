package code

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strings"

	helm "github.com/simjay/plous/internal/integrations/helm/common"
	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/plousobserve"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/kustomize/kyaml/openapi"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
	"sigs.k8s.io/kustomize/kyaml/yaml/merge2"
	"sigs.k8s.io/yaml"
)

const EngineVersion = "krusty v0.21.2"

// Krusty maintains process-wide OpenAPI state. Serialize builds to isolate schemas.
var engineSlot = make(chan struct{}, 1)

type buildFS struct {
	filesys.FileSystem
	ctx context.Context
}

func (f buildFS) ReadFile(name string) ([]byte, error) {
	if err := f.ctx.Err(); err != nil {
		return nil, err
	}
	return f.FileSystem.ReadFile(name)
}
func observedBuild(ctx context.Context, directory string, prepare func() error, observation *observer) ([]byte, string, error) {
	select {
	case engineSlot <- struct{}{}:
		defer func() { <-engineSlot }()
	case <-ctx.Done():
		return nil, EngineVersion, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, EngineVersion, err
	}
	openapi.ResetOpenAPI()
	defer openapi.ResetOpenAPI()
	if err := prepare(); err != nil {
		return nil, EngineVersion, err
	}
	options := krusty.MakeDefaultOptions()
	if observation != nil {
		restore := plousobserve.Install(observation.capture)
		defer restore()
	}
	options.Reorder = krusty.ReorderOptionUnspecified
	result, err := krusty.MakeKustomizer(options).Run(buildFS{filesys.MakeFsOnDisk(), ctx}, directory)
	if ctx.Err() != nil {
		return nil, EngineVersion, ctx.Err()
	}
	if err != nil {
		return nil, EngineVersion, err
	}
	if observation != nil {
		observation.finish(result)
	}
	data, err := result.AsYaml()
	return data, EngineVersion, err
}

func prepareHelm(ctx context.Context, s *Inventory, name, root string) error {
	seen := map[string]string{}
	variants := 0
	var visit func(string, string, string) (string, error)
	visit = func(name, inheritedNamespace, materialized string) (string, error) {
		entry := s.Entry(name)
		if entry == nil {
			return "", fmt.Errorf("missing kustomization %s", name)
		}
		namespace, _ := entry.Configuration["namespace"].(string)
		if namespace == "" {
			namespace = inheritedNamespace
		}
		directory := filepath.Join(materialized, filepath.FromSlash(name))
		if prior, ok := seen[directory]; ok {
			if prior == namespace {
				return directory, nil
			}
			// The same base can be included by overlays with different Helm namespaces.
			// Give that invocation its own captured tree so rendered charts cannot leak.
			variants++
			if variants > 64 {
				return "", fmt.Errorf("too many distinct Helm namespace variants")
			}
			variant, err := os.MkdirTemp(root, "plous-variant-")
			if err != nil {
				return "", err
			}
			if err := copyVariant(ctx, s, root, variant); err != nil {
				return "", err
			}
			materialized = variant
			directory = filepath.Join(materialized, filepath.FromSlash(name))
		}
		seen[directory] = namespace
		config := maps.Clone(entry.Configuration)
		for _, field := range []string{"resources", "bases", "components"} {
			values, _ := config[field].([]any)
			rewritten := append([]any(nil), values...)
			for i, value := range values {
				relative, ok := value.(string)
				if !ok {
					continue
				}
				child := path.Clean(path.Join(name, relative))
				if s.Entry(child) == nil {
					continue
				}
				target, err := visit(child, namespace, materialized)
				if err != nil {
					return "", err
				}
				relativeTarget, err := filepath.Rel(directory, target)
				if err != nil {
					return "", err
				}
				rewritten[i] = filepath.ToSlash(relativeTarget)
			}
			if len(values) > 0 {
				config[field] = rewritten
			}
		}
		var input struct {
			HelmCharts  []types.HelmChart `json:"helmCharts"`
			HelmGlobals types.HelmGlobals `json:"helmGlobals"`
		}
		if err := yaml.Unmarshal(s.Files[entry.File], &input); err != nil {
			return "", err
		}
		if len(input.HelmCharts) > 0 {
			generated, err := os.MkdirTemp(directory, "plous-helm-")
			if err != nil {
				return "", err
			}
			var resources strings.Builder
			for _, chart := range input.HelmCharts {
				if chart.Namespace == "" {
					chart.Namespace = namespace
				}
				data, err := renderChart(ctx, directory, generated, input.HelmGlobals, chart)
				if err != nil {
					return "", fmt.Errorf("%s: %w", entry.File, err)
				}
				objects, err := manifest.Resources(string(data))
				if err != nil {
					return "", err
				}
				for _, resource := range objects {
					metadata, _ := resource.Object["metadata"].(map[string]any)
					if metadata == nil {
						metadata = map[string]any{}
						resource.Object["metadata"] = metadata
					}
					annotations, _ := metadata["annotations"].(map[string]any)
					if annotations == nil {
						annotations = map[string]any{}
						metadata["annotations"] = annotations
					}
					annotations[konfig.HelmGeneratedAnnotation] = "true"
					encoded, err := yaml.Marshal(resource.Object)
					if err != nil {
						return "", err
					}
					resources.WriteString("---\n")
					resources.Write(encoded)
				}
			}
			if err := os.WriteFile(filepath.Join(generated, "resources.yaml"), []byte(resources.String()), 0600); err != nil {
				return "", err
			}
			if err := os.WriteFile(filepath.Join(generated, "kustomization.yaml"), []byte("apiVersion: kustomize.config.k8s.io/v1alpha1\nkind: Component\nresources: [resources.yaml]\n"), 0600); err != nil {
				return "", err
			}
			// Components share their parent's accumulator. Insert chart output after
			// built-in generators and before user components and transformations.
			components, _ := config["components"].([]any)
			config["components"] = append([]any{filepath.Base(generated)}, components...)
			delete(config, "helmCharts")
			delete(config, "helmGlobals")
		}
		data, err := yaml.Marshal(config)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(materialized, filepath.FromSlash(entry.File)), data, 0600); err != nil {
			return "", err
		}
		return directory, nil
	}
	_, err := visit(name, "", root)
	return err
}

func copyVariant(ctx context.Context, s *Inventory, original, variant string) error {
	configurations := map[string]bool{}
	for _, entry := range s.Entries {
		configurations[entry.File] = true
	}
	for _, name := range s.Paths() {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := filepath.Join(variant, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if configurations[name] {
			if err := os.WriteFile(target, s.Files[name], 0600); err != nil {
				return err
			}
		} else if err := os.Link(filepath.Join(original, filepath.FromSlash(name)), target); err != nil {
			return err
		}
	}
	return nil
}

func renderChart(ctx context.Context, directory, temp string, globals types.HelmGlobals, chart types.HelmChart) ([]byte, error) {
	home := globals.ChartHome
	if home == "" {
		home = types.HelmDefaultHome
	}
	home = filepath.Join(directory, home)
	// Code captures charts at chartHome/name before rendering, independent of repository.
	valuesFile := chart.ValuesFile
	if valuesFile == "" {
		valuesFile = filepath.Join(home, chart.Name, "values.yaml")
	} else {
		valuesFile = filepath.Join(directory, valuesFile)
	}
	values, err := os.ReadFile(valuesFile)
	if err != nil {
		return nil, err
	}
	if len(chart.ValuesInline) > 0 {
		inline, err := kyaml.FromMap(chart.ValuesInline)
		if err != nil {
			return nil, err
		}
		base, err := kyaml.Parse(string(values))
		if err != nil {
			return nil, err
		}
		var merged *kyaml.RNode
		switch chart.ValuesMerge {
		case "", "override":
			merged, err = merge2.Merge(inline, base, kyaml.MergeOptions{})
		case "merge":
			merged, err = merge2.Merge(base, inline, kyaml.MergeOptions{})
		case "replace":
			merged = inline
		default:
			return nil, fmt.Errorf("unsupported Helm valuesMerge %q", chart.ValuesMerge)
		}
		if err != nil {
			return nil, err
		}
		text, err := merged.String()
		if err != nil {
			return nil, err
		}
		values = []byte(text)
	} else if chart.ValuesMerge != "" && chart.ValuesMerge != "merge" && chart.ValuesMerge != "override" && chart.ValuesMerge != "replace" {
		return nil, fmt.Errorf("unsupported Helm valuesMerge %q", chart.ValuesMerge)
	}
	file, err := os.CreateTemp(temp, "values-*.yaml")
	if err != nil {
		return nil, err
	}
	if _, err = file.Write(values); err != nil {
		file.Close()
		return nil, err
	}
	if err = file.Close(); err != nil {
		return nil, err
	}
	chart.ValuesFile = file.Name()
	for i, value := range chart.AdditionalValuesFiles {
		chart.AdditionalValuesFiles[i] = filepath.Join(directory, value)
	}
	release := chart.ReleaseName
	if release == "" {
		release = "release-name"
	}
	valuesFiles := append([]string{chart.ValuesFile}, chart.AdditionalValuesFiles...)
	return helm.Render(ctx, helm.TemplateRequest{ChartPath: filepath.Join(home, chart.Name), Release: release, Namespace: chart.Namespace,
		ValuesFiles: valuesFiles, APIVersions: chart.ApiVersions, KubeVersion: chart.KubeVersion, NameTemplate: chart.NameTemplate,
		IncludeCRDs: chart.IncludeCRDs, SkipTests: chart.SkipTests, NoHooks: chart.SkipHooks})
}
