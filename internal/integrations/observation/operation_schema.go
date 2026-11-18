package observation

import (
	"maps"
	"slices"
)

type OperationVariant struct {
	Operation string
	Arguments []string
	Schema    map[string]any
}

// OperationSchema keeps combinators off the root for MCP clients that require a
// plain object there. Variant constraints apply to fields admitted by the root.
func OperationSchema(properties map[string]any, variants []OperationVariant) map[string]any {
	fields := maps.Clone(properties)
	if fields == nil {
		fields = map[string]any{}
	}
	var operations []string
	for _, variant := range variants {
		if !slices.Contains(operations, variant.Operation) {
			operations = append(operations, variant.Operation)
		}
		for _, field := range variant.Arguments {
			if _, exists := fields[field]; !exists {
				fields[field] = map[string]any{}
			}
		}
	}

	branches := make([]any, 0, len(variants))
	for _, variant := range variants {
		branch := maps.Clone(variant.Schema)
		if branch == nil {
			branch = map[string]any{}
		}
		overrides, _ := branch["properties"].(map[string]any)
		allowed := map[string]any{"operation": map[string]any{"enum": []string{variant.Operation}}}
		maps.Copy(allowed, overrides)
		for field := range fields {
			if !slices.Contains(variant.Arguments, field) {
				allowed[field] = false
			}
		}
		branch["properties"] = allowed
		branches = append(branches, branch)
	}
	fields["operation"] = map[string]any{"type": "string", "enum": operations}
	return map[string]any{
		"type": "object", "properties": fields, "required": []string{"operation"}, "additionalProperties": false,
		"dependentSchemas": map[string]any{"operation": map[string]any{"anyOf": branches}},
	}
}
