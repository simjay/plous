// Package source acquires source metadata and reads selected inputs.
// Rendering captures its dependencies separately from configuration discovery.
package source

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	catalog "github.com/simjay/plous/internal/integrations/catalog"
	helmcode "github.com/simjay/plous/internal/integrations/helm/code"
	kubecode "github.com/simjay/plous/internal/integrations/kubernetes/code"
	kustomize "github.com/simjay/plous/internal/integrations/kustomize/code"
	kustomizecommon "github.com/simjay/plous/internal/integrations/kustomize/common"
)

type Entry struct {
	ID, Kind, Path, File string
	Component            bool
	References           []kustomizecommon.Reference
}

func (e Entry) Renderable() bool {
	return !e.Component && (e.Kind == "plain" || e.Kind == "helm" || e.Kind == "kustomize" || e.Kind == "kustomize+helm")
}

type Finding struct {
	Severity, Rule, Target, File, Message string
	Resource, Field, Summary              string
}
type Inventory struct {
	generators    []Finding
	work          *sourceWork
	discoveryErr  error
	Discovering   bool
	Source        *Snapshot
	Kustomize     *kustomize.Inventory
	Entries       []Entry
	Findings      []Finding
	Terraform     []string
	Catalog       []catalog.Item
	DirectoryTags map[string]string
	charts        map[string]HelmChart
}

func Browse(s *Snapshot) *Inventory {
	return &Inventory{Source: s, Kustomize: &kustomize.Inventory{}, Discovering: true}
}
func Discover(s *Snapshot) *Inventory {
	inventory, _ := DiscoverContext(context.Background(), s)
	return inventory
}
func DiscoverContext(ctx context.Context, s *Snapshot) (*Inventory, error) {
	if err := ctx.Err(); err != nil {
		return Browse(s), err
	}
	inventory := discover(ctx, s)
	if err := ctx.Err(); err != nil {
		inventory.Discovering = true
		return inventory, err
	}
	if inventory.discoveryErr != nil {
		inventory.Discovering = true
		return inventory, inventory.discoveryErr
	}
	inventory.work = nil
	return inventory, nil
}
func discover(ctx context.Context, s *Snapshot) *Inventory {
	i := &Inventory{Source: s, charts: map[string]HelmChart{}, work: newSourceWork(ctx)}
	for _, name := range s.Paths() {
		if err := i.work.retain(128 + len(name)); err != nil {
			i.Kustomize = &kustomize.Inventory{}
			i.discoveryErr = err
			return i
		}
	}
	i.Kustomize, i.discoveryErr = kustomize.Inspect(s.Paths(), func(name string) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := s.ReadFile(name)
		i.work.bytes += len(data)
		return data, err
	}, i.work.retain)
	if i.discoveryErr != nil {
		return i
	}
	charts := []string{}
	tf := map[string]bool{}
	for _, file := range s.Paths() {
		if ctx.Err() != nil || i.discoveryErr != nil {
			return i
		}
		if path.Base(file) == "Chart.yaml" {
			charts = append(charts, path.Dir(file))
		}
		if strings.HasSuffix(file, ".tf") || strings.HasSuffix(file, ".tf.json") {
			tf[path.Dir(file)] = true
		}
	}
	for dir := range tf {
		if !i.keep(dir) {
			return i
		}
		i.Terraform = append(i.Terraform, dir)
	}
	slices.Sort(i.Terraform)
	for _, e := range i.Kustomize.Entries {
		kind := "kustomize"
		if charts, ok := e.Configuration["helmCharts"].([]any); ok && len(charts) > 0 {
			kind = "kustomize+helm"
		}
		entry := Entry{ID: "kustomize:" + e.Path, Kind: kind, Path: e.Path, File: e.File, References: e.References, Component: e.Configuration["kind"] == "Component"}
		if !i.keep(entry) {
			return i
		}
		i.Entries = append(i.Entries, entry)
		if e.Error != "" {
			i.add("error", "parse", entry.ID, e.File, e.Error)
		}
		for _, message := range kustomize.DeprecatedFields(i.discoveryFile(e.File)) {
			i.add("warning", "deprecated-kustomization-field", entry.ID, e.File, message)
		}
		for _, ref := range e.References {
			if ref.Error != "" {
				i.add("error", "reference", entry.ID, e.File, ref.Path+": "+ref.Error)
			}
		}
	}
	for _, dir := range charts {
		file := path.Join(dir, "Chart.yaml")
		id := "helm:" + dir
		entry := Entry{ID: id, Kind: "helm", Path: dir, File: file}
		if !i.keep(entry) {
			return i
		}
		i.Entries = append(i.Entries, entry)
		chart := i.readHelmChart(entry)
		if !i.keep(chart) {
			return i
		}
		i.charts[id] = chart
		if err := helmcode.ValidateMetadata(i.discoveryFile(file)); err != nil {
			i.add("error", "parse", id, file, "Invalid chart metadata")
		}
	}
	for _, file := range s.Paths() {
		if ctx.Err() != nil || i.discoveryErr != nil {
			return i
		}
		if !strings.HasSuffix(file, ".yaml") && !strings.HasSuffix(file, ".yml") && !strings.HasSuffix(file, ".json") {
			continue
		}
		inChart := false
		for _, dir := range charts {
			if dir == "." || strings.HasPrefix(file, dir+"/") {
				inChart = true
				break
			}
		}
		if inChart {
			continue
		}
		if slices.ContainsFunc(i.Kustomize.Entries, func(e kustomizecommon.Entry) bool { return e.File == file }) {
			continue
		}
		excluded := false
		for _, entry := range i.Kustomize.Entries {
			for _, ref := range entry.References {
				if ref.Path == file && ref.Error == "" {
					excluded = true
				}
			}
		}
		if excluded {
			continue
		}
		detected, err := kubecode.Detect(i.discoveryFile(file))
		if !detected {
			continue
		}
		if err != nil {
			i.add("error", "parse", "plain:"+file, file, err.Error())
		}
		entry := Entry{ID: "plain:" + file, Kind: "plain", Path: file, File: file}
		if !i.keep(entry) {
			return i
		}
		i.Entries = append(i.Entries, entry)
	}
	for _, gap := range s.Gaps {
		i.add("warning", "coverage", "", "", gap)
	}
	// Detect cycles independently of rendering so failures do not hide graph facts.
	done, visiting := map[string]bool{}, map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		if visiting[name] {
			i.add("error", "cycle", "kustomize:"+name, "", "Dependency cycle at "+name)
			return
		}
		if done[name] {
			return
		}
		visiting[name] = true
		if e := i.Kustomize.Entry(name); e != nil {
			for _, r := range e.References {
				if r.Entry {
					visit(r.Path)
				}
			}
		}
		visiting[name] = false
		done[name] = true
	}
	for _, e := range i.Kustomize.Entries {
		visit(e.Path)
	}
	i.discoverCatalog(ctx)
	if ctx.Err() != nil || i.discoveryErr != nil {
		return i
	}
	i.inspectStructure()
	i.inspectGenerators()
	slices.SortFunc(i.Entries, func(a, b Entry) int { return strings.Compare(a.ID, b.ID) })
	return i
}
func (i *Inventory) discoveryFile(name string) []byte {
	if i.discoveryErr != nil {
		return nil
	}
	if !i.Source.Has(name) {
		return nil
	}
	data, err := i.Source.ReadFile(name)
	if i.work != nil {
		i.work.bytes += len(data)
	}
	if err != nil {
		if !slices.ContainsFunc(i.Findings, func(f Finding) bool { return f.Rule == "unavailable-file" && f.File == name }) {
			i.add("warning", "unavailable-file", "", name, "Cannot inspect file: "+DisplayText(err.Error()))
		}
	}
	return data
}
func (i *Inventory) add(severity, rule, target, file, message string) {
	finding := Finding{Severity: severity, Rule: rule, Target: target, File: file, Message: message}
	if i.keep(finding) {
		i.Findings = append(i.Findings, finding)
	}
}
func (i *Inventory) Entry(id string) *Entry {
	for n := range i.Entries {
		if i.Entries[n].ID == id {
			return &i.Entries[n]
		}
	}
	if strings.HasPrefix(id, "terraform:") {
		for _, entry := range i.SourceEntries() {
			if entry.ID == id {
				return &entry
			}
		}
	}
	return nil
}

