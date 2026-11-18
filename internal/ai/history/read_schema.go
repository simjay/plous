package history

import (
	"slices"

	"github.com/simjay/plous/internal/integrations/observation"
)

const ReadHelp = "Search saved history, read exchanges or calculate over projected observations. Scope is enforced by the host. Search hits are not counts. Only count/compare accept field and equals filters; fields selects returned row facts for read. Arguments for other operations are rejected." +
	" Observation coverage and aggregate facts describe the saved collection. page.returned describes delivered rows; page.next_offset is null only when saved rows are exhausted. read offset is a row index for observations or byte offset for text; next_text_offset continues text. Omit read limit to fit available space. Search/window limit is at most 100. Counts deduplicate identity; comparisons require the same schema and selection. Missing facts remain unknown."

func ReadSchema() map[string]any {
	properties := map[string]any{
		"query":  map[string]any{"type": "string"},
		"ref":    map[string]any{"type": "string", "minLength": 1},
		"other":  map[string]any{"type": "string", "minLength": 1},
		"offset": map[string]any{"type": "integer", "minimum": 0},
		"limit":  map[string]any{"type": "integer", "minimum": 0},
		"fields": observation.FieldsSchema(),
		"field":  map[string]any{"type": "string", "minLength": 1},
		"equals": map[string]any{"type": "object", "properties": map[string]any{
			"text": map[string]any{"type": "string"}, "number": map[string]any{"type": "integer"}, "boolean": map[string]any{"type": "boolean"},
		}, "additionalProperties": false, "minProperties": 1, "maxProperties": 1},
	}
	var variants []observation.OperationVariant
	for _, d := range readDefinitions {
		branch := map[string]any{}
		if d.operation == "search" || d.operation == "window" {
			branch["properties"] = map[string]any{"limit": map[string]any{"maximum": MaxPageResults, "default": DefaultPageResults}}
		}
		if len(d.required) > 0 {
			branch["required"] = d.required
		}
		if slices.Contains(d.arguments, "field") {
			branch["dependentRequired"] = map[string]any{"field": []string{"equals"}, "equals": []string{"field"}}
		}
		variants = append(variants, observation.OperationVariant{Operation: d.operation, Arguments: d.arguments, Schema: branch})
	}
	return observation.OperationSchema(properties, variants)
}
