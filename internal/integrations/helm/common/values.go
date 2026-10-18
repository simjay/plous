package common

import (
	"errors"

	"helm.sh/helm/v3/pkg/strvals"
	"sigs.k8s.io/yaml"
)

func ReadValues(files, overrides []string, input string) (map[string]any, error) {
	result := map[string]any{}
	for _, file := range files {
		data := []byte(input)
		if file != "-" {
			var err error
			data, err = readBoundedFile(file, 2<<20)
			if err != nil {
				return nil, err
			}
		}
		if len(data) > 2<<20 {
			return nil, errors.New("values exceed 2 MiB")
		}
		var values map[string]any
		if err := yaml.Unmarshal(data, &values); err != nil {
			return nil, err
		}
		result = mergeValues(result, values)
	}
	for _, override := range overrides {
		if err := strvals.ParseInto(override, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func mergeValues(base, overrides map[string]any) map[string]any {
	for key, value := range overrides {
		if child, ok := value.(map[string]any); ok {
			if prior, ok := base[key].(map[string]any); ok {
				value = mergeValues(prior, child)
			}
		}
		base[key] = value
	}
	return base
}
