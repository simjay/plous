package code

import (
	"strings"

	"sigs.k8s.io/kustomize/api/types"
)

func DeprecatedFields(data []byte) []string {
	var configuration types.Kustomization
	if configuration.Unmarshal(data) != nil {
		return nil
	}
	var messages []string
	for _, warning := range *configuration.CheckDeprecatedFields() {
		message, _, _ := strings.Cut(strings.TrimPrefix(warning, "# Warning: "), " Run '")
		messages = append(messages, strings.TrimSuffix(message, " [EXPERIMENTAL]"))
	}
	return messages
}
