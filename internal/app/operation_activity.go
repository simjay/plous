package app

import (
	"fmt"
	"strings"

	"github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	"github.com/simjay/plous/internal/workspace"
)

func (m *model) activityKey() string {
	return m.active + "/" + m.tool + "/" + m.connection + "/" + modes[2]
}

func (m *model) activityPosition() int {
	if m.activityUI.Filter == activity.All {
		return m.positions[m.activityKey()]
	}
	return m.activityUI.Positions[fmt.Sprintf("%s/%d", m.activityKey(), m.activityUI.Filter)]
}

func (m *model) setActivityPosition(position int) {
	if m.activityUI.Filter == activity.All {
		m.positions[m.activityKey()] = position
		return
	}
	if m.activityUI.Positions == nil {
		m.activityUI.Positions = map[string]int{}
	}
	m.activityUI.Positions[fmt.Sprintf("%s/%d", m.activityKey(), m.activityUI.Filter)] = position
}

func (m *model) rebaseActivitySearch(s *panelSearch) {
	if s == nil || s.kind != "activity" || s.scope != m.activityKey()+"/" {
		return
	}
	entries := m.activityUI.Entries(m.active, m.tool, m.connection, "")
	s.cursor = min(max(0, s.cursor), max(0, len(entries)-1))
	for i, entry := range entries {
		if entry.Identity() == s.activityIdentity {
			s.cursor = i
			break
		}
	}
	s.activityFilter = m.activityUI.Filter
}

func (m *model) restoreActivityPosition(s *panelSearch) {
	if s.scope == m.activityKey()+"/" {
		m.rebaseActivitySearch(s)
		m.setActivityPosition(s.cursor)
	} else if s.activityFilter == activity.All {
		m.positions[strings.TrimSuffix(s.scope, "/")] = s.cursor
	}
}

func (m *model) activityEntries() []activity.ActivityEntry {
	query := ""
	if search := m.searches[1]; search != nil && search.kind == "activity" {
		query = search.input.Text
	} else if search := m.activityListSearch; search != nil && search.scope == m.viewKey()+"/" {
		query = search.input.Text
	}
	return m.activityUI.Entries(m.active, m.tool, m.connection, query)
}

func (m *model) openActivityEntry(e activity.ActivityEntry) {
	if search := m.searches[1]; search != nil && search.kind == "activity" {
		m.activityListSearch, m.searches[1] = search, nil
	}
	m.activityRecord, m.activityObservations = &e, false
	if e.Change != nil {
		m.setActivityDetail(*e.Change)
		return
	}
	op := e.Operation
	m.operationDetail = op
	m.activityReview = nil
	m.cursor = 0
	m.updateActivityRecordDetail()
}

func (m *model) activityReleaseUnavailable(op *workspace.Operation) string {
	if op == nil {
		return "No release is recorded for this entry."
	}
	if op.WorkspaceID != m.active {
		return "The recorded release belongs to another workspace."
	}
	found := false
	if w := m.current(); w != nil {
		for _, c := range w.Connections {
			if c.ID == op.ConnectionID && monitorCluster(c.Tool) {
				if c.Settings["context"] != op.Context || op.ConnectionFingerprint != "" && monitor.HelmConfig(&c).Fingerprint() != op.ConnectionFingerprint {
					return "The saved Helm connection has changed. Restore its recorded settings before opening this release."
				}
				found = true
			}
		}
	}
	if !found {
		return "The recorded Helm connection is no longer saved in this workspace."
	}
	return ""
}

func (m *model) updateActivityRecordDetail() {
	m.detail = m.activityRecord.Detail(m.activityObservations)
	if reason := m.activityReleaseUnavailable(m.operationDetail); reason != "" {
		m.detail += "\n\n" + reason
	}
}

func (m *model) openOperationRelease() {
	op := m.operationDetail
	if op == nil {
		return
	}
	if reason := m.activityReleaseUnavailable(op); reason != "" {
		m.status = reason
		return
	}
	m.selectScope(op.WorkspaceID, "helm", op.ConnectionID)
	h := m.helmer()
	if !m.chooseHelmSection("Releases") {
		return
	}
	m.mode, m.focus = 1, 1
	revision := op.Revision
	if op.Kind != "revision" {
		revision = 0
	}
	h.RequestedRelease = &helmtool.Release{Name: op.Release, Namespace: op.Namespace, Revision: revision}
	if h.Client != nil {
		h.Pending = "detail"
	}
	m.rememberHelm(h)
}
