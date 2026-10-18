package app

import (
	"github.com/simjay/plous/internal/app/monitor"

	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
	diffview "github.com/simjay/plous/internal/ui/diff"
	"strings"
)

func (m *model) executeHelm(command string) bool {
	if !m.helmActive() {
		return false
	}
	argument := ""
	if name, value, ok := strings.Cut(command, " "); ok && name == "namespace" {
		command, argument = name, strings.TrimSpace(value)
		if !monitor.ValidExploreNamespace(argument) {
			m.status = "Use a namespace name or * for all namespaces."
			return true
		}
	}
	if !monitor.ContainsString([]string{"refresh", "namespace", "actions", "rollback", "review", "release", "revision"}, command) {
		return false
	}
	if m.form != nil || m.settingsOverlay != nil {
		m.status = "Finish the open setup form before using Helm actions."
		return true
	}
	h := m.helmer()
	if h.Document != nil || h.Comparison != nil {
		m.status = "Close the document or comparison before using Helm actions."
		return true
	}
	m.closeHelp()
	m.focus = 1
	switch command {
	case "refresh":
		if !h.Busy {
			h.Err = ""
			if h.Client == nil {
				h.Pending = "connect"
			} else if h.Detail != nil {
				h.Pending = "detail"
			} else if h.Timeline != nil {
				h.StopTimelineLoad()
				h.Pending = "timeline"
			} else if h.WorkflowVisible() {
				h.Notice = "Preview in Chart inputs rebuilds the preview. Use Actions to edit prepared inputs."
			} else {
				h.Pending = "list"
			}
		}
	case "namespace":
		if h.WorkflowVisible() {
			h.Notice = "Set the namespace in Chart inputs. Prepared previews keep their reviewed target."
		} else if h.Running != "execute" {
			if h.Cancel != nil {
				h.Cancel()
				h.Cancel = nil
				h.Run++
			}
			h.Busy, h.Running, h.Pending = false, "", ""
			if argument != "" {
				m.chooseHelmNamespace(h, monitor.ExploreNamespace(argument))
			} else {
				m.openHelmNamespacePicker(h)
			}
		}
	case "actions":
		m.helmActions(h)
	case "rollback":
		if h.WorkflowVisible() && h.Plan != nil && h.Plan.Operation == "rollback" {
			m.confirmHelm(h)
			return true
		}
		if h.Workflow != nil {
			h.Notice = "Resume or discard the unfinished Helm action from Actions first."
			return true
		}
		m.rollbackHelmSelection(h)
	case "release", "revision":
		h.OpenInspectorPicker(command)
	case "review":
		if h.Timeline != nil && h.Detail == nil {
			m.reviewHelmRevision(h)
			return true
		}
		if h.Plan != nil && h.WorkflowVisible() {
			before, after := manifestutil.ManifestComparison(h.Plan.Before, h.Plan.Manifest, h.Plan.Namespace)
			h.Comparison = diffview.New(before, after, h.Plan.Context+" / "+h.Plan.Namespace+" / "+h.Plan.Name, uint64(h.Plan.CurrentRevision), false)
			h.Comparison.Notice = "Resource preview. Sensitive fields masked. Esc returns to the preview."
		}
	default:
		return false
	}
	return true
}
