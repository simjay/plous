package monitor

import "slices"

const ReleasesHelp = "Inspect Helm releases on the captured cluster: list, status, metadata, history or chart. list requires no name; other operations require a release name. Namespace * supports list only. revision selects a retained revision where supported; omission selects the latest. Use helm_configuration_read for values/manifests/hooks and helm_diff_read for revision differences."
const ConfigurationHelp = "Read masked Helm values, manifest or hooks for a named release and optional retained revision. Values include chart defaults. Manifest and hooks return resource identities and masked scalar fields. Namespace must be concrete. Release notes and writes are unavailable."
const CompareHelp = "Compare retained Helm revisions of one named release. revision selects the after revision; compare_revision selects the before revision. Omitting compare_revision compares against the preceding revision (revision 1 uses an empty baseline). Results contain masked field changes and completeness. This reads recorded revisions, without rendering or changing a release."

func ReadSchema(tool string) map[string]any {
	var definitions []ReadDefinition
	for _, d := range ReadCommands() {
		if d.Tool == tool {
			definitions = append(definitions, d)
		}
	}
	properties := map[string]any{}
	var operations []string
	var variants []any
	for _, definition := range definitions {
		operations = append(operations, definition.Operation)
		variants = append(variants, map[string]any{"properties": map[string]any{"operation": map[string]any{"enum": []string{definition.Operation}}}, "required": definition.Required})
		for _, name := range definition.Arguments {
			typ := "string"
			switch name {
			case "revision", "compare_revision":
				typ = "integer"
			}
			field := map[string]any{"type": typ}
			if name == "revision" {
				field["minimum"], field["default"] = 0, 0
			}
			if name == "compare_revision" {
				field["minimum"] = 1
			}
			if name == "name" {
				field["minLength"], field["maxLength"] = 1, 53
			}
			properties[name] = field
		}
	}
	for i, d := range definitions {
		branch := variants[i].(map[string]any)["properties"].(map[string]any)
		for name := range properties {
			if !slices.Contains(d.Arguments, name) {
				branch[name] = false
			}
		}
	}
	properties["operation"] = map[string]any{"type": "string", "enum": operations}
	schema := map[string]any{"type": "object", "properties": properties, "required": []string{"operation"}, "dependentSchemas": map[string]any{"operation": map[string]any{"anyOf": variants}}, "additionalProperties": false}
	if len(definitions) == 1 {
		delete(properties, "operation")
		delete(schema, "dependentSchemas")
		schema["required"] = definitions[0].Required
	}
	if tool == "helm_diff_read" {
		schema["if"] = map[string]any{"required": []string{"compare_revision"}}
		schema["then"] = map[string]any{"required": []string{"revision"}, "properties": map[string]any{"revision": map[string]any{"minimum": 1}}}
	}
	return schema
}