// Unsupported detected formats remain browsable alongside renderable entries.
func (i *Inventory) SourceEntries() []Entry {
	entries := slices.Clone(i.Entries)
	for _, directory := range i.Terraform {
		entry := Entry{ID: "terraform:" + directory, Kind: "terraform", Path: directory}
		for _, file := range i.Source.Paths() {
			if path.Dir(file) == directory && (strings.HasSuffix(file, ".tf") || strings.HasSuffix(file, ".tf.json")) {
				entry.File = file
				break
			}
		}
		entries = append(entries, entry)
	}
	return entries
}
func (i *Inventory) Consumers(id string) []string {
	e := i.Entry(id)
	if e == nil && strings.HasPrefix(id, "plain:") {
		file := strings.TrimPrefix(id, "plain:")
		if i.Source.Has(file) {
			e = &Entry{Path: file, File: file}
		}
	}
	if e == nil {
		return nil
	}
	var result []string
	for _, candidate := range i.Entries {
		for _, r := range candidate.References {
			if r.Path == e.Path || r.Path == e.File {
				result = append(result, candidate.ID)
				break
			}
		}
	}
	return result
}
func (i *Inventory) Affected(id string) []string {
	seen := map[string]bool{}
	var result []string
	var walk func(string)
	walk = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		for _, next := range i.Consumers(id) {
			if !seen[next] {
				result = append(result, next)
				walk(next)
			}
		}
	}
	walk(id)
	slices.Sort(result)
	return result
}
func (i *Inventory) Description(id string) string {
	e := i.Entry(id)
	if e == nil {
		return "Select an entry point."
	}
	if e.Kind == "terraform" {
		return e.Path + "\nTerraform configuration detected. Render, Trace, and validation are not available. Open its configuration in Files."
	}
	lines := []string{e.Path + " · " + e.Kind, "", "Includes (consumer → dependency):"}
	for _, r := range e.References {
		row := r.Relation + " → " + r.Path
		if r.Error != "" {
			row += " · " + r.Error
		}
		lines = append(lines, row)
	}
	if len(e.References) == 0 {
		lines = append(lines, "No declared repository references.")
	}
	if k := i.Kustomize.Entry(e.Path); k != nil && strings.HasPrefix(e.ID, "kustomize:") {
		lines = append(lines, "", "Changes here:", k.Summary())
	}
	consumers := i.Consumers(id)
	lines = append(lines, "", "Used by: "+strings.Join(consumers, ", "))
	if e.Component {
		lines = append(lines, "Component: build through a consuming kustomization.")
	} else if len(consumers) == 0 {
		lines = append(lines, "Top-level candidate in this loaded scope.")
	}
	lines = append(lines, fmt.Sprintf("Affected consumers: %d", len(i.Affected(id))))
	return strings.Join(lines, "\n")
}

func (i *Inventory) keep(record any) bool {
	if i.discoveryErr != nil {
		return false
	}
	if i.work != nil {
		if err := i.work.record(record); err != nil {
			i.discoveryErr = err
			return false
		}
	}
	return true
}
