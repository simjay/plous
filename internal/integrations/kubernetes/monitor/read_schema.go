package monitor

import "slices"

const ResourcesHelp = "Read projected Kubernetes resources with get or describe. get lists when name is omitted; describe requires a name. Namespace defaults to the captured connection; * lists all namespaces. Go follows API pagination and computes counts, readiness and ownership facts. Continue saved rows through history_read using page.next_offset. Workload generation and missing fields remain explicit. Events are resources. Secrets, exec and writes are denied. Use kubernetes_logs_read for logs and kubernetes_usage_read for identity-checked measurements."
const LogsHelp = "Read bounded logs for a named Pod or supported workload in one namespace, with an explicit container. Workload reads follow controller ownership and include at most 16 Pods. tail_lines bounds each source; max_bytes bounds the combined excerpts. Both 0 and omission use defaults. since_seconds and previous select the log window. Logs are excerpts, never a complete history; missing sources and collection limits remain explicit."
const UsageHelp = "Read current identity-checked CPU and memory usage for a named Pod, Node or supported workload. Returns CPU nanocores, memory bytes, declared budgets, source intervals and coverage. Workload measurements follow ownership. Unknown and incomplete measurements are distinct from zero."

func ReadSchema(tool string) map[string]any {
	var definitions []CommandDefinition
	for _, d := range ReadCommands() {
		if d.Tool == tool {
			definitions = append(definitions, d)
		}
	}
	return readSchema(definitions)
}

func readSchema(definitions []CommandDefinition) map[string]any {
	properties := map[string]any{}
	var operations []string
	var variants []any
	for _, definition := range definitions {
		operations = append(operations, definition.Operation)
		for _, name := range definition.Arguments {
			field := map[string]any{"type": "string"}
			switch name {
			case "resource", "name":
				field["minLength"], field["maxLength"] = 1, 253
			case "container":
				field["minLength"], field["maxLength"] = 1, 63
			case "label_selector", "field_selector":
				field["maxLength"] = MaxSelectorLength
			case "max_bytes":
				field = map[string]any{"type": "integer", "minimum": 0, "maximum": MaxAgentBytes, "default": MaxAgentBytes, "description": "0 uses the default"}
			case "tail_lines":
				field = map[string]any{"type": "integer", "minimum": 0, "maximum": MaxAgentLines, "default": DefaultAgentLines, "description": "0 uses the default"}
			case "since_seconds":
				field = map[string]any{"type": "integer", "minimum": 0, "maximum": MaxSinceSeconds}
			case "previous":
				field = map[string]any{"type": "boolean", "default": false}
			}
			properties[name] = field
		}
		variants = append(variants, map[string]any{"properties": map[string]any{"operation": map[string]any{"enum": []string{definition.Operation}}}, "required": definition.Required})
	}
	for i, definition := range definitions {
		branch := variants[i].(map[string]any)["properties"].(map[string]any)
		for name := range properties {
			if !slices.Contains(definition.Arguments, name) {
				branch[name] = false
			}
		}
		if definition.Operation == "logs" {
			branch["resource"] = map[string]any{"enum": []string{"pods", "deployments.apps", "statefulsets.apps", "daemonsets.apps", "replicasets.apps", "replicationcontrollers", "jobs.batch", "cronjobs.batch"}}
		}
	}
	if len(definitions) == 1 {
		definition := definitions[0]
		if definition.Operation == "logs" {
			properties["resource"] = variants[0].(map[string]any)["properties"].(map[string]any)["resource"]
		}
		return map[string]any{"type": "object", "properties": properties, "required": definition.Required, "additionalProperties": false}
	}
	properties["operation"] = map[string]any{"type": "string", "enum": operations}
	return map[string]any{"type": "object", "properties": properties, "required": []string{"operation"}, "dependentSchemas": map[string]any{"operation": map[string]any{"anyOf": variants}}, "additionalProperties": false}
}

// Controller reads retain the Kubernetes grammar with a narrower operation set.
func ControllerReadSchema(resources, operations []string) map[string]any {
	var definitions []CommandDefinition
	for _, definition := range ReadCommands() {
		if slices.Contains(operations, definition.Operation) {
			definitions = append(definitions, definition)
		}
	}
	schema := readSchema(definitions)
	schema["properties"].(map[string]any)["resource"] = map[string]any{"type": "string", "enum": resources}
	return schema
}
