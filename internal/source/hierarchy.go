package source

import (
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/simjay/plous/internal/integrations/manifest"
	"sigs.k8s.io/yaml"
)

// HelmChart describes captured inputs or a declaration, without acquiring a chart.
// Name and Version come from Chart.yaml when captured; declarations remain separate
// because the renderer prefers captured charts even when a different version is declared.
type HelmChart struct {
	Name, Version, Repository, Path, EntryID string
	DeclaredName, DeclaredVersion            string
	DeclaredBy, Status, Error                string
	Inherited                                bool
}

// EntryDependencies contains only captured source nodes that contribute resources.
// Patch, values, and configuration files remain references in the node inspector.
func (i *Inventory) EntryDependencies(id string) []string {
	e := i.Entry(id)
	if e == nil {
		return nil
	}
	var result []string
	for _, ref := range e.References {
		if ref.Entry && ref.Error == "" && (ref.Relation == "resources" || ref.Relation == "bases" || ref.Relation == "components") {
			if dependency := i.Entry("kustomize:" + ref.Path); dependency != nil {
				result = append(result, dependency.ID)
			}
		}
	}
	for _, chart := range i.declaredHelmCharts(id) {
		if chart.EntryID != "" {
			result = append(result, chart.EntryID)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func (i *Inventory) EntryConsumers(id string) []string {
	var result []string
	for _, entry := range i.Entries {
		if slices.Contains(i.EntryDependencies(entry.ID), id) {
			result = append(result, entry.ID)
		}
	}
	slices.Sort(result)
	return result
}

// Roles describe relationships in this snapshot, never directory naming conventions.
func (i *Inventory) EntryRoles(id string) []string {
	e := i.Entry(id)
	if e == nil {
		return nil
	}
	var result []string
	consumers := i.EntryConsumers(id)
	if e.Component {
		result = append(result, "component")
	} else if strings.HasPrefix(id, "kustomize:") {
		overlay := slices.ContainsFunc(i.EntryDependencies(id), func(dependency string) bool {
			entry := i.Entry(dependency)
			return strings.HasPrefix(dependency, "kustomize:") && entry != nil && !entry.Component
		})
		if overlay {
			result = append(result, "overlay")
		}
		if len(consumers) > 0 {
			result = append(result, "base")
		}
	}
	if len(consumers) > 1 {
		result = append(result, "shared")
	}
	if len(result) == 0 {
		result = append(result, "entry")
	}
	return result
}

// EntryHelmCharts includes declarations inherited through Kustomize resources and
// components. A shared dependency is visited once, and cycles do not recurse forever.
func (i *Inventory) EntryHelmCharts(id string) []HelmChart {
	e := i.Entry(id)
	if e == nil {
		return nil
	}
	if e.Kind == "helm" {
		chart := i.capturedHelmChart(*e)
		chart.DeclaredBy = e.ID
		return []HelmChart{chart}
	}
	var result []HelmChart
	seen := map[string]bool{}
	var visit func(string)
	visit = func(current string) {
		if seen[current] {
			return
		}
		seen[current] = true
		for _, chart := range i.declaredHelmCharts(current) {
			chart.Inherited = current != id
			result = append(result, chart)
		}
		for _, dependency := range i.EntryDependencies(current) {
			if strings.HasPrefix(dependency, "kustomize:") {
				visit(dependency)
			}
		}
	}
	visit(id)
	return result
}

func (i *Inventory) declaredHelmCharts(id string) []HelmChart {
	if i.Kustomize == nil || !strings.HasPrefix(id, "kustomize:") {
		return nil
	}
	e := i.Kustomize.Entry(strings.TrimPrefix(id, "kustomize:"))
	if e == nil {
		return nil
	}
	globals, _ := e.Configuration["helmGlobals"].(map[string]any)
	home, _ := globals["chartHome"].(string)
	if home == "" {
		home = "charts"
	}
	declarations, _ := e.Configuration["helmCharts"].([]any)
	var result []HelmChart
	for _, declaration := range declarations {
		values, ok := declaration.(map[string]any)
		chart := HelmChart{DeclaredBy: id}
		name, _ := values["name"].(string)
		version, _ := values["version"].(string)
		repository, _ := values["repo"].(string)
		chart.Name, chart.Version = manifest.SafeText(name), manifest.SafeText(version)
		chart.DeclaredName, chart.DeclaredVersion = chart.Name, chart.Version
		chart.Repository = chartRepositoryLabel(repository)
		switch {
		case !ok || name == "" || path.Base(name) != name || !fs.ValidPath(name) || strings.HasPrefix(name, "-"):
			chart.Status, chart.Error = "invalid", "Chart name must be a simple directory name"
		case !fs.ValidPath(home):
			chart.Status, chart.Error = "out-of-scope", "Chart home must stay inside the kustomization directory"
		default:
			chart.Path = path.Join(e.Path, home, name)
			if local := i.Entry("helm:" + chart.Path); local != nil {
				captured := i.capturedHelmChart(*local)
				chart.Name, chart.Version = captured.Name, captured.Version
				chart.EntryID, chart.Status, chart.Error = captured.EntryID, captured.Status, captured.Error
			} else if repository != "" {
				chart.Status = "remote"
				if version == "" || strings.ContainsAny(version, "*^~<>= ,") {
					chart.Error = "Uncaptured chart requires an exact version and repository before rendering"
				} else if parsed, err := url.Parse(repository); err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http" && parsed.Scheme != "oci") {
					chart.Error = "Repository must be an HTTP(S) or OCI URL without embedded credentials"
				}
			} else {
				chart.Status, chart.Error = "missing", "Chart is unavailable in this source snapshot"
			}
		}
		result = append(result, chart)
	}
	return result
}

func (i *Inventory) capturedHelmChart(e Entry) HelmChart {
	if chart, ok := i.charts[e.ID]; ok {
		return chart
	}
	return i.readHelmChart(e)
}
func (i *Inventory) readHelmChart(e Entry) HelmChart {
	chart := HelmChart{Path: e.Path, EntryID: e.ID, Status: "captured"}
	var metadata struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	// Helm accepts extra chart fields and converts YAML scalar versions to strings.
	if i.Source == nil || yaml.Unmarshal(i.discoveryFile(e.File), &metadata) != nil || metadata.Name == "" || metadata.Version == "" {
		chart.Name = path.Base(e.Path)
		chart.Status, chart.Error = "invalid", "Invalid chart metadata"
		return chart
	}
	chart.Name, chart.Version = manifest.SafeText(metadata.Name), manifest.SafeText(metadata.Version)
	return chart
}

var displayURL = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s<>]+`)

// DisplayText masks credentials in source declarations and their diagnostic text.
// It is for presentation only; captured source paths and bytes remain untouched.
func DisplayText(text string) string {
	text = displayURL.ReplaceAllStringFunc(text, func(location string) string {
		urlText := strings.TrimRight(location, ".,;:)]}`\"'")
		return chartRepositoryLabel(urlText) + location[len(urlText):]
	})
	return manifest.SafeText(text)
}

func chartRepositoryLabel(repository string) string {
	parsed, err := url.Parse(repository)
	if err != nil && strings.Contains(repository, "://") {
		return "[invalid repository URL]"
	}
	if err == nil && parsed.Host != "" {
		parsed.User, parsed.RawQuery, parsed.Fragment = nil, "", ""
		return manifest.SafeText(parsed.String())
	}
	return manifest.SafeText(repository)
}
