package code

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	kustomizecommon "github.com/simjay/plous/internal/integrations/kustomize/common"
	"sigs.k8s.io/yaml"
)

func isKustomization(name string) bool {
	return name == "kustomization.yaml" || name == "kustomization.yml" || name == "Kustomization"
}
func (s *Inventory) discover(retain func(int) error) error {
	account := func(value any) error {
		if retain == nil {
			return nil
		}
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return retain(len(data) + 128)
	}
	byPath := map[string]int{}
	for _, file := range s.Paths() {
		if !isKustomization(path.Base(file)) {
			continue
		}
		directory := path.Dir(file)
		e := kustomizecommon.Entry{Path: directory, File: file}
		data, err := s.readFile(file)
		if err != nil {
			e.Error = "Kustomization input unavailable"
		} else if err := yaml.UnmarshalStrict(data, &e.Configuration); err != nil {
			e.Error = "Invalid kustomization YAML"
		}
		// A Flux Kustomization is a resource declaration, not a build recipe.
		if api := text(e.Configuration["apiVersion"]); api != "" && !strings.HasPrefix(api, "kustomize.config.k8s.io/") {
			continue
		}
		if i, ok := byPath[directory]; ok {
			s.Entries[i].Error = "Multiple kustomization files in this directory"
			continue
		}
		if err := account(e); err != nil {
			return err
		}
		byPath[directory] = len(s.Entries)
		s.Entries = append(s.Entries, e)
	}
	sort.Slice(s.Entries, func(i, j int) bool { return s.Entries[i].Path < s.Entries[j].Path })
	for i := range s.Entries {
		refs := s.references(s.Entries[i])
		if err := account(refs); err != nil {
			return err
		}
		s.Entries[i].References = refs
	}
	return nil
}
func list(v any) []any            { values, _ := v.([]any); return values }
func object(v any) map[string]any { values, _ := v.(map[string]any); return values }
func text(v any) string           { value, _ := v.(string); return value }

func (s *Inventory) references(e kustomizecommon.Entry) []kustomizecommon.Reference {
	var refs []kustomizecommon.Reference
	add := func(kind, value string, keyValue bool) {
		if keyValue {
			if _, file, found := strings.Cut(value, "="); found {
				value = file
			}
		}
		if value == "" {
			return
		}
		r := kustomizecommon.Reference{Relation: kind, Path: value}
		if strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\\x00\r\n") || strings.Contains(value, ":") || strings.Contains(value, "?") || strings.HasPrefix(value, "github.com/") || strings.HasPrefix(value, "gitlab.com/") {
			r.Error = "External references are unavailable; place dependencies inside the selected source"
		} else {
			r.Path = path.Join(e.Path, value)
			if r.Path == ".." || strings.HasPrefix(r.Path, "../") {
				r.Error = "Reference leaves the selected source"
			} else if s.inspect != nil {
				dir, err := s.inspect(r.Path)
				if err != nil {
					r.Error = err.Error()
				} else {
					r.Entry = dir
				}
			} else if target := s.Entry(r.Path); target != nil {
				r.Entry = true
			} else if !s.has(r.Path) {
				r.Error = "File unavailable in this source snapshot"
			}
		}
		refs = append(refs, r)
	}
	c := e.Configuration
	for _, field := range []string{"resources", "bases", "components", "configurations", "crds", "generators", "transformers"} {
		for _, item := range list(c[field]) {
			if field == "transformers" && strings.Contains(text(item), "\n") {
				continue
			}
			add(field, text(item), false)
		}
	}
	for _, field := range []string{"patches", "patchesJson6902", "replacements"} {
		for _, item := range list(c[field]) {
			add(field, text(object(item)["path"]), false)
		}
	}
	for _, item := range list(c["patchesStrategicMerge"]) {
		if value := text(item); value != "" && !strings.Contains(value, "\n") {
			add("patchesStrategicMerge", value, false)
		}
	}
	for _, field := range []string{"configMapGenerator", "secretGenerator"} {
		for _, item := range list(c[field]) {
			generator := object(item)
			for _, value := range list(generator["files"]) {
				add(field+" files", text(value), true)
			}
			for _, value := range list(generator["envs"]) {
				add(field+" envs", text(value), false)
			}
			add(field+" env", text(generator["env"]), false)
		}
	}
	for _, item := range list(c["helmCharts"]) {
		chart := object(item)
		add("helm values", text(chart["valuesFile"]), false)
		for _, value := range list(chart["additionalValuesFiles"]) {
			add("helm values", text(value), false)
		}
	}
	add("openapi", text(object(c["openapi"])["path"]), false)
	inputs, err := s.transformerInputs(e)
	if err != nil {
		refs = append(refs, kustomizecommon.Reference{Relation: "transformers", Path: e.File, Error: err.Error()})
	}
	for _, input := range inputs {
		add("transformer input", input, false)
	}
	return refs
}

func (s *Inventory) validateBuildOptions(name string, visiting map[string]bool, done map[string]bool, helm bool) error {
	if done[name] {
		return nil
	}
	if visiting[name] {
		return fmt.Errorf("dependency cycle at %s", name)
	}
	e := s.Entry(name)
	if e == nil {
		return fmt.Errorf("kustomization %s is unavailable", name)
	}
	if e.Error != "" {
		return fmt.Errorf("%s: %s", name, e.Error)
	}
	visiting[name] = true
	allowed := strings.Fields("apiVersion kind metadata resources bases components namespace namePrefix nameSuffix commonLabels commonAnnotations labels annotations images replicas patches patchesJson6902 patchesStrategicMerge replacements configMapGenerator secretGenerator generatorOptions configurations crds vars openapi sortOptions buildMetadata transformers")
	if helm {
		allowed = append(allowed, "helmCharts", "helmGlobals")
	}
	for key := range e.Configuration {
		found := false
		for _, field := range allowed {
			if key == field {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: configuration field %s is not supported by Plous builds", name, key)
		}
	}
	for _, ref := range e.References {
		if ref.Error != "" {
			return fmt.Errorf("%s: %s: %s", name, ref.Path, ref.Error)
		}
		if ref.Entry {
			if err := s.validateBuildOptions(ref.Path, visiting, done, helm); err != nil {
				return err
			}
		}
	}
	delete(visiting, name)
	done[name] = true
	return nil
}
