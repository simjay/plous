package code

import (
	kustomize "github.com/simjay/plous/internal/integrations/kustomize/code"
	"github.com/simjay/plous/internal/source"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"path"
	"slices"
	"strings"
)

func (s *CodeState) Prompt(kind, value string) {
	e := textview.NewField(value)
	s.Input = &e
	s.InputKind = kind
	s.InputMenu, s.InputChoice = s.Menu, s.Choice
	s.Menu = ""
}

func (s *CodeState) ShowDocument(name, text string) {
	s.Document = docedit.New(text)
	s.DocumentSource = s.Inventory
	s.Document.ReadOnly = true
	s.Document.Name = name
	s.Document.Target = "Captured source · sensitive values masked"
	s.File = nil
	s.Review = nil
	s.DocumentKind, s.DocumentPath, s.DocumentAction = "", "", 0
}

func (s *CodeState) Files() []string {
	if s.Inventory == nil {
		return nil
	}
	var files []string
	for _, file := range s.Inventory.Source.Paths() {
		if ui.SearchContains(file, s.Filter) {
			files = append(files, file)
		}
	}
	return files
}

func (s *CodeState) Entries() []source.Entry {
	if s.Inventory == nil {
		return nil
	}
	var entries []source.Entry
	chain := s.RenderChain()
	for _, e := range s.Inventory.SourceEntries() {
		if s.RenderRoot != "" && !chain[e.ID] {
			continue
		}
		if ui.SearchContains(e.Path+" "+e.Kind, s.Filter) {
			entries = append(entries, e)
		}
	}
	slices.SortFunc(entries, func(a, b source.Entry) int {
		if a.Kind != b.Kind {
			return strings.Compare(a.Kind, b.Kind)
		}
		return strings.Compare(a.Path, b.Path)
	})
	return entries
}

func (s *CodeState) AllFindings() []source.Finding {
	return s.AssistantReadView().Findings()
}

func (s *CodeState) Findings() []source.Finding {
	findings := s.AllFindings()
	if s.ValidationView == "Images" {
		findings = nil
		for _, entry := range s.Inventory.Entries {
			if b := s.Builds[entry.ID]; b != nil && s.Status(entry.ID) != "stale" && s.Status(entry.ID) != "failed" {
				findings = append(findings, source.Images(b)...)
			}
		}
	} else if s.ValidationView == "Generators" {
		findings = s.Inventory.Generators()
	}
	var filtered []source.Finding
	for _, finding := range findings {
		if (s.ValidationSource == "" || finding.Target == s.ValidationSource) && (s.ValidationView != "" && s.ValidationView != "Findings" || s.ValidationCategory == "" || s.ValidationCategory == "All" || source.FindingCategory(finding) == s.ValidationCategory) && ui.SearchContains(finding.Severity+" "+finding.Rule+" "+finding.Target+" "+finding.File+" "+finding.Message, s.Filter) {
			filtered = append(filtered, finding)
		}
	}
	return filtered
}

func (s *CodeState) MenuTitle() string {
	return map[string]string{"actions": "Actions", "more-tools": "More Tools", "comparison-options": "Comparison Options", "entry": "Entry Actions", "compare": "Compare With", "document": "File Actions", "helm-inputs": "Helm Rendering Inputs", "validation-category": "Finding Category", "validation-source": "Filter Source", "render-target": "What Do You Want to Render?", "stages": "Captured Stages"}[s.Menu]
}

