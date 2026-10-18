package source

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	kustomizecommon "github.com/simjay/plous/internal/integrations/kustomize/common"
	manifest "github.com/simjay/plous/internal/integrations/manifest"
	yaml "go.yaml.in/yaml/v3"
)

type FieldWrite struct {
	Stage             int
	Scope, Kind, File string
	Line              int
	Before, After     string
}

// FieldValue compares the same named field across verified resource lineage.
func FieldValue(resource, reference manifest.Resource, pointer string) (string, bool) {
	v, exists, safe := alignedField(manifest.Redact(resource.Object), manifest.Redact(reference.Object), pointer)
	return fieldText(v, exists), safe
}

func observedWrites(i *Inventory, r *Result, resource manifest.Resource, pointer string) ([]FieldWrite, bool) {
	observation := r.Observation
	if observation == nil || r.captured == nil && r.SourceIdentity != i.Source.Identity {
		return nil, false
	}
	id := observation.FinalIDs[resource.Key()]
	if id == 0 {
		return nil, false
	}
	var writes []FieldWrite
	complete := !observation.Truncated
	final := manifest.Redact(resource.Object)
	for n, stage := range observation.Stages {
		before, had := stage.Before[id]
		after, has := stage.After[id]
		if !had && !has {
			continue
		}
		a, aok, asafe := alignedField(before.Object, final, pointer)
		b, bok, bsafe := alignedField(after.Object, final, pointer)
		if !asafe || !bsafe {
			complete = false
			continue
		}
		if aok == bok && manifest.JSON(a) == manifest.JSON(b) {
			continue
		}
		file, line := stageLocation(i, stage)
		writes = append(writes, FieldWrite{Stage: n + 1, Scope: stage.Scope, Kind: stage.Kind, File: file, Line: line, Before: fieldText(a, aok), After: fieldText(b, bok)})
	}
	if len(writes) > 0 {
		value, exists, safe := alignedField(final, final, pointer)
		if !safe || writes[len(writes)-1].After != fieldText(value, exists) {
			complete = false
		}
	}
	return writes, complete
}
func fieldText(v any, exists bool) string {
	if !exists {
		return "<absent>"
	}
	return fmt.Sprint(v)
}

// Align named list members against the final object. For unnamed lists, only
// identical lists are safe; indices alone do not establish field lineage.
func CorrespondingField(resource, reference manifest.Resource, pointer string) (string, bool) {
	_, exists, safe, mapped := alignedFieldPath(manifest.Redact(resource.Object), manifest.Redact(reference.Object), pointer)
	return mapped, exists && safe
}

func alignedField(object, final any, pointer string) (any, bool, bool) {
	value, exists, safe, _ := alignedFieldPath(object, final, pointer)
	return value, exists, safe
}

func alignedFieldPath(object, final any, pointer string) (any, bool, bool, string) {
	var mapped []string
	for _, token := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		key := strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		switch reference := final.(type) {
		case map[string]any:
			mapped = append(mapped, token)
			final = reference[key]
			current, ok := object.(map[string]any)
			if !ok {
				return nil, false, true, ""
			}
			object, ok = current[key]
			if !ok {
				return nil, false, true, ""
			}
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(reference) {
				return nil, false, false, ""
			}
			current, ok := object.([]any)
			if !ok {
				return nil, false, true, ""
			}
			member, named := reference[index].(map[string]any)
			name, _ := member["name"].(string)
			if named && name != "" {
				matches := 0
				var found any
				foundIndex := 0
				for n, v := range current {
					if m, ok := v.(map[string]any); ok && m["name"] == name {
						found = v
						foundIndex = n
						matches++
					}
				}
				finalMatches := 0
				for _, v := range reference {
					if m, ok := v.(map[string]any); ok && m["name"] == name {
						finalMatches++
					}
				}
				if matches > 1 || finalMatches != 1 {
					return nil, false, false, ""
				}
				if matches == 0 {
					return nil, false, true, ""
				}
				object = found
				mapped = append(mapped, strconv.Itoa(foundIndex))
			} else {
				if manifest.JSON(current) != manifest.JSON(reference) {
					return nil, false, false, ""
				}
				object = current[index]
				mapped = append(mapped, key)
			}
			final = reference[index]
		default:
			return nil, false, false, ""
		}
	}
	return object, true, true, "/" + strings.Join(mapped, "/")
}

func stageLocation(i *Inventory, stage kustomizecommon.Stage) (string, int) {
	entry := i.Kustomize.Entry(stage.Scope)
	if entry == nil {
		return "", 0
	}
	if stage.Path != "" {
		file := path.Join(stage.Scope, stage.Path)
		if i.Source.Files[file] != nil {
			return file, 0
		}
	}
	key := map[string]string{"PatchTransformer": "patches", "PatchStrategicMergeTransformer": "patchesStrategicMerge", "PatchJson6902Transformer": "patchesJson6902", "ReplicaCountTransformer": "replicas", "NamespaceTransformer": "namespace", "PrefixTransformer": "namePrefix", "SuffixTransformer": "nameSuffix", "LabelTransformer": "labels", "AnnotationsTransformer": "commonAnnotations", "ImageTagTransformer": "images", "ReplacementTransformer": "replacements"}[stage.Kind]
	if stage.Kind == "LabelTransformer" && entry.Configuration["commonLabels"] != nil {
		key = "commonLabels"
	}
	var node yaml.Node
	lines := map[string]int{}
	if yaml.Unmarshal(i.Source.Files[entry.File], &node) == nil && len(node.Content) > 0 {
		nodeLines(node.Content[0], "", lines)
	}
	if !stage.External && lines["/"+key] > 0 {
		return entry.File, lines["/"+key]
	}
	// A unique referenced builtin configuration is useful evidence. Multiple
	// configurations of the same kind remain ambiguous and keep only the scope.
	file := ""
	for _, ref := range entry.References {
		if ref.Relation != "transformers" {
			continue
		}
		var config struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
		}
		if stage.External && yaml.Unmarshal(i.Source.Files[ref.Path], &config) == nil && config.Kind == stage.Kind && config.Metadata.Name == stage.ConfigurationName {
			if file != "" {
				return "", 0
			}
			file = ref.Path
		}
	}
	return file, 0
}

func TransformationLabel(kind string) string {
	label := map[string]string{"PatchTransformer": "Patch", "PatchStrategicMergeTransformer": "Strategic merge patch", "PatchJson6902Transformer": "JSON patch", "ReplicaCountTransformer": "Replicas", "NamespaceTransformer": "Namespace", "PrefixTransformer": "Name prefix", "SuffixTransformer": "Name suffix", "PrefixSuffixTransformer": "Name prefix / suffix", "LabelTransformer": "Labels", "AnnotationsTransformer": "Annotations", "ImageTagTransformer": "Images", "ReplacementTransformer": "Replacements", "HashTransformer": "Generated names"}[kind]
	if label == "" {
		return kind
	}
	return label
}
