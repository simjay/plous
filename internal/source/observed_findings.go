package source

import (
	"fmt"
	"strings"
)

func (r *Result) inspectOverwrites(i *Inventory) {
	if r.Observation == nil || r.Observation.Truncated {
		return
	}
	for _, resource := range r.Resources {
		for _, field := range Trace(i, r, resource) {
			if !field.ObservedComplete {
				continue
			}
			for n, write := range field.Writes {
				if !strings.HasPrefix(write.Kind, "Patch") || n == len(field.Writes)-1 {
					continue
				}
				last := field.Writes[len(field.Writes)-1]
				if write.After == last.After {
					continue
				}
				r.Findings = append(r.Findings, Finding{Severity: "info", Rule: "overwritten-field", Target: r.ID, File: write.File, Resource: resource.Key(), Field: field.Pointer, Summary: "Patched field overwritten", Message: fmt.Sprintf("%s/%s %s: patch sets %s; %s later sets %s. This finding concerns this field, not the entire patch.", resource.Kind, resource.Name, field.Pointer, write.After, TransformationLabel(last.Kind), last.After)})
				break
			}
		}
	}
}