func (s *CodeState) MenuItems() []string {
	if s.Menu == "stages" {
		return s.StageItems()
	}
	if s.Menu == "validation-source" {
		return s.ValidationSourceItems()
	}
	if s.Menu == "validation-category" {
		return []string{"All", "Build", "Patches", "Images", "Generators", "Structure"}
	}
	switch s.Menu {
	case "render-target":
		var items []string
		for _, e := range s.RenderTargets {
			label := "Kustomize"
			if e.Kind == "helm" {
				label = "Helm chart only"
			} else if e.Kind == "kustomize+helm" {
				label = "Kustomize including chart"
			}
			items = append(items, label+" · "+e.Path)
		}
		return items
	case "document":
		items := []string{}
		if s.DocumentKind == "file" && s.Config.Repository == "" {
			items = append(items, "Edit Selected File")
		} else if s.DocumentPath != "" && s.DocumentKind != "file" {
			items = append(items, "Open Source File")
		}
		return append(items, "Close Preview")
	case "compare":
		items := []string{}
		if s.Inventory == nil {
			return items
		}
		for _, e := range s.ComparisonEntries() {
			items = append(items, e.ID)
		}
		return items
	case "helm-inputs":
		return CodeHelmInputs
	case "entry":
		items := []string{}
		if e := s.Inventory.Entry(s.Preference.Selected); e != nil {
			if e.Renderable() {
				items = append(items, "Render Selected")
			}
			items = append(items, "Show References", "Open Source File")
			if e.Kind == "helm" {
				items = append(items, "Helm Inputs")
			}
			if e.Renderable() && len(s.ComparisonEntries()) > 0 {
				items = append(items, "Compare With…")
			}
		}
		return items
	default:
		items := []string{"Refresh Source"}
		if s.Inventory != nil {
			if s.View == "code-kustomize" {
				if s.KustomizeDirectory() {
					return []string{"Open group", "Refresh Source"}
				}
				items = []string{"Overview", "Refresh Source"}
				if s.BrowserDetail {
					items = []string{"Graph", "Open source", "Configuration details", "Refresh Source"}
					if len(s.RenderCandidates()) > 0 {
						items = append(items, "Render")
					}
					if s.KustomizeBaseOnly() {
						items = append(items, "Render Base Only")
					}
					if item, ok := s.SelectedCatalogItem(); ok && s.Builds[item.EntryID] != nil {
						items = append(items, "View output")
					}
				} else if s.NodeGraph == "" {
					items = append(items, "Choose Group")
				}
				if s.PreviewFailure(s.DiagnosticEntry()) != "" {
					items = append(items, "Technical details")
				}
				return items
			}
			if IsBrowser(s.View) {
				items = append([]string{"Open in Files"}, items...)
				if len(s.RenderCandidates()) > 0 {
					items = append([]string{"Render"}, items...)
				}
			} else if s.View == "files" || s.View == "code" {
				if row, ok := s.SelectedFileRow(); ok && !row.Directory {
					items = append(items, "Open Selected File")
					if s.Config.Repository == "" {
						items = append(items, "Edit Selected File")
					}
				}
			} else if s.View == "manifests" {
				if s.ReturnView == "code-kustomize" {
					items = append([]string{"Node overview", "View Findings", "Repository Findings"}, items...)
					if s.PreviewFailure(s.Preference.Selected) != "" {
						items = append(items, "Technical details")
					}
				} else {
					items = append([]string{"Back to Browser", "Graph", "Output", "View Findings", "Repository Findings"}, items...)
				}
				if s.ResourceOrigin() != "" && s.Preference.Section == 1 {
					items = append(items, "Open Resource Source")
				}
				if _, ok := s.CurrentField(); ok && s.Preference.Section == 1 && !s.ComparisonActive() {
					items = append(items, "Field History")
				}
				if s.Preference.Section == 0 {
					items = append(items, "Reference Graph")
				}
				if s.Preference.Section == 3 {
					items = append(items, "Findings", "Images", "Generators", "Filter Category", "Finding Entry Details", "Open Finding in Render")
					findings := s.Findings()
					if s.Cursor >= 0 && s.Cursor < len(findings) && findings[s.Cursor].Field != "" {
						items = append(items, "Trace Finding Field")
					}
					items = append(items, "Filter Source")
				}
				if s.Preference.Section == 1 && len(s.StageItems()) > 1 {
					items = append(items, "Captured Stages")
				}
				if s.ComparisonActive() {
					items = append(items, "Show Final YAML", "Side by Side Comparison", "Unified Comparison", "Open Comparison Reviewer", "Clear Comparison", "Swap Comparison", "Changed Only", "All Lines")
				} else if s.Preference.Section == 1 && s.Comparison != nil && s.ComparisonScope == s.Preference.Selected {
					items = append(items, "Show Changes")
				}
				if e := s.Inventory.Entry(s.Preference.Selected); e != nil {
					if e.Renderable() {
						items = append(items, "Render Selected")
						if len(s.ComparisonEntries()) > 0 {
							items = append(items, "Compare Entry Points")
						}
						if s.Config.Repository != "" {
							items = append(items, "Compare Git Revision")
						}
					}
					items = append(items, "Show References", "Open Selected Source File")
					if e.Kind == "helm" {
						items = append(items, "Helm Inputs")
					}
				}
				if s.Preference.Section == 1 && s.Builds[s.Preference.Selected] != nil {
					items = append(items, "Export Rendered YAML")
				}
				if s.Preference.Section == 2 && strings.HasPrefix(s.Preference.Selected, "kustomize:") {
					items = append(items, "Trace Across All Overlays")
				}
				if len(s.Inventory.Entries) > 0 {
					items = append(items, "Validate All Entry Points")
				}
			} else if s.View == "tfcode" && len(s.TerraformDirectories()) > 0 {
				items = append(items, "Open Configuration Files")
			}
			if s.Config.Repository != "" {
				items = append(items, "Create Editable Local Copy")
			}
		}
		if s.Config.Repository != "" {
			items = append(items, "Choose Git Revision")
		}
		if s.Err != "" || s.Notice != "" && !strings.HasPrefix(s.Notice, "Rendered ") && s.Notice != "Saved changes rendered." || s.ReturnView != "code-kustomize" && s.PreviewFailure(s.Preference.Selected) != "" {
			items = append(items, "Diagnostics")
		}
		if s.View == "manifests" && s.Inventory != nil {
			return s.GroupActions(items)
		}
		return items
	}
}

