package code

import (
	"errors"

	"sigs.k8s.io/yaml"
)

func ValidateMetadata(data []byte) error {
	var metadata map[string]any
	if err := yaml.UnmarshalStrict(data, &metadata); err != nil || metadata["name"] == nil || metadata["version"] == nil {
		return errors.New("Invalid chart metadata")
	}
	return nil
}
