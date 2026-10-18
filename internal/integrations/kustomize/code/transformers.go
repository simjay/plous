package code

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"

	kustomizecommon "github.com/simjay/plous/internal/integrations/kustomize/common"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/yaml"
)

// Native builtins do not require enabling executable or network plugins.
func (s *Inventory) transformerInputs(e kustomizecommon.Entry) ([]string, error) {
	var inputs []string
	for _, item := range list(e.Configuration["transformers"]) {
		value := text(item)
		name, data := e.File+" (inline transformer)", []byte(value)
		if !strings.Contains(value, "\n") {
			name = path.Join(e.Path, value)
			var err error
			data, err = s.readFile(name)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
		}
		reader := kyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
		for {
			document, err := reader.Read()
			if err == io.EOF {
				break
			}
			var config map[string]any
			if err != nil || yaml.UnmarshalStrict(document, &config) != nil {
				return nil, fmt.Errorf("%s: invalid transformer configuration", name)
			}
			if config == nil {
				continue
			}
			kind := text(config["kind"])
			if text(config["apiVersion"]) != "builtin" || kind == "HelmChartInflationGenerator" {
				return nil, fmt.Errorf("%s: external transformer %s is unavailable; native builtin transformers are supported", name, kind)
			}
			switch kind {
			case "PatchTransformer", "PatchJson6902Transformer":
				if file := text(config["path"]); file != "" {
					inputs = append(inputs, file)
				}
			case "PatchStrategicMergeTransformer":
				factory := provider.NewDefaultDepProvider().GetResourceFactory()
				for _, value := range list(config["paths"]) {
					file := text(value)
					// The native builtin first tries each entry as inline patch YAML.
					if _, err := factory.SliceFromBytes([]byte(file)); err != nil {
						inputs = append(inputs, file)
					}
				}
			case "ReplacementTransformer":
				for _, replacement := range list(config["replacements"]) {
					if file := text(object(replacement)["path"]); file != "" {
						inputs = append(inputs, file)
					}
				}
			case "ValueAddTransformer":
				if file := text(config["targetFilePath"]); file != "" {
					inputs = append(inputs, file)
				}
			}
		}
	}
	return inputs, nil
}