func (s *CodeState) ComparisonEntries() []source.Entry {
	var entries []source.Entry
	if s.Inventory != nil {
		for _, e := range s.Inventory.Entries {
			if e.Renderable() && e.ID != s.Preference.Selected {
				entries = append(entries, e)
			}
		}
	}
	return entries
}

func (s *CodeState) GroupActions(items []string) []string {
	groups := map[string]string{
		"Side by Side Comparison": "comparison-options", "Unified Comparison": "comparison-options",
		"Open Comparison Reviewer": "comparison-options", "Clear Comparison": "comparison-options",
		"Swap Comparison": "comparison-options", "Changed Only": "comparison-options", "All Lines": "comparison-options",
		"Refresh Source": "more-tools", "Render Selected": "more-tools", "Compare Entry Points": "more-tools",
		"Compare Git Revision": "more-tools", "Captured Stages": "more-tools", "Export Rendered YAML": "more-tools",
		"Repository Findings": "more-tools", "Validate All Entry Points": "more-tools", "Show References": "more-tools",
		"Open Selected Source File": "more-tools", "Choose Git Revision": "more-tools", "Create Editable Local Copy": "more-tools",
	}
	var result []string
	for _, item := range items {
		group := groups[item]
		if s.Menu == "more-tools" || s.Menu == "comparison-options" {
			if group == s.Menu {
				result = append(result, item)
			}
		} else if group == "" {
			// These destinations already have their own contextual actions.
			if item == "Configuration Map" && s.Preference.Section == 0 || item == "View Findings" && s.Preference.Section == 3 {
				continue
			}
			result = append(result, item)
		}
	}
	if s.Menu != "more-tools" && s.Menu != "comparison-options" {
		if s.ComparisonActive() {
			result = append(result, "Comparison Options")
		}
		result = append(result, "More Tools")
	}
	return result
}

var CodeHelmInputs = []string{"Helm Release Name", "Helm Namespace", "Helm Values Files (In Order)", "Helm Overrides", "Helm Kubernetes Version", "Helm API Versions"}

func ListInput(value string) []string {
	var values []string
	for _, v := range strings.Split(value, ",") {
		if v = strings.TrimSpace(v); v != "" {
			values = append(values, v)
		}
	}
	return values
}

func CodeFilePreview(s *CodeState, name string) string {
	data, err := s.Inventory.Source.ReadFile(name)
	if err != nil {
		return "File unavailable in the captured snapshot."
	}
	ext := strings.ToLower(path.Ext(name))
	if ext == ".yaml" || ext == ".yml" || ext == ".json" || path.Base(name) == "Kustomization" {
		return kustomize.Document(name, data)
	}
	if strings.HasPrefix(path.Base(name), ".env") || strings.HasSuffix(name, ".tfstate") || strings.HasSuffix(name, ".tfvars") {
		return "Sensitive configuration. Use Edit Selected File to inspect the original local bytes. Remote snapshots cannot be edited."
	}
	return manifest.SafeText(string(data))
}
