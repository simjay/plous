package source

import (
	"fmt"
	"io"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// PreviewLine maps a source node to its redacted, reformatted preview. It never
// copies raw source values into the viewer or assumes unchanged line offsets.
func PreviewLine(source, preview string, originalLine int) int {
	locations := func(text string) map[string]int {
		out := map[string]int{}
		decoder := yaml.NewDecoder(strings.NewReader(text))
		for doc := 0; ; doc++ {
			var node yaml.Node
			if err := decoder.Decode(&node); err == io.EOF {
				break
			} else if err != nil {
				return nil
			}
			if len(node.Content) > 0 {
				nodeLines(node.Content[0], fmt.Sprintf("/%d", doc), out)
			}
		}
		return out
	}
	from, to := locations(source), locations(preview)
	key := ""
	for pointer, line := range from {
		if line == originalLine && to[pointer] > 0 && (key == "" || len(pointer) < len(key) || len(pointer) == len(key) && pointer < key) {
			key = pointer
		}
	}
	return to[key]
}
