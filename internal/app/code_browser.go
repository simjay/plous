package app

import (
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/ui"
	"path"
)

func (m *model) requestCodeRender() {
	s := m.coder()
	if s.View == "files" || s.View == "code" {
		s.Notice = "Select a configuration in Helm, Kustomize, Argo, or Flux, then use :r to render an available target."
		return
	}
	if s.Inventory == nil {
		s.Err = "Load a source first."
		return
	}
	if s.KustomizeDirectory() {
		s.Notice = "Open a group, then select a configuration to render."
		return
	}
	if s.View == "manifests" {
		if s.SelectedUnsupported() {
			s.Err = "Rendering is not available for this entry. Open its configuration in Files."
			return
		}
		m.renderSelectedCode()
		return
	}
	targets := s.RenderCandidates()
	if len(targets) == 0 {
		s.Err = "No renderable Helm chart or Kustomization found for this selection. Open its files to inspect the configuration."
		if s.View == "code-kustomize" {
			s.Err = "Select an overlay to render its composition."
			if s.KustomizeBaseOnly() {
				s.Err += " For this base alone, use Actions → Render Base Only."
			}
		}
		return
	}
	s.Err, s.Notice = "", ""
	s.RenderTargets = targets
	if len(targets) > 1 {
		s.Menu, s.Choice = "render-target", 0
		return
	}
	m.openCodeRenderTarget(targets[0])
}
func (m *model) openCodeRenderTarget(entry source.Entry) {
	s := m.coder()
	if s.View == "code-kustomize" {
		if !s.BrowserDetail {
			s.OpenKustomizeOverview(entry.ID)
		}
		s.PushLocation()
	}
	s.RememberSelection()
	if s.View != "manifests" {
		s.ReturnView = s.View
	}
	s.RenderRoot = entry.ID
	m.tool, s.View = "manifests", "manifests"
	s.Preference.Selected = entry.ID
	s.Menu = ""
	if s.ReturnView != "code-kustomize" {
		s.Details = nil
	}
	s.Preference.Section = 1
	s.SyncListScope()
	s.Filter = ""
	s.RestoreCursor()
	s.InspectorArea, s.DetailOffset = 2, 0
	s.Err, s.Notice = "", ""
	s.Pending = "render"
	m.rememberCode(s)
}
func (m *model) backCodeBrowser() {
	s := m.coder()
	if s.ReturnView == "code-kustomize" && len(s.Details) > 0 {
		if s.Busy {
			s.Stop()
		}
		m.closeCodeDetail()
		return
	}
	view := s.ReturnView
	if !appcode.IsBrowser(view) {
		view = "files"
	}
	s.Menu, s.Details = "", nil
	s.ReturnView, s.RenderRoot = "", ""
	m.tool, s.View = view, view
	s.SyncListScope()
	s.RestoreCursor()
	m.rememberCode(s)
}
func (m *model) openCatalogFile() {
	s := m.coder()
	item, ok := s.SelectedCatalogItem()
	if !ok {
		return
	}
	s.RememberSelection()
	if s.View == "code-kustomize" && s.BrowserDetail {
		s.Preference.File = item.File
		m.openCodeFile(s.Config.Repository == "")
		return
	}
	m.tool, s.View = "files", "files"
	s.BrowserDetail = false
	s.SyncListScope()
	s.Filter, s.FileRoot = "", ""
	s.Preference.File = item.File
	s.RevealFile(item.File)
	s.RestoreCursor()
	m.rememberCode(s)
}
func (m *model) codeBrowserKey(key string) bool {
	s := m.coder()
	if !appcode.IsBrowser(s.View) {
		return false
	}
	switch key {
	case "enter":
		if s.View == "code-kustomize" {
			if s.KustomizeDirectory() {
				groups := s.KustomizeGroupItems()
				if s.Cursor >= 0 && s.Cursor < len(groups) {
					s.OpenKustomizeGroup(groups[s.Cursor].Path)
				}
			} else if !s.BrowserDetail {
				if item, ok := s.SelectedCatalogItem(); ok {
					s.OpenKustomizeOverview(item.EntryID)
				}
			}
			break
		}
		s.BrowserDetail = true
		s.DetailOffset = 0
	case "d":
		m.openCatalogFile()
	case "esc":
		if s.Filter != "" {
			s.Filter = ""
			s.RestoreCursor()
		} else if s.View == "code-kustomize" && len(s.Details) > 0 {
			m.closeCodeDetail()
		} else if s.BrowserDetail {
			s.BrowserDetail = false
			s.DetailOffset = 0
		} else if s.View == "code-kustomize" && !s.KustomizeDirectory() {
			s.ShowKustomizeGroups()
		} else {
			m.focusRootSidebar()
		}
	case "up", "down", "j", "k", "left", "right", "home", "end", "pgup", "pgdown":
		if s.KustomizeDirectory() {
			s.Cursor = ui.MoveCursor(s.Cursor, len(s.KustomizeGroupItems()), key)
			break
		}
		if s.BrowserDetail {
			s.DetailOffset = ui.MoveCursor(s.DetailOffset, 10000, key)
			return true
		}
		s.MoveCatalog(key)
	default:
		return false
	}
	s.RememberSelection()
	m.rememberCode(s)
	return true
}
func (m *model) codeBrowserControl(action string) {
	if !m.codeVisible() {
		return
	}
	s := m.coder()
	fileNavigation := s.BrowsingDuringDiscovery() && (action == "root" || action == "parent")
	if s.Inventory == nil || s.Document != nil || s.Review != nil || s.Input != nil || s.Menu != "" || s.Busy && action != "back" && !fileNavigation {
		return
	}
	switch action {
	case "diagnostics":
		m.activateCodeDiagnostics()
	case "failure-source":
		if e := s.Inventory.Entry(s.DiagnosticEntry()); e != nil {
			s.Preference.File = e.File
			m.openCodeFile(s.Config.Repository == "")
		}
	case "base":
		if s.KustomizeBaseOnly() {
			item, _ := s.SelectedCatalogItem()
			m.openCodeRenderTarget(*s.Inventory.Entry(item.EntryID))
		}
	case "saved-output":
		if item, ok := s.SelectedCatalogItem(); ok && s.Builds[item.EntryID] != nil {
			m.openCodeRenderTarget(*s.Inventory.Entry(item.EntryID))
			s.Pending = ""
		}
	case "groups":
		if s.View == "code-kustomize" {
			s.ShowKustomizeGroups()
		}
	case "render":
		m.requestCodeRender()
	case "back":
		m.backCodeBrowser()
	case "files":
		m.openCatalogFile()
	case "root", "parent":
		if action == "root" {
			s.FileRoot = ""
		} else {
			s.FileRoot = path.Dir(s.FileRoot)
			if s.FileRoot == "." {
				s.FileRoot = ""
			}
		}
		s.Filter = ""
		s.Cursor = 0
		s.RememberSelection()
	case "graph":
		m.openCodeGraph()
	case "output":
		if s.Builds[s.Preference.Selected] == nil {
			m.renderSelectedCode()
		} else {
			s.ChooseSection(1)
		}
	}
	m.rememberCode(s)
}

