package app

import (
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/ui"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/source"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
	"path"
	"slices"
	"strings"
)

func (m *model) validateAllCode() {
	s := m.coder()
	if s.Inventory == nil || s.Busy {
		return
	}
	s.OpenFindings(true)
	s.Pending = "check"
}

func (m *model) executeCode(command string) bool {
	if !m.codeVisible() || m.form != nil || m.settingsOverlay != nil {
		return false
	}
	s := m.coder()
	switch command {
	case "refresh", "render", "actions", "review", "revision", "yaml", "describe", "graph":
	default:
		return false
	}
	if s.Document != nil || s.Review != nil {
		m.status = "Close the document to use source actions."
		return true
	}
	if s.Busy && !(s.Running == "preview" && command == "actions") && !(s.BrowsingDuringDiscovery() && (command == "actions" || command == "describe")) {
		m.status = "A source operation is running. Esc cancels it."
		return true
	}
	switch command {
	case "graph":
		m.openCodeGraph()
	case "refresh":
		s.Pending = "load"
	case "render":
		m.requestCodeRender()
	case "actions":
		s.Menu = "actions"
		s.Choice = 0
	case "review":
		if m.tool != "manifests" {
			s.Err = "Open an editable file for review, or render a configuration to compare output."
			return true
		}
		if s.SelectedUnsupported() {
			s.Err = "Rendered comparisons are not available for this entry. Open its configuration in Files."
			return true
		}
		s.Menu = "compare"
		s.Choice = 0
	case "revision":
		if s.Config.Repository != "" {
			s.Prompt("Git Revision", s.Preference.Revision)
		} else {
			s.Err = "Revision selection is available for remote repository snapshots."
		}
	case "yaml":
		if m.tool == "manifests" {
			m.openCodeResource()
		} else {
			m.status = "Render a configuration to inspect its output."
		}
	case "describe":
		if appcode.IsBrowser(s.View) {
			m.openCatalogFile()
		} else if m.tool == "files" || m.tool == "code" {
			m.openCodeFile(false)
		} else if m.tool == "tfcode" {
			m.openCodeTerraformFiles()
		} else if s.View == "manifests" && s.Preference.Section == 2 {
			m.openCodeFieldSource()
		} else if s.Inventory != nil && s.Preference.Section == 0 {
			if e := s.Inventory.Entry(s.Preference.Selected); e != nil {
				s.Preference.File = e.File
				m.openCodeFile(s.Config.Repository == "")
			}
		} else if s.Inventory != nil && s.Preference.Section == 3 {
			findings := s.Findings()
			if s.Cursor >= 0 && s.Cursor < len(findings) && s.Inventory.Source.Has(findings[s.Cursor].File) {
				s.Preference.File = findings[s.Cursor].File
				m.openCodeFile(s.Config.Repository == "")
			}
		} else if s.Inventory != nil {
			m.openCodeResourceSource()
		}
	}
	return true
}

func (m *model) codeCount() int {
	s := m.coder()
	if s.KustomizeDirectory() {
		return len(s.KustomizeGroupItems())
	}
	if appcode.IsBrowser(s.View) {
		return len(s.CatalogItems())
	}
	if s.Inventory == nil {
		return 0
	}
	if m.tool == "files" || m.tool == "code" {
		return len(s.FileRows())
	}
	if m.tool == "tfcode" {
		return len(s.TerraformDirectories())
	}
	if s.Preference.Section == 0 {
		return len(s.Entries())
	}
	if s.Preference.Section == 3 {
		return len(s.Findings())
	}
	return len(s.Resources())
}

