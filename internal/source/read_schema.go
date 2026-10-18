package source

import "slices"

const FilesHelp = `Read the source captured for this turn. File tools do not wait for configuration discovery or fetch Git. Local file reads use current saved bytes; acquired Git reads remain pinned to their commit. list enumerates a directory with path, optional glob, offset, limit (1-100) and identity. read requires a file path and accepts offset, max_bytes (1-16384), and an expected content identity. search requires query and accepts a directory or exact file path, glob, regex, case_sensitive, limit and an opaque cursor. The default search is literal and case-sensitive. Globs without / match basenames; globs containing / match relative paths, using * ? and character classes (no recursive **). List and search apply the global profile .plousignore and repository .gitignore rules. An exact read bypasses scanning exclusions but never access restrictions. Search traverses permitted files in bounded pages. Continue with the same query, scope and options. Matches are excerpts, not repository-wide counts or an atomic local snapshot. Use their identities for follow-up reads. Paths are relative; traversal, symlinks, hidden files and credentials are denied. Unsaved drafts are not included. Use source_analysis_read for captured Code results.`
const AnalysisHelp = `Read captured discovery/render results: catalog lists entries, dependencies, consumers and result IDs; resources lists rendered resources; fields returns fields and provenance; findings returns diagnostics with coverage; compare compares a target/result against other result ID. Analysis accepts offset and limit 1-100. resources/fields/compare require target or result; fields requires resource and accepts field (JSON pointer). No rendering occurs. Captured analysis is timestamped and does not prove local files are still unchanged. File bytes and text search belong to source_files_read.`

func ReadSchema(tool string) map[string]any {
	var definitions []readDefinition
	for _, d := range readDefinitions {
		if d.tool == tool {
			definitions = append(definitions, d)
		}
	}
	properties := map[string]any{}
	for _, field := range []string{"path", "identity", "query", "glob", "cursor", "target", "result", "resource", "field", "other"} {
		properties[field] = map[string]any{"type": "string"}
	}
	for _, field := range []string{"regex", "case_sensitive"} {
		properties[field] = map[string]any{"type": "boolean"}
	}
	for _, field := range []string{"offset", "limit", "max_bytes"} {
		properties[field] = map[string]any{"type": "integer"}
	}
	properties["offset"] = map[string]any{"type": "integer", "minimum": 0}
	properties["limit"] = map[string]any{"type": "integer", "minimum": 0, "maximum": MaxReadResults, "description": "0 or omission uses the operation default"}
	properties["max_bytes"] = map[string]any{"type": "integer", "minimum": 0, "maximum": MaxFileReadBytes, "default": DefaultFileReadBytes}
	properties["query"] = map[string]any{"type": "string", "maxLength": 1024}
	properties["cursor"] = map[string]any{"type": "string", "maxLength": 4096}
	properties["identity"] = map[string]any{"type": "string", "maxLength": 64}
	var operations []string
	var variants []any
	for name := range properties {
		used := false
		for _, d := range definitions {
			used = used || slices.Contains(d.arguments, name)
		}
		if !used {
			delete(properties, name)
		}
	}
	for _, d := range definitions {
		operations = append(operations, d.operation)
		branch := map[string]any{"operation": map[string]any{"enum": []string{d.operation}}}
		for name := range properties {
			if !slices.Contains(d.arguments, name) {
				branch[name] = false
			}
		}
		for _, name := range d.required {
			branch[name] = map[string]any{"minLength": 1}
		}
		variant := map[string]any{"properties": branch}
		if len(d.required) > 0 {
			variant["required"] = d.required
		}
		if usesCapturedResult(d.operation) {
			variant["anyOf"] = []any{map[string]any{"required": []string{"target"}, "properties": map[string]any{"target": map[string]any{"minLength": 1}}}, map[string]any{"required": []string{"result"}, "properties": map[string]any{"result": map[string]any{"minLength": 1}}}}
		}
		variants = append(variants, variant)
	}
	properties["operation"] = map[string]any{"type": "string", "enum": operations}
	return map[string]any{"type": "object", "properties": properties, "required": []string{"operation"}, "additionalProperties": false, "dependentSchemas": map[string]any{"operation": map[string]any{"anyOf": variants}}}
}
