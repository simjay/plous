package code

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"path"
	"strings"
)

func CodeEntryKind(kind string) string {
	return map[string]string{"helm": "Helm Charts", "kustomize": "Kustomize", "kustomize+helm": "Kustomize + Helm", "plain": "Kubernetes YAML", "terraform": "Terraform"}[kind]
}

func CodeListLabel(name, detail string, width int) string {
	if detail == "" || width < 32 {
		return ui.Clipped(name, max(1, width))
	}
	left := max(1, width-ansi.StringWidth(detail)-2)
	name = ui.Clipped(name, left)
	return name + strings.Repeat(" ", max(2, width-ansi.StringWidth(name)-ansi.StringWidth(detail))) + detail
}

func (s *CodeState) Actions() []textview.ActionGroup {
	if s.Inventory == nil {
		if !s.LoadAttempted {
			return nil
		}
		return []textview.ActionGroup{{Actions: []textview.Action{{Key: "g", Label: "Retry"}, {Key: "a", Label: "Actions"}}}}
	}
	if IsBrowser(s.View) {
		return s.CatalogActions()
	}
	actions := []textview.Action{{Key: "Enter", Label: "Editor"}, {Key: "/", Label: "Filter"}, {Key: "a", Label: "Actions"}}
	if s.View == "files" || s.View == "code" {
		if row, ok := s.SelectedFileRow(); ok && row.Directory {
			actions[0].Label = "Toggle Folder"
			if s.Filter != "" {
				actions[0].Label = "Open Folder"
			}
		}
	} else if s.View == "manifests" {
		actions = []textview.Action{{Key: "a", Label: "Actions"}}
		switch s.Preference.Section {
		case 0:
			if e := s.Inventory.Entry(s.Preference.Selected); e != nil && s.SourceIndex(e.ID) >= 0 {
				label := "Render Selected"
				if e.Component {
					label = "Show Relationships"
				} else if !e.Renderable() {
					label = "Open Files"
				}
				actions = append([]textview.Action{{Key: "Enter", Label: label, Primary: e.Renderable()}, {Key: "/", Label: "Find Node"}}, actions...)
			}
		case 3:
			actions = append([]textview.Action{{Key: "Enter", Label: "Inspect Finding"}, {Key: "/", Label: "Filter"}, {Label: "Validate All", Mouse: "code-validate-all/"}}, actions...)
			if len(s.Findings()) == 0 {
				actions = []textview.Action{{Label: "Validate All", Mouse: "code-validate-all/"}, {Key: "a", Label: "Actions"}}
			}
		case 2:
			if s.FieldSourceAvailable() {
				actions = append([]textview.Action{{Key: "d", Label: "Open Source"}}, actions...)
			}
		default:
			primary := textview.Action{Key: "Enter", Label: "Field History", Primary: true}
			if s.InspectorArea == 1 {
				primary.Label = "Inspect YAML"
			}
			if s.SelectedUnsupported() {
				primary.Label = "Open Files"
			}
			if s.ComparisonActive() {
				primary = textview.Action{Key: "Enter", Label: "Open YAML", Primary: true}
				if s.InspectorArea == 1 {
					primary.Label = "Inspect Changes"
				}
			}
			rebuild := textview.Action{Key: "r", Label: "Render Again"}
			if s.PreviewFailure(s.Preference.Selected) != "" {
				rebuild.Label = "Retry"
			}
			if e := s.Inventory.Entry(s.Preference.Selected); e == nil {
				primary.Label = "Configuration Map"
				actions = append([]textview.Action{primary}, actions...)
			} else if e.Component {
				primary.Label = "Configuration Map"
				actions = append([]textview.Action{primary}, actions...)
			} else if !e.Renderable() {
				actions = append([]textview.Action{primary}, actions...)
			} else if _, ok := s.CurrentResource(); !ok {
				actions = append([]textview.Action{rebuild}, actions...)
			} else {
				actions = append([]textview.Action{primary, rebuild}, actions...)
			}
		}
		if s.Preference.Section != 0 || len(s.Details) > 0 {
			actions = append([]textview.Action{{Key: "Esc", Label: s.DetailBackLabel()}}, actions...)
		}
	}
	return []textview.ActionGroup{{Actions: actions}}
}

type Presentation struct {
	ConnectionName  string
	Expand          textview.Action
	DocumentActions []textview.ActionGroup
	Loading         func(string) string
	FindText        func(string, int, int, int) string
}

