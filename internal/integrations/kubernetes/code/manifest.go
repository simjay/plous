// Package code recognizes and validates Kubernetes manifest source.
package code

import (
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/simjay/plous/internal/integrations/manifest"
)

var apiMarker = regexp.MustCompile(`(?m)^apiVersion:\s*`)
var kindMarker = regexp.MustCompile(`(?m)^kind:\s*`)

// Detect distinguishes resource input from unrelated YAML, including invalid resource input.
func Detect(data []byte) (bool, error) {
	resources, err := manifest.Resources(string(data))
	if err != nil {
		if apiMarker.Match(data) && kindMarker.Match(data) {
			return true, err
		}
		return false, nil
	}
	return len(resources) > 0 && !slices.ContainsFunc(resources, func(r manifest.Resource) bool { return r.Name == "" }), nil
}
func Validate(resources []manifest.Resource) error {
	if len(resources) > 5000 {
		return errors.New("render exceeds 5,000 resources")
	}
	seen := map[string]bool{}
	for _, resource := range resources {
		if resource.Name == "" {
			return errors.New("rendered resource is missing metadata.name")
		}
		if seen[resource.Key()] {
			return fmt.Errorf("duplicate resource identity: %s", resource.Key())
		}
		seen[resource.Key()] = true
	}
	return nil
}
