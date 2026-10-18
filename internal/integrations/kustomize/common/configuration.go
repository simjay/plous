package common

import (
	"bufio"
	"bytes"
	"io"
	"strings"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

type Reference struct {
	Relation, Path, Error string
	Entry                 bool
}

type Entry struct {
	Path, File, Error string
	Configuration     map[string]any
	References        []Reference
}

func (e Entry) Summary() string {
	var lines []string
	for _, key := range []string{"kind", "namespace", "namePrefix", "nameSuffix", "images", "replicas", "labels", "commonLabels", "commonAnnotations", "patches", "patchesStrategicMerge", "patchesJson6902", "replacements", "configMapGenerator", "secretGenerator", "generatorOptions"} {
		if value, ok := e.Configuration[key]; ok {
			lines = append(lines, strings.TrimSpace(MaskConfiguration([]byte(manifest.YAML(map[string]any{key: value}, false)))))
		}
	}
	if len(lines) == 0 {
		return "No transformations configured."
	}
	return strings.Join(lines, "\n\n")
}

func MaskConfiguration(data []byte) string {
	// Inline patches and generator literals can contain values without sensitive keys.
	var mask func(any) any
	mask = func(value any) any {
		switch v := value.(type) {
		case map[string]any:
			out := map[string]any{}
			for key, child := range v {
				if key == "literals" || key == "patch" || key == "value" || key == "patchesStrategicMerge" {
					out[key] = "[redacted]"
				} else {
					out[key] = mask(child)
				}
			}
			return out
		case []any:
			out := make([]any, len(v))
			for i, child := range v {
				out[i] = mask(child)
			}
			return out
		default:
			return value
		}
	}
	reader := kyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	var documents []string
	for {
		document, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "Invalid YAML; preview unavailable."
		}
		var value any
		if err := yaml.Unmarshal(document, &value); err != nil {
			return "Invalid YAML; preview unavailable."
		}
		if value != nil {
			documents = append(documents, manifest.YAML(mask(value), true))
		}
	}
	return strings.Join(documents, "---\n")
}