func (s *CodeState) Body(width, height int, chrome Presentation) string {
	if s.Discard {
		return ui.TitleStyle.Render("Unsaved File") + "\n\n" + ui.ChoiceList([]string{"Keep Editing", "Discard Changes"}, s.DiscardChoice, width, height-2, ui.SettingsChoiceLine)
	}
	if s.Review != nil && s.Menu == "" {
		s.Review.ViewActions = []textview.ActionGroup{{Label: "View", Actions: []textview.Action{chrome.Expand, {Key: ":h", Label: "Help"}}}}
		if s.File != nil && s.Document != nil {
			s.Review.CanConfirm = s.Document.Dirty() && !s.Review.Stale
		}
		return s.Review.View(width, height, ui.WorkspaceTheme())
	}
	if s.Document != nil && s.Menu == "" {
		s.Document.Actions = chrome.DocumentActions
		return s.Document.View(width, height, ui.WorkspaceTheme())
	}
	title := chrome.ConnectionName + " / " + s.ViewLabel()
	if s.View == "manifests" && width < 76 {
		title = "Render · " + chrome.ConnectionName
	}
	if (s.View == "code" || s.View == "files") && s.FileRoot != "" {
		title += " / " + s.FileRoot
	}
	identity := CodeIdentity(s)
	if width < 44 {
		identity, _, _ = strings.Cut(identity, "\n")
	}
	header := ui.Heading(ui.Clipped(title, width)) + "\n" + ui.Muted.Render(ansi.Wrap(identity, width, ""))
	if s.View == "code-kustomize" && s.Inventory != nil {
		if item, ok := s.SelectedCatalogItem(); ok && s.BrowserDetail {
			title += " / " + s.KustomizeName(item) + " / Overview"
		}
		header = ui.Heading(ui.Clipped(title, width))
	}
	if s.View == "manifests" && s.Inventory != nil {
		header = ui.Heading(ui.Clipped(title, width)) + "\n" + s.Breadcrumb(width)
	}
	if s.Menu == "" && s.Input == nil && (!s.Busy || s.Running == "preview" || s.BrowsingDuringDiscovery()) {
		actions := s.Actions()
		if s.View == "manifests" && width < 76 && len(actions) > 0 && len(actions[0].Actions) > 0 {
			primary := actions[0].Actions[0]
			if s.Preference.Section == 3 {
				primary = textview.Action{Label: "Validate All", Mouse: "code-validate-all/"}
			}
			compact := []textview.Action{primary}
			if s.Preference.Section == 1 && s.PreviewFailure(s.Preference.Selected) != "" {
				compact = append(compact, textview.Action{Key: "r", Label: "Retry"})
			}
			if primary.Key != "a" {
				compact = append(compact, textview.Action{Key: "a", Label: "Actions"})
			}
			actions = []textview.ActionGroup{{Actions: compact}}
		}
		actionRows := 2
		if height < 14 {
			actionRows = 1
		}
		header += "\n" + textview.ActionBar(actions, width, actionRows, ui.WorkspaceTheme())
	}
	if s.Menu == "" && s.Input == nil && s.Inventory != nil {
		if s.View == "manifests" {
			if controls := s.RenderControls(width); controls != "" {
				header += "\n" + controls
			}
		} else if s.View == "files" || s.View == "code" {
			header += "\n" + s.FileControls(width)
		}
	}
	if s.Busy && !(s.Running == "preview" && s.View == "manifests" && s.Preference.Section == 0) {
		label := map[string]string{"load": "Loading Source", "preview": "Rendering Preview", "render": "Rendering Entry", "check": "Validating Entries", "trace-all": "Tracing All Overlays", "compare": "Comparing Entries", "compare-revision": "Comparing Revisions"}[s.Running]
		if (s.Running == "preview" || s.Running == "render") && s.Inventory != nil {
			if entry := s.Inventory.Entry(s.RunningEntry); entry != nil {
				label += " · " + entry.Path
			}
		}
		header += "\n" + ui.Muted.Render(ui.Clipped(chrome.Loading(label), width))
	}
	if s.Err != "" {
		header += "\n" + ui.RemovedStyle.Render(ui.Clipped("Error: "+ui.OneLine(s.Err), width)) + "\n" + ui.MouseAction(ui.Muted.Render("a Actions → Diagnostics"), "a")
	}
	if s.Notice != "" && !strings.HasPrefix(s.Notice, "Rendered ") && s.Menu == "" && !(s.SavedComparison && s.Notice == "Saved changes rendered.") {
		header += "\n" + ui.Muted.Render(ui.Clipped(s.Notice, width))
	}
	if s.Filter != "" {
		header += "\n" + ui.MouseAction(ui.Muted.Render(ui.Clipped("/ Filter: "+s.Filter, width)), "/") + "\n" + ui.Muted.Render("Backspace / Esc Clear filter")
	}
	space := max(1, height-lipgloss.Height(header)-1)
	var body string
	switch {
	case s.Input != nil:
		help := s.InputHelp()
		if s.InputKind == "Export Rendered YAML" && (width < 76 || space < 12) {
			help = "All final resources, masked.\nNew file only.\nEnter Export · Esc Cancel"
		}
		body = ui.TitleStyle.Render(ui.Clipped(s.InputKind, width)) + "\n" + ui.Muted.Render(ansi.Wrap(help, width, "")) + "\n\n" + ui.EditorLine(*s.Input, max(1, width-2), true, "input")
	case s.Menu != "":
		menuTitle := s.MenuTitle()
		if s.Menu == "more-tools" {
			menuTitle += "\nCompare, export, or check other entries."
		}
		if s.Menu == "stages" {
			menuTitle = "Captured Stages · scoped to each source"
		}
		if s.Menu == "stages" {
			if b := s.Builds[s.Preference.Selected]; b != nil && b.Observation != nil && b.Observation.Truncated {
				menuTitle += "\nPartial capture: limit reached or a snapshot was unavailable. Final output is complete."
			}
		}
		items := s.MenuItems()
		body = ui.TitleStyle.Render(ansi.Wrap(menuTitle, width, "")) + "\n" + ui.ChoiceList(items, s.Choice, width, max(1, space-strings.Count(ansi.Wrap(menuTitle, width, ""), "\n")-1), ui.SettingsChoiceLine)
		if len(items) == 0 {
			message := "No options available. Esc returns to the output."
			if s.Menu == "compare" {
				message = "No other renderable entries. Esc returns to the output."
			}
			body += ui.Muted.Render(ansi.Wrap(message, width, ""))
		}
	case s.Inventory == nil:
		if s.Busy || !s.LoadAttempted {
			body = ui.Muted.Render(ansi.Wrap("Reading source files and detecting configuration.", width, ""))
		} else {
			title, detail := "Source Unavailable", "Retry loading or edit the source connection in Settings."
			if s.Err == "" {
				title, detail = "Loading Cancelled", "Retry when you are ready to open this source."
			}
			body = ui.EmptyState(title, detail, "Enter Retry", space)
		}
	case IsBrowser(s.View):
		body = s.CatalogView(width, space)
		if s.View == "code-kustomize" && s.BrowserDetail && chrome.FindText != nil {
			if item, ok := s.SelectedCatalogItem(); ok {
				body = chrome.FindText(s.KustomizeOverview(item, width), width, space, s.DetailOffset)
			}
		}
	case s.View == "files" || s.View == "code":
		body = s.FilesView(width, space)
	case s.View == "tfcode":
		intro := ui.Muted.Render(ansi.Wrap("Configuration directories · Enter browses files. Terraform execution is not available.", width, ""))
		rows := []string{}
		for n, directory := range s.TerraformDirectories() {
			count := 0
			for _, file := range s.Inventory.Source.Paths() {
				if path.Dir(file) == directory && (strings.HasSuffix(file, ".tf") || strings.HasSuffix(file, ".tf.json")) {
					count++
				}
			}
			label := CodeListLabel("▸ "+directory+"/", CodeCountLabel(count, "file", "files"), width-2)
			rows = append(rows, ui.MouseTarget(ui.SettingsChoiceLine(label, n == s.Cursor, width), fmt.Sprintf("code-row/%d", n)))
		}
		body = intro + "\n\n" + ui.ScrollSelection(rows, s.Cursor, max(1, space-lipgloss.Height(intro)-2))
		if len(rows) == 0 {
			body += "\nNo configuration directories match this filter."
		}
	case s.Preference.Section == 0:
		body = s.SourcesView(width, space)
	case s.Preference.Section == 3:
		body = s.ValidationBody(width, space)
	case s.Preference.Selected == "" || s.Inventory.Entry(s.Preference.Selected) == nil:
		body = ui.EmptyState("No Entry Selected", "Select a node in the configuration map. Files is available for all source content.", "Enter Configuration Map", space)
	default:
		body = s.RenderInspector(width, space)
	}

	return textview.Fit(header+"\n\n"+body, width, height)
}
