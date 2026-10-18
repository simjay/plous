package source

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// ValidateDocument checks document syntax where the file format is unambiguous.
// Helm templates and programming languages retain their original text semantics.
func ValidateDocument(name string, data []byte) error {
	if strings.Contains("/"+name, "/templates/") {
		return nil
	}
	if path.Ext(name) == ".json" {
		var document json.RawMessage
		if err := json.Unmarshal(data, &document); err != nil {
			return fmt.Errorf("invalid JSON; draft retained: %w", err)
		}
	}
	if path.Ext(name) != ".yaml" && path.Ext(name) != ".yml" && path.Base(name) != "Kustomization" {
		return nil
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("invalid YAML; draft retained: %w", err)
		}
	}
}