func (m *model) closeCodeDetail() bool {
	s := m.coder()
	if !s.CloseDetail() {
		return false
	}
	m.tool = s.View
	m.rememberCode(s)
	return true
}

func (m *model) openCodeGraph() {
	s := m.coder()
	if s.View == "code-kustomize" {
		s.OpenKustomizeGraph()
		m.rememberCode(s)
		return
	}
	if s.ReturnView == "code-kustomize" {
		if m.openCodeNodeOverview(s.Preference.Selected) {
			s.OpenKustomizeGraph()
			m.rememberCode(s)
		}
		return
	}
	s.OpenConfigurationMap()
}

func (m *model) openCodeNodeOverview(id string) bool {
	s := m.coder()
	if s.ReturnView != "code-kustomize" || s.Inventory == nil {
		return false
	}
	entry := s.Inventory.Entry(id)
	if entry == nil || entry.Kind != "kustomize" && entry.Kind != "kustomize+helm" {
		return false
	}
	s.PushLocation()
	m.tool, s.View = "code-kustomize", "code-kustomize"
	s.NodeGraph = ""
	s.SyncListScope()
	s.Filter = ""
	s.SelectKustomizeNode(id)
	s.BrowserDetail, s.Preference.Selected = true, id
	m.rememberCode(s)
	return true
}

func (m *model) activateCodeDiagnostics() {
	s := m.coder()
	detail := s.PreviewFailure(s.DiagnosticEntry())
	if detail == "" {
		detail = s.Err
	}
	s.ShowDocument("Render technical details", detail)
}
