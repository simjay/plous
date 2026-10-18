package workspace

const ReadHelp = "Read saved configuration or Activity metadata within the captured connection. This tool cannot read assistant conversation or reasoning history; use history_read for that."

func ReadOperations() []string { return []string{"workspace", "activity"} }

func ReadSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"operation": map[string]any{"type": "string", "enum": ReadOperations()}, "cursor": map[string]any{"type": "string", "maxLength": 2048}, "limit": map[string]any{"type": "integer", "minimum": 0, "maximum": MaxResults, "default": DefaultResults}}, "required": []string{"operation"}, "additionalProperties": false}
}