func (m *model) updateCode(key string) {
	s := m.coder()
	if s.Busy && !s.BrowsingDuringDiscovery() && (s.Running != "preview" || key == "esc" && s.Filter == "" && s.Menu == "") {
		if key == "esc" {
			s.Stop()
			if s.Running == "preview" && s.Inventory != nil {
				s.MarkPreviewAttempt(s.Preference.Selected)
				s.Failures[s.Preference.Selected] = "Rendering cancelled. Use :r to retry."
			}
			if s.ReturnView == "code-kustomize" && s.Running == "render" {
				m.backCodeBrowser()
				return
			}
			s.Notice = "Cancelled; previous results retained."
			if s.Inventory == nil {
				s.Notice = "Loading Cancelled."
			}
		}
		return
	}
	if s.Menu != "" {
		if key == "esc" {
			if s.Menu == "more-tools" || s.Menu == "comparison-options" {
				label := "More Tools"
				if s.Menu == "comparison-options" {
					label = "Comparison Options"
				}
				s.Menu = "actions"
				s.Choice = slices.Index(s.MenuItems(), label)
			} else {
				s.Menu = ""
			}
		} else if key == "enter" {
			m.activateCodeMenu()
		} else {
			s.Choice = ui.MoveCursor(s.Choice, len(s.MenuItems()), key)
		}
		return
	}
	if s.Inventory == nil && key == "enter" {
		s.Pending = "load"
		return
	}
	if m.codeBrowserKey(key) {
		return
	}
	if m.codeInspectorKey(key) {
		return
	}
	switch key {
	case "g", "G", "r", "a", "v", "F", "y", "d":
		m.executeCode(canonicalCommand(key))
		return
	case "/":
		s.Prompt("Filter", s.Filter)
		return
	case "backspace":
		s.Filter = ""
		s.RestoreCursor()
		s.RestoreTraceReference()
		return
	case "[", "]", "left", "right":
		if (m.tool == "files" || m.tool == "code") && (key == "left" || key == "right") {
			s.FileTreeKey(key)
			m.rememberCode(s)
			return
		}
		return
	case "esc":
		if s.Filter != "" {
			s.Filter = ""
			s.RestoreCursor()
			s.RestoreTraceReference()
			return
		}
		if s.View == "manifests" && s.ReturnView != "" && len(s.Details) == 0 {
			m.backCodeBrowser()
			return
		}
		if m.closeCodeDetail() {
			m.rememberCode(s)
			return
		}
		m.focusRootSidebar()
		return
	case "enter":
		if s.Inventory == nil {
			return
		}
		if m.tool == "files" || m.tool == "code" {
			if row, ok := s.SelectedFileRow(); ok {
				if row.Directory {
					s.FileTreeKey("enter")
					m.rememberCode(s)
				} else {
					m.openCodeFile(s.Config.Repository == "")
				}
			}
			return
		}
		if m.tool == "tfcode" {
			m.openCodeTerraformFiles()
			return
		}
		switch s.Preference.Section {
		case 0:
			entries := s.Entries()
			if len(entries) > 0 {
				s.Preference.Selected = entries[min(s.Cursor, len(entries)-1)].ID
				if entries[min(s.Cursor, len(entries)-1)].Kind == "terraform" {
					m.openCodeDirectory(entries[min(s.Cursor, len(entries)-1)].Path)
					return
				} else if entries[min(s.Cursor, len(entries)-1)].Component {
					s.ShowDocument("Source Relationships", s.Inventory.Description(s.Preference.Selected))
				} else {
					m.renderSelectedCode()
				}
			}
		case 1:
			if s.SelectedUnsupported() {
				m.openCodeDirectory(s.Inventory.Entry(s.Preference.Selected).Path)
				return
			}
			m.openCodeResource()
		case 2:
			if s.SelectedUnsupported() {
				m.openCodeDirectory(s.Inventory.Entry(s.Preference.Selected).Path)
				return
			}
			m.openCodeTrace()
		case 3:
			findings := s.Findings()
			if len(findings) > 0 {
				f := findings[min(s.Cursor, len(findings)-1)]
				if f.Target != "" {
					s.Preference.Selected = f.Target
				}
				s.ShowDocument(f.Rule, f.Message+"\n\nSource: "+f.File+"\nTarget: "+f.Target)
				s.DocumentKind = "finding"
				if s.Inventory.Source.Has(f.File) {
					s.DocumentPath = f.File
				}
			}
		}
	default:
		s.Cursor = ui.MoveCursor(s.Cursor, m.codeCount(), key)
		if s.Preference.Section == 2 && s.InspectorArea == 1 {
			s.TraceReference, s.TraceUnavailable = nil, ""
		}
		s.DetailOffset = 0
	}
	s.RememberSelection()
	m.rememberCode(s)
}

