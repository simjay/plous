package source

import (
	"crypto/sha256"
	"fmt"
	"path"
	"slices"
	"strings"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
)

func FindingCategory(f Finding) string {
	switch f.Rule {
	case "mutable-image", "image":
		return "Images"
	case "overwritten-field", "duplicate-patch":
		return "Patches"
	case "render", "parse":
		return "Build"
	case "generator":
		return "Generators"
	default:
		return "Structure"
	}
}

func Images(r *Result) []Finding {
	var rows []Finding
	for _, resource := range r.Resources {
		inspectImages(manifest.Redact(resource.Object), "", func(field, image string) {
			state := "mutable"
			if imageHasDigest(image) {
				state = "pinned"
			}
			rows = append(rows, Finding{Severity: state, Rule: "image", Target: r.ID, File: r.Origins[resource.Key()], Resource: resource.Key(), Field: field, Summary: image, Message: resource.Kind + "/" + resource.Name + "\n" + field + "\n" + image + "\n" + state})
		})
	}
	slices.SortFunc(rows, func(a, b Finding) int { return strings.Compare(a.Resource+"/"+a.Field, b.Resource+"/"+b.Field) })
	return rows
}

func (i *Inventory) Generators() []Finding { return slices.Clone(i.generators) }
func (i *Inventory) inspectGenerators() {
	var rows []Finding
	for _, entry := range i.Kustomize.Entries {
		for _, kind := range []string{"configMapGenerator", "secretGenerator"} {
			items, _ := entry.Configuration[kind].([]any)
			for _, item := range items {
				config, _ := item.(map[string]any)
				name, _ := config["name"].(string)
				var keys, inputs []string
				for _, field := range []string{"literals", "files", "envs"} {
					values, _ := config[field].([]any)
					for _, value := range values {
						text, _ := value.(string)
						key, file, has := strings.Cut(text, "=")
						switch field {
						case "literals":
							if has {
								keys = append(keys, key)
							}
						case "files":
							if !has {
								file, key = text, path.Base(text)
							}
							keys = append(keys, key)
							inputs = append(inputs, path.Join(entry.Path, file))
						case "envs":
							file = path.Join(entry.Path, text)
							inputs = append(inputs, file)
							for _, line := range strings.Split(string(i.discoveryFile(file)), "\n") {
								key, _, has := strings.Cut(strings.TrimSpace(line), "=")
								if has && !strings.HasPrefix(key, "#") {
									keys = append(keys, strings.TrimSpace(key))
								}
							}
						}
					}
				}
				slices.Sort(keys)
				keys = slices.Compact(keys)
				keyCount := fmt.Sprintf("%d keys", len(keys))
				if len(keys) == 1 {
					keyCount = "1 key"
				}
				inputText := strings.Join(inputs, ", ")
				if inputText == "" {
					inputText = "literals only"
				}
				message := kind + " " + name + "\nDeclared keys: " + strings.Join(keys, ", ") + "\nInputs: " + inputText + "\nValues are hidden. Render findings report unavailable inputs."
				finding := Finding{Severity: map[string]string{"configMapGenerator": "ConfigMap", "secretGenerator": "Secret"}[kind], Rule: "generator", Target: "kustomize:" + entry.Path, File: entry.File, Summary: kind + " " + name + " · " + keyCount, Message: message}
				if !i.keep(finding) {
					return
				}
				rows = append(rows, finding)
			}
		}
	}
	i.generators = rows
}

func (i *Inventory) inspectStructure() {
	patches := map[[32]byte][]string{}
	owners := map[string]string{}
	for _, entry := range i.Entries {
		if entry.Component && len(i.Consumers(entry.ID)) == 0 {
			i.add("info", "unused-component", entry.ID, entry.File, "No kustomization in the loaded source includes this component.")
		}
		if entry.Kind == "plain" && len(i.Kustomize.Entries) > 0 {
			i.add("info", "unreferenced-manifest", entry.ID, entry.File, "No kustomization in the loaded source references this manifest. It may be an intentional standalone entry.")
		}
		for _, ref := range entry.References {
			if !strings.HasPrefix(ref.Relation, "patches") || ref.Error != "" {
				continue
			}
			data := i.discoveryFile(ref.Path)
			if len(data) == 0 {
				continue
			}
			if _, seen := owners[ref.Path]; seen {
				continue
			}
			owners[ref.Path] = entry.ID
			digest := sha256.Sum256(data)
			patches[digest] = append(patches[digest], ref.Path)
		}
	}
	var groups [][]string
	for _, files := range patches {
		if len(files) > 1 {
			slices.Sort(files)
			groups = append(groups, files)
		}
	}
	slices.SortFunc(groups, func(a, b []string) int { return strings.Compare(a[0], b[0]) })
	for _, files := range groups {
		i.add("info", "duplicate-patch", owners[files[0]], files[0], "Byte-identical patch files: "+strings.Join(files, ", ")+". Consider a shared component if their intended behavior is the same.")
	}
}
