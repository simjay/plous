package source

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/integrations/observation"
)

// ReadView freezes analysis selections and acquired Git revisions at send time.
// Local file tools read current saved bytes within the captured source root.
type ReadView struct {
	inventory *Inventory
	builds    map[string]*Result
	results   map[string]*Result
	inputs    map[string]string
	failures  map[string]string
}

func CaptureReadView(i *Inventory, builds map[string]*Result, inputs map[string]Inputs, failures map[string]string, comparisons ...*Result) *ReadView {
	if i == nil || i.Source == nil {
		return nil
	}
	v := &ReadView{inventory: i, builds: maps.Clone(builds), results: map[string]*Result{}, inputs: map[string]string{}, failures: maps.Clone(failures)}
	for id, in := range inputs {
		v.inputs[id] = in.Identity()
	}
	for id, result := range builds {
		v.inputs[id] = inputs[id].Identity()
		if result != nil {
			v.results[result.OperationID] = result
		}
	}
	for _, result := range comparisons {
		if result != nil {
			v.results[result.OperationID] = result
		}
	}
	return v
}

func (v *ReadView) status(e Entry) string {
	if !e.Renderable() {
		return "not independently renderable"
	}
	return ResultState(v.inventory.Source.Identity, v.inputs[e.ID], v.builds[e.ID], v.failures[e.ID])
}

func ResultState(sourceIdentity, inputIdentity string, result *Result, failure string) string {
	if failure != "" {
		return "failed"
	}
	if result == nil {
		return "not rendered"
	}
	if result.SourceIdentity != sourceIdentity || result.InputIdentity != inputIdentity {
		return "stale"
	}
	return "rendered"
}

func (v *ReadView) Findings() []Finding {
	if v == nil {
		return nil
	}
	findings := append([]Finding(nil), v.inventory.Findings...)
	for _, directory := range v.inventory.Terraform {
		entry := v.inventory.Entry("terraform:" + directory)
		findings = append(findings, Finding{Severity: "info", Rule: "unsupported", Target: entry.ID, File: entry.File, Message: directory + ": Terraform validation is not available; configuration files remain browsable."})
	}
	for _, e := range v.inventory.Entries {
		if failure := v.failures[e.ID]; failure != "" {
			summary := "Render failed"
			if strings.Contains(failure, "no matches for Id") || strings.Contains(failure, "no resource matches") {
				summary = "Patch target matches no resource"
			}
			findings = append(findings, Finding{Severity: "error", Rule: "render", Target: e.ID, File: e.File, Message: failure, Summary: summary})
		} else if v.status(e) == "rendered" {
			findings = append(findings, v.builds[e.ID].Findings...)
		}
	}
	severity := map[string]int{"error": 0, "warning": 1, "info": 2}
	slices.SortStableFunc(findings, func(a, b Finding) int { return severity[a.Severity] - severity[b.Severity] })
	return findings
}

func (v *ReadView) result(c ReadRequest) (*Result, error) {
	b := v.results[c.Result]
	if c.Result == "" {
		b = v.builds[c.Target]
	}
	if b == nil || c.Target != "" && b.ID != c.Target {
		return nil, errors.New("render result is not captured; select and render the target in Code before sending a new message")
	}
	return b, nil
}