func (m *model) activateCodeMenu() {
	s := m.coder()
	items := s.MenuItems()
	if len(items) == 0 {
		return
	}
	label := items[min(s.Choice, len(items)-1)]
	menu := s.Menu
	s.Menu = ""
	if menu == "render-target" {
		if s.Choice >= 0 && s.Choice < len(s.RenderTargets) {
			m.openCodeRenderTarget(s.RenderTargets[s.Choice])
		}
		return
	}
	if menu == "stages" {
		if s.Choice == 0 {
			s.SelectFinalResource()
		} else {
			s.Stage = s.Choice
			s.Cursor = 0
			s.FieldResource = ""
		}
		s.InspectorArea = 2
		return
	}
	if menu == "validation-source" {
		s.ValidationSource = ""
		entries := s.NavigationEntries()
		if s.Choice > 0 && s.Choice <= len(entries) {
			s.ValidationSource = entries[s.Choice-1].ID
		}
		s.Cursor, s.DetailOffset = 0, 0
		return
	}
	if menu == "validation-category" {
		s.ValidationCategory = label
		s.Cursor, s.DetailOffset = 0, 0
		return
	}
	if menu == "compare" {
		s.Other = label
		s.Pending = "compare"
		return
	}
	in := s.Inputs[s.Preference.Selected]
	if s.Inventory == nil && label != "Refresh Source" && label != "Choose Git Revision" && label != "Diagnostics" && label != "Close Preview" {
		s.Err = "Load a source first."
		return
	}
	switch label {
	case "Open group":
		m.codeBrowserKey("enter")
	case "Overview":
		if item, ok := s.SelectedCatalogItem(); ok {
			s.OpenKustomizeOverview(item.EntryID)
		}
	case "Node overview":
		found := false
		for n := len(s.Details) - 1; n >= 0; n-- {
			previous := s.Details[n]
			if previous.View == "code-kustomize" && previous.BrowserDetail && previous.Selected == s.Preference.Selected {
				s.Details = s.Details[:n+1]
				m.closeCodeDetail()
				found = true
				break
			}
		}
		if !found {
			m.openCodeNodeOverview(s.Preference.Selected)
		}
	case "View output":
		m.codeBrowserControl("saved-output")
	case "Configuration details":
		if item, ok := s.SelectedCatalogItem(); ok {
			s.ShowDocument("Configuration details", s.CatalogDetail(item, 90))
		}
	case "Choose Group":
		s.ShowKustomizeGroups()
	case "Render Base Only":
		if s.KustomizeBaseOnly() {
			item, _ := s.SelectedCatalogItem()
			if entry := s.Inventory.Entry(item.EntryID); entry != nil && entry.Renderable() {
				m.openCodeRenderTarget(*entry)
			}
		}
	case "More Tools":
		s.Menu, s.Choice = "more-tools", 0
	case "Comparison Options":
		s.Menu, s.Choice = "comparison-options", 0
	case "Back to Browser":
		m.backCodeBrowser()
	case "Open in Files", "Open source":
		m.openCatalogFile()
	case "Render":
		m.requestCodeRender()
	case "Graph":
		m.openCodeGraph()
	case "Output":
		m.codeBrowserControl("output")
	case "Configuration Map":
		m.openCodeGraph()
	case "Entry Details":
		if !m.openCodeNodeOverview(s.Preference.Selected) {
			s.OpenDetail(0)
		}
	case "View Findings", "Repository Findings":
		s.OpenFindings(label == "Repository Findings")
	case "Field History":
		m.openCodeTrace()
	case "Open Resource Source":
		m.openCodeResourceSource()
	case "Findings", "Images", "Generators":
		s.SelectValidationView(label)
	case "Trace Across All Overlays":
		m.startTraceAcrossOverlays()
	case "Export Rendered YAML":
		b := s.Builds[s.Preference.Selected]
		if b == nil || s.Status(s.Preference.Selected) == "stale" || s.Status(s.Preference.Selected) == "failed" {
			s.Err = "Render the selected source successfully before exporting."
			break
		}
		s.ExportResult = b
		s.Err, s.Notice = "", ""
		s.Prompt(label, "")
	case "Filter Source":
		s.Menu, s.Choice = "validation-source", 0
	case "Filter Category":
		s.Menu, s.Choice = "validation-category", 0
	case "Captured Stages":
		s.Menu, s.Choice = "stages", s.Stage
	case "Trace Finding Field":
		m.codeFindingDestination("trace")
	case "Reference Graph":
		m.openCodeGraph()
	case "Finding Entry Details":
		m.codeFindingDestination("sources")
	case "Open Finding in Render":
		m.codeFindingDestination("render")
	case "Side by Side Comparison", "Unified Comparison":
		s.ComparisonUnified = label == "Unified Comparison"
	case "Show Final YAML", "Show Changes":
		s.RememberSelection()
		s.ComparisonHidden = label == "Show Final YAML"
		s.RestoreCursor()
	case "Swap Comparison":
		s.ComparisonSwapped = !s.ComparisonSwapped
		s.ComparisonOffset = 0
	case "Changed Only", "All Lines":
		s.ComparisonChanged = label == "Changed Only"
		s.ComparisonOffset = 0
		s.RestoreCursor()
	case "Clear Comparison":
		s.Comparison, s.ComparisonBase = nil, nil
		s.SavedComparison = false
		s.RestoreCursor()
	case "Open Comparison Reviewer":
		if s.Comparison != nil && s.ComparisonBase != nil {
			before, after := s.Comparison, s.ComparisonBase
			if s.ComparisonSwapped {
				before, after = after, before
			}
			var err error
			s.Review, err = appcode.CodeComparison(before, after)
			if err != nil {
				s.Err = err.Error()
			}
		}
	case "Refresh Source":
		s.Pending = "load"
	case "Render Selected":
		m.renderSelectedCode()
	case "Validate All Entry Points":
		m.validateAllCode()
	case "Show References":
		s.ShowDocument("Source Relationships", s.Inventory.Description(s.Preference.Selected))
	case "Open Source File", "Open Selected Source File":
		if s.Document != nil && s.DocumentPath != "" {
			m.openCodeDocumentSource()
			break
		} else if e := s.Inventory.Entry(s.Preference.Selected); e != nil {
			s.Preference.File = e.File
		}
		m.openCodeFile(false)
	case "Open Selected File":
		m.openCodeFile(false)
	case "Close Preview":
		s.CloseDocument()
	case "Open Configuration Files":
		m.openCodeTerraformFiles()
	case "Edit Selected File":
		m.openCodeFile(true)
	case "Helm Inputs":
		s.Menu = "helm-inputs"
		s.Choice = 0
	case "Compare With…", "Compare Entry Points":
		s.Menu = "compare"
		s.Choice = 0
	case "Helm Release Name":
		s.Prompt(label, in.Release)
	case "Helm Namespace":
		s.Prompt(label, in.Namespace)
	case "Helm Values Files (In Order)":
		s.Prompt(label, strings.Join(in.Values, ","))
	case "Helm Overrides":
		s.Prompt(label, strings.Join(in.Overrides, ","))
	case "Helm Kubernetes Version":
		s.Prompt(label, in.KubeVersion)
	case "Helm API Versions":
		s.Prompt(label, strings.Join(in.APIVersions, ","))
	case "Choose Git Revision":
		m.executeCode("revision")
	case "Compare Git Revision":
		if s.Config.Repository != "" {
			s.Prompt(label, "")
		} else {
			s.Err = "Select a remote repository for revision comparison."
		}
	case "Create Editable Local Copy":
		s.Prompt(label, "")
	case "Technical details":
		m.activateCodeDiagnostics()
	case "Diagnostics":
		detail := s.Err
		if failure := s.PreviewFailure(s.Preference.Selected); failure != "" && !strings.Contains(detail, failure) {
			detail += "\n" + failure
		}
		s.ShowDocument("Source Diagnostics", strings.TrimSpace(detail+"\n"+s.Notice))
	}
	if s.Input != nil {
		s.InputMenu, s.InputChoice = menu, s.Choice
	}
}

