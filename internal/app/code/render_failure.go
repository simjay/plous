package code

import (
	"path"
	"strings"

	kustomize "github.com/simjay/plous/internal/integrations/kustomize/common"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (s *CodeState) DiagnosticEntry() string {
	if IsBrowser(s.View) {
		if item, ok := s.SelectedCatalogItem(); ok {
			return item.EntryID
		}
		return ""
	}
	return s.Preference.Selected
}

func (s *CodeState) FailureView(id string, width int) string {
	failure := s.PreviewFailure(id)
	if failure == "" {
		return ""
	}
	title := "Couldn't render configuration"
	if entry := s.Inventory.Entry(id); entry != nil {
		title = "Couldn't render " + path.Base(entry.Path)
	}
	summary := "Could not generate Kubernetes YAML from this configuration."
	if strings.HasPrefix(id, "kustomize:") {
		if explanation := kustomize.RenderFailure(failure); explanation != "" {
			summary = explanation
		}
	}
	if strings.Contains(strings.ToLower(failure), "cancel") {
		title = "Render cancelled"
		summary = "Retry when you are ready."
		if s.Builds[id] != nil {
			summary = "Previous successful output is still available. " + summary
		}
	}
	actions := []textview.Action{{Label: "Technical details", Mouse: "code-browser/diagnostics"}}
	if s.Inventory.Entry(id) != nil && !s.BrowserDetail {
		actions = append([]textview.Action{{Label: "Open source", Mouse: "code-browser/failure-source"}}, actions...)
	}
	bar := textview.ActionBar([]textview.ActionGroup{{Actions: actions}}, max(1, width-3), 2, ui.WorkspaceTheme())
	return ui.DetailCallout(ui.OneLine(title), ui.Plain(summary), bar, width, ui.RemovedStyle)
}