func readView(ctx context.Context, scope ReadScope, c ReadRequest) (observation.Result, error) {
	r := observation.New("source.analysis.v1", c.Operation, time.Now().UTC())
	r.Connection = scope.ConnectionID
	if err := ctx.Err(); err != nil {
		return r, err
	}
	if err := validateReadScope(scope); err != nil {
		return r, err
	}
	if err := validateReadArguments(c); err != nil {
		return r, err
	}
	if c.Offset < 0 || c.Limit < 0 || c.Limit > MaxReadResults || len(c.Target)+len(c.Result)+len(c.Resource)+len(c.Field)+len(c.Other) > 4096 {
		return r, denySourceRead(errors.New("invalid analysis read arguments"))
	}
	unavailable := func(message string) (observation.Result, error) {
		r.Outcome = "unsupported"
		r.Facts["reason"] = observation.Text(message)
		return r, nil
	}
	v := scope.View
	if v == nil {
		return unavailable("Code snapshot is not captured; open or refresh Code before sending a new message")
	}
	r.ObservedAt = v.inventory.Source.Observed
	if v.inventory.Discovering {
		r.Partial("configuration discovery has not completed")
	}
	r.Coverage.Snapshot = v.inventory.Source.Identity
	r.Selection = map[string]string{"target": c.Target, "result": c.Result, "resource": c.Resource, "field": c.Field, "other": c.Other}
	r.Facts["source_identity"] = observation.Text(v.inventory.Source.Identity)
	r.Facts["revision"] = observation.Text(v.inventory.Source.Revision)
	if (c.Operation == "catalog" || c.Operation == "findings") && c.Target != "" && v.inventory.Entry(c.Target) == nil {
		return unavailable("target is absent from the captured source catalog")
	}
	if slices.ContainsFunc(v.inventory.Findings, func(f Finding) bool { return f.Rule == "unavailable-file" }) {
		r.Partial("configuration discovery could not inspect some files")
	}
	if len(v.inventory.Source.Gaps) > 0 {
		r.Partial("source acquisition omitted content")
	}
	add := func(kind, name string, facts map[string]observation.Value) {
		if len(r.Rows) >= 10000 {
			if r.Facts["row_limit"].Boolean == nil {
				r.Facts["row_limit"] = observation.Boolean(true)
				r.Partial("analysis row limit reached; select a target, result or resource")
			}
			return
		}
		r.Rows = append(r.Rows, observation.Row{Identity: observation.Identity{Kind: kind, Name: name}, Facts: facts})
	}
	switch c.Operation {
	case "catalog":
		for _, e := range v.inventory.SourceEntries() {
			if c.Target != "" && c.Target != e.ID {
				continue
			}
			if !sourcePath(e.File) {
				r.Partial("source read policy excludes entries")
				continue
			}
			f := textFacts("kind", e.Kind, "file", e.File, "path", e.Path, "status", v.status(e))
			f["renderable"] = observation.Boolean(e.Renderable())
			for n, id := range v.inventory.EntryDependencies(e.ID) {
				f[fmt.Sprintf("dependency.%d", n)] = observation.Text(id)
			}
			for n, id := range v.inventory.EntryConsumers(e.ID) {
				f[fmt.Sprintf("consumer.%d", n)] = observation.Text(id)
			}
			if b := v.builds[e.ID]; b != nil {
				f["result"] = observation.Text(b.OperationID)
			}
			add("Entry", e.ID, f)
		}
		for _, item := range v.inventory.Catalog {
			if c.Target != "" && c.Target != item.EntryID {
				continue
			}
			if !sourcePath(item.File) {
				continue
			}
			add("Configuration", item.ID, textFacts("technology", item.Technology, "kind", item.Kind, "name", item.Name, "file", item.File, "target", item.EntryID, "version", item.Version, "location", item.Location))
		}
		for n, generator := range v.inventory.Generators() {
			if c.Target != "" && c.Target != generator.Target || !sourcePath(generator.File) {
				continue
			}
			add("Generator", fmt.Sprint(n), textFacts("target", generator.Target, "file", generator.File, "summary", generator.Summary, "details", generator.Message))
		}
		ids := make([]string, 0, len(v.results))
		for id := range v.results {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			b := v.results[id]
			if c.Target != "" && c.Target != b.ID {
				continue
			}
			add("Render", id, textFacts("target", b.ID, "source_identity", b.SourceIdentity, "revision", b.Revision, "input_identity", b.InputIdentity, "engine", b.Engine, "built_at", b.Built.Format(time.RFC3339Nano)))
		}
	case "findings":
		findings := v.Findings()
		for _, e := range v.inventory.SourceEntries() {
			if c.Target != "" && c.Target != e.ID {
				continue
			}
			state := v.status(e)
			if state != "rendered" {
				r.Partial("target " + e.ID + ": " + state)
				findings = append(findings, Finding{Severity: "info", Rule: "coverage", Target: e.ID, File: e.File, Message: state})
			}
		}
		for n, f := range findings {
			if c.Target != "" && f.Target != "" && f.Target != c.Target {
				continue
			}
			if !sourcePath(f.File) {
				r.Partial("source read policy excludes findings")
				continue
			}
			add("Finding", fmt.Sprint(n), textFacts("severity", f.Severity, "rule", f.Rule, "target", f.Target, "file", f.File, "resource", f.Resource, "field", f.Field, "message", f.Message))
		}
	default:
		b, err := v.result(c)
		if err != nil {
			return unavailable(err.Error())
		}
		r.ObservedAt, r.Coverage.Snapshot = b.Built, b.OperationID
		r.Facts["source_identity"], r.Facts["revision"] = observation.Text(b.SourceIdentity), observation.Text(b.Revision)
		for k, value := range textFacts("result", b.OperationID, "target", b.ID, "input_identity", b.InputIdentity, "engine", b.Engine, "release", b.Inputs.Release, "namespace", b.Inputs.Namespace, "kube_version", b.Inputs.KubeVersion) {
			r.Facts[k] = value
		}
		r.Facts["stale"] = observation.Boolean(b.SourceIdentity != v.inventory.Source.Identity || b.InputIdentity != v.inputs[b.ID])
		r.Facts["content_identity"] = observation.Text(b.ContentIdentity)
		r.Facts["capture_consistency"] = observation.Text("inputs captured during the render operation; local multi-file capture is not an atomic filesystem snapshot")
		r.Facts["freshness"] = observation.Text("captured render; local files have not been rechecked")
		r.Facts["inputs"] = observation.Text("input identity covers values, overrides and API capabilities; sensitive inputs are not returned")
		switch c.Operation {
		case "resources":
			for _, resource := range b.Resources {
				if c.Resource != "" && c.Resource != resource.Key() {
					continue
				}
				add("Resource", resource.Key(), textFacts("api_version", resource.APIVersion, "kind", resource.Kind, "namespace", resource.Namespace, "name", resource.Name, "source", b.Origins[resource.Key()]))
			}
		case "fields":
			found := false
			for _, resource := range b.Resources {
				if resource.Key() != c.Resource {
					continue
				}
				found = true
				values, complete := manifest.Fields(resource.Object)
				if !complete {
					r.Partial("resource field limit reached or oversized values omitted")
				}
				traces := map[string]Field{}
				for _, field := range Trace(v.inventory, b, resource) {
					traces[field.Pointer] = field
				}
				matched := false
				for _, value := range values {
					if c.Field != "" && value.Pointer != c.Field {
						continue
					}
					matched = true
					field, traced := traces[value.Pointer]
					if !traced {
						field.Evidence = "Field attribution unavailable"
					}
					f := textFacts("file", field.File, "evidence", field.Evidence)
					f["value"] = value.Value
					f["line"] = observation.Number(int64(field.Line))
					f["history_complete"] = observation.Boolean(field.ObservedComplete)
					add("Field", value.Pointer, f)
					for n, w := range field.Writes {
						write := textFacts("field", field.Pointer, "kind", w.Kind, "scope", w.Scope, "file", w.File, "before", w.Before, "after", w.After)
						write["line"] = observation.Number(int64(w.Line))
						write["stage"] = observation.Number(int64(w.Stage))
						add("FieldWrite", fmt.Sprintf("%s/write/%d", field.Pointer, n), write)
					}
				}
				if !matched && c.Field != "" {
					return unavailable("field is absent or unavailable in the captured render")
				}
			}
			if !found {
				return unavailable("resource is absent from the captured render")
			}
			r.Coverage.Limitations = append(r.Coverage.Limitations, "field history completeness is reported per field; values describe the captured render")
		case "compare":
			other := v.results[c.Other]
			if other == nil {
				return unavailable("comparison result is not captured")
			}
			r.Facts["before_result"] = observation.Text(other.OperationID)
			r.Facts["before_source_identity"] = observation.Text(other.SourceIdentity)
			r.Facts["before_revision"] = observation.Text(other.Revision)
			for _, change := range Compare(other, b) {
				resource := change.Resource()
				if c.Resource != "" && resource.Key() != c.Resource {
					continue
				}
				add("ResourceChange", resource.Key(), textFacts("status", change.Status, "match", change.Match))
				if change.Status == "Unchanged" {
					continue
				}
				fields, complete := manifest.FieldChanges(change.Before.Object, change.After.Object)
				if !complete {
					r.Partial("comparison field limit reached")
				}
				for _, field := range fields {
					add("FieldChange", resource.Key()+field.Pointer, map[string]observation.Value{"resource": observation.Text(resource.Key()), "field": observation.Text(field.Pointer), "before": field.Before, "after": field.After, "had_before": observation.Boolean(field.HadBefore), "has_after": observation.Boolean(field.HasAfter)})
				}
			}
			r.Coverage.Limitations = append(r.Coverage.Limitations, "field differences compare document positions, not list-element lineage")
		}
	}
	if err := ctx.Err(); err != nil {
		return r, err
	}
	if c.Offset > len(r.Rows) {
		return r, denySourceRead(errors.New("offset exceeds captured rows"))
	}
	if c.Limit == 0 {
		c.Limit = 50
	}
	r.Facts["selected_rows"] = observation.Number(int64(len(r.Rows)))
	end := min(len(r.Rows), c.Offset+c.Limit)
	if end < len(r.Rows) {
		r.Facts["next_offset"] = observation.Number(int64(end))
	}
	if c.Offset != 0 || end != len(r.Rows) {
		r.Partial("selected page of captured results")
	}
	r.Rows = r.Rows[c.Offset:end]
	r.Coverage.Observed = len(r.Rows)
	if r.Outcome == "completed" {
		r.Complete()
	}
	return r, r.Validate()
}

func textFacts(values ...string) map[string]observation.Value {
	f := map[string]observation.Value{}
	for n := 0; n+1 < len(values); n += 2 {
		value := manifest.SafeText(values[n+1])
		if len(value) > 2048 {
			value = value[:utf8Prefix(value, 2048)] + " [excerpt]"
		}
		f[values[n]] = observation.Text(strings.ToValidUTF8(value, ""))
	}
	return f
}

type ReadSelection struct {
	Path           string `json:"path,omitempty"`
	SourceIdentity string `json:"source_identity,omitempty"`
	Revision       string `json:"revision,omitempty"`
	Target         string `json:"target,omitempty"`
	Result         string `json:"result,omitempty"`
	Resource       string `json:"resource,omitempty"`
	Field          string `json:"field,omitempty"`
	Other          string `json:"other,omitempty"`
	Section        string `json:"section,omitempty"`
	Unsaved        bool   `json:"unsaved_changes,omitempty"`
}