func (m *model) submitCodeInput() {
	s := m.coder()
	kind, value := s.InputKind, strings.TrimSpace(s.Input.Text)
	s.Input = nil
	in := s.Inputs[s.Preference.Selected]
	switch kind {
	case "Export Rendered YAML":
		name, err := source.Export(s.ExportResult, value)
		if err != nil {
			s.Err = err.Error()
			e := textview.NewField(value)
			s.Input = &e
		} else {
			s.Err, s.ExportResult = "", nil
			s.Notice = "Exported masked final YAML to " + name
		}
		return
	case "Filter":
		s.Filter = value
		s.Cursor = 0
		s.RestoreCursor()
		if value != "" && (s.View == "files" || s.View == "code") {
			for n, row := range s.FileRows() {
				if !row.Directory {
					s.Cursor = n
					break
				}
			}
		}
	case "Git Revision":
		s.Other, s.Pending = value, "revision"
	case "Compare Git Revision":
		s.Other = value
		s.Pending = "compare-revision"
	case "Helm Release Name":
		in.Release = value
	case "Helm Namespace":
		in.Namespace = value
	case "Helm Values Files (In Order)":
		in.Values = appcode.ListInput(value)
	case "Helm Overrides":
		in.Overrides = nil
		if value != "" {
			in.Overrides = []string{value}
		}
	case "Helm Kubernetes Version":
		in.KubeVersion = value
	case "Helm API Versions":
		in.APIVersions = appcode.ListInput(value)
	case "Create Editable Local Copy":
		if s.Inventory == nil {
			return
		}
		directory, err := source.ResolveDirectory(value, "")
		if err == nil {
			err = s.Inventory.Source.Copy(directory)
		}
		if err != nil {
			s.Err = err.Error()
			e := textview.NewField(value)
			s.Input = &e
			return
		}
		w := *m.current()
		w.Connections = append(append([]workspace.Connection(nil), w.Connections...), workspace.Connection{Tool: "code", Name: path.Base(directory), Settings: map[string]string{"source_type": "Local directory", "path": directory}})
		if m.saveWorkspace(w) {
			s.Notice = "Created editable local copy: " + directory
		}
		return
	}
	if strings.HasPrefix(kind, "Helm ") {
		s.Menu, s.Choice = s.InputMenu, s.InputChoice
		s.Notice = "Rendering inputs updated. Preview updates when you return to the list."
	}
	s.Inputs[s.Preference.Selected] = in
	s.RememberSelection()
	m.rememberCode(s)
}

