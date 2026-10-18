package common

import (
	"fmt"
	"regexp"
	"strings"
)

// ChangeDescriptions reports declarations without exposing patch or generator values.
func (e Entry) ChangeDescriptions() []string {
	var lines []string
	for _, setting := range [][2]string{{"namespace", "Sets namespace to "}, {"namePrefix", "Prefixes names with "}, {"nameSuffix", "Suffixes names with "}} {
		if value, ok := e.Configuration[setting[0]].(string); ok && value != "" {
			lines = append(lines, setting[1]+value)
		}
	}
	for _, setting := range [][2]string{{"patches", "patch"}, {"patchesStrategicMerge", "strategic merge patch"}, {"patchesJson6902", "JSON patch"}, {"images", "image override"}, {"replicas", "replica override"}, {"replacements", "replacement"}, {"configMapGenerator", "ConfigMap generator"}, {"secretGenerator", "Secret generator"}} {
		if values, ok := e.Configuration[setting[0]].([]any); ok && len(values) > 0 {
			label := setting[1]
			if len(values) > 1 {
				label += "es"
				if !strings.HasSuffix(setting[1], "patch") {
					label = setting[1] + "s"
				}
			}
			lines = append(lines, fmt.Sprintf("%d %s", len(values), label))
		}
	}
	for _, key := range []string{"labels", "commonLabels", "commonAnnotations"} {
		if value, ok := e.Configuration[key]; ok && value != nil {
			lines = append(lines, "Adds "+map[string]string{"labels": "labels", "commonLabels": "common labels", "commonAnnotations": "annotations"}[key])
		}
	}
	return lines
}

var patchIdentity = regexp.MustCompile(`(?:patch "|for Id )([A-Za-z][A-Za-z0-9]*)\.[^/\s"]+/([^\s";]+)\.\[`)

// RenderFailure explains known engine failures without guessing a fix or location.
func RenderFailure(message string) string {
	if strings.Contains(message, "no matches for Id") || strings.Contains(message, "no resource matches strategic merge patch") {
		if match := patchIdentity.FindStringSubmatch(message); len(match) == 3 {
			return "A patch targets " + match[1] + " " + match[2] + ", but no matching resource was found."
		}
		return "A patch could not find its target resource. Check the patch's kind and name against the included resources."
	}
	if strings.Contains(message, "failed to find unique target") {
		return "A patch could not identify a unique target resource. Check its kind, name, and namespace."
	}
	if strings.Contains(message, "yaml:") || strings.Contains(message, "MalformedYAMLError") {
		return "A source file contains invalid YAML. Open the technical details for the parser's location and explanation."
	}
	return ""
}
