package source

import (
	"fmt"
	"io"
	"slices"
	"strings"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
	yaml "go.yaml.in/yaml/v3"
)

type Field struct {
	Pointer, Value, File, Evidence string
	Line                           int
	RenderedLine                   int
	Writes                         []FieldWrite
	ObservedComplete               bool
}

func Trace(i *Inventory, r *Result, resource manifest.Resource) []Field {
	if r.captured != nil {
		i = r.captured
	}
	var fields []Field
	file := r.Origins[resource.Key()]
	current := r.captured != nil || r.SourceIdentity == i.Source.Identity
	if !current {
		file = ""
	}
	lines := map[string]int{}
	renderedLines := map[string]int{}
	var rendered yaml.Node
	if yaml.Unmarshal([]byte(manifest.YAML(resource.Object, true)), &rendered) == nil && len(rendered.Content) > 0 {
		nodeLines(rendered.Content[0], "", renderedLines)
	}
	exact := strings.HasPrefix(r.ID, "plain:") && current
	if exact {
		decoder := yaml.NewDecoder(strings.NewReader(string(i.Source.Files[file])))
		for {
			var doc yaml.Node
			if err := decoder.Decode(&doc); err == io.EOF {
				break
			} else if err != nil {
				break
			}
			if len(doc.Content) == 0 {
				continue
			}
			node := doc.Content[0]
			var object map[string]any
			if node.Decode(&object) != nil {
				continue
			}
			if manifest.JSON(object) != manifest.JSON(resource.Object) {
				continue
			}
			nodeLines(node, "", lines)
			break
		}
	}
	var walk func(any, string)
	walk = func(value any, pointer string) {
		switch v := value.(type) {
		case map[string]any:
			keys := []string{}
			for key := range v {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			for _, key := range keys {
				walk(v[key], pointer+"/"+escapePointer(key))
			}
		case []any:
			for n, child := range v {
				walk(child, fmt.Sprintf("%s/%d", pointer, n))
			}
		default:
			evidence := "Partial: resource origin only; field writers unavailable"
			if file == "" {
				evidence = "Field attribution unavailable"
				if strings.HasPrefix(r.ID, "helm:") && resource.Source != "" {
					evidence = "Helm template " + resource.Source + "; source file not captured; field writers unavailable"
				}
			}
			if exact && lines[pointer] > 0 {
				evidence = "Exact source field"
			}
			if !current {
				evidence = "Source snapshot changed; render again to inspect field origin"
			}
			writes, complete := observedWrites(i, r, resource, pointer)
			if len(writes) > 0 {
				evidence = "Observed field changes"
				if !complete {
					evidence = "Partial field history"
				}
			}
			fields = append(fields, Field{Pointer: pointer, Value: fmt.Sprint(value), File: file, Line: lines[pointer], Evidence: evidence, RenderedLine: renderedLines[pointer], Writes: writes, ObservedComplete: complete})
		}
	}
	// Trace displays the same redacted values as Render, including generic secrets.
	walk(manifest.Redact(resource.Object), "")
	slices.SortStableFunc(fields, func(a, b Field) int { return a.RenderedLine - b.RenderedLine })
	return fields
}
func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}
func nodeLines(n *yaml.Node, p string, out map[string]int) {
	out[p] = n.Line
	switch n.Kind {
	case yaml.MappingNode:
		for k := 0; k+1 < len(n.Content); k += 2 {
			nodeLines(n.Content[k+1], p+"/"+escapePointer(n.Content[k].Value), out)
		}
	case yaml.SequenceNode:
		for k, child := range n.Content {
			nodeLines(child, fmt.Sprintf("%s/%d", p, k), out)
		}
	}
}