func (m *model) openCodeResource() {
	s := m.coder()
	b := s.Builds[s.Preference.Selected]
	resources := s.Resources()
	if b == nil {
		s.Err = "Render an entry point first."
		return
	}
	if len(resources) == 0 {
		m.status = "No resource is selected. Clear the filter or choose another entry."
		return
	}
	resource := resources[min(s.Cursor, len(resources)-1)]
	if changes := s.ComparisonChanges(); s.Cursor >= 0 && s.Cursor < len(changes) && changes[s.Cursor].After.Name == "" {
		b = s.Comparison
	}
	s.RememberSelection()
	s.ShowDocument(resource.Kind+" / "+resource.Name, manifest.YAML(resource.Object, true))
	s.Document.Syntax = "yaml"
	s.Document.Target = b.Path + " · " + s.Status(b.ID) + " · " + b.Engine
	s.DocumentKind = "resource"
	if b.CapturedSource() != nil {
		s.DocumentSource = b.CapturedSource()
	}
	if s.Inventory != nil && b.SourceIdentity == s.Inventory.Source.Identity {
		if origin := b.Origins[resource.Key()]; s.Inventory.Source.Has(origin) {
			s.DocumentPath = origin
		}
	}
}

func (m *model) openCodeTrace() {
	s := m.coder()
	if _, ok := s.CurrentResource(); !ok {
		m.status = "Select a rendered resource to trace."
		return
	}
	s.RememberSelection()
	s.OpenDetail(2)
	s.InspectorArea = 3
	m.rememberCode(s)
}

func (m *model) openCodeResourceSource() {
	s := m.coder()
	name := s.ResourceOrigin()
	if name == "" {
		m.status = "No verified resource source is available. Use Entry Details to inspect the inputs."
		return
	}
	s.Preference.File = name
	m.openCodeFile(s.Config.Repository == "")
}

func (m *model) openCodeFile(edit bool) {
	s := m.coder()
	if s.Inventory == nil {
		return
	}
	if (m.tool == "files" || m.tool == "code") && s.Document == nil {
		row, ok := s.SelectedFileRow()
		if !ok || row.Directory {
			m.status = "No file is selected. Clear the filter or refresh the source."
			return
		}
		s.Preference.File = row.Path
	}
	name := s.Preference.File
	if s.Document != nil && s.DocumentPath != "" {
		name = s.DocumentPath
	}
	if name == "" {
		if e := s.Inventory.Entry(s.Preference.Selected); e != nil {
			name = e.File
		}
	}
	if name == "" {
		s.Err = "Select a file first."
		return
	}
	if s.Document != nil && s.Document.Dirty() {
		s.Err = "Close or save the current draft before opening another file."
		return
	}
	if edit {
		if view := s.OtherEditorView(); view != "" {
			m.status = "Return to " + strings.ToUpper(view[:1]) + view[1:] + " to finish the open editor before editing another file."
			return
		}
		file, err := s.Inventory.Source.Edit(name)
		if err != nil {
			s.Err = err.Error()
			if s.Document != nil {
				s.Document.Diagnostic = err.Error()
			}
			return
		}
		s.File = file
		s.Document = docedit.New(string(file.Baseline))
		s.DocumentSource = s.Inventory
		s.Document.Name = name
		s.Document.Target = "Local source file · unsaved content stays in memory"
		s.Document.Notice = "Editing original file bytes. Save explicitly with :w."
		s.Review = nil
	} else {
		s.ShowDocument(name, appcode.CodeFilePreview(s, name))
	}
	s.DocumentKind, s.DocumentPath = "file", name
	if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
		s.Document.Syntax = "yaml"
	}
	s.Preference.File = name
	m.rememberCode(s)
}

func (m *model) openCodeTerraformFiles() {
	s := m.coder()
	directories := s.TerraformDirectories()
	if len(directories) == 0 {
		return
	}
	directory := directories[min(s.Cursor, len(directories)-1)]
	m.openCodeDirectory(directory)
}
