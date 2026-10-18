package app

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"github.com/simjay/plous/internal/app/activity"
	"maps"
	"strings"
)

func (m *model) queueSession() error {
	if m.store == nil {
		return nil
	}
	if r := m.chatRestore.Worker; r != nil {
		select {
		case <-r.Done:
		default:
			return nil
		}
	}
	for _, c := range m.chats {
		if c.Restore != nil {
			return nil
		}
	}
	m.queueChatSaves()
	s := session{ChatIDs: m.chatIDs, Version: 2, Workspace: m.active, Connection: m.connection, Tool: m.tool, Mode: m.mode, Focus: m.focus, Expanded: m.expanded, Drafts: map[string]string{}, Positions: m.positions}
	s.LeftHidden, s.RightHidden = m.leftHidden, m.rightHidden
	s.AssistantSize = m.assistantSize
	s.Explore = m.explorePreferences
	s.Helm = m.helmPreferences
	s.GitOps = m.gitOpsPreferences
	s.Kustomize = m.kustomizePreferences
	s.Code = m.codeUI.Preferences
	s.AutoExpanded = m.autoExpanded
	rows := m.treeRows()
	if m.treeCursor >= 0 && m.treeCursor < len(rows) {
		s.TreeKey = rows[m.treeCursor].key
	}
	if search := m.searches[0]; search != nil {
		s.TreeKey = search.treeKey
	}
	if search := m.searches[1]; search != nil && search.kind == "activity" && search.activityFilter == activity.All {
		s.Positions = maps.Clone(m.positions)
		s.Positions[strings.TrimSuffix(search.scope, "/")] = search.cursor
	}
	if search := m.activityListSearch; search != nil && search.activityFilter == activity.All {
		s.Positions = maps.Clone(s.Positions)
		s.Positions[strings.TrimSuffix(search.scope, "/")] = search.cursor
	}
	for id, e := range m.drafts {
		s.Drafts[id] = e.Text
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	pending := m.persistenceActive != nil && m.persistenceActive.key == "session"
	for _, job := range m.persistenceQueue {
		pending = pending || job.key == "session"
	}
	if bytes.Equal(data, m.lastSession) && !pending {
		m.dirty = false
		return nil
	}
	store := m.store
	m.dirty = false
	m.enqueuePersistence(&persistenceJob{key: "session", write: func() any { return store.WriteSession(data) }, apply: func(result any) tea.Cmd {
		if err, _ := result.(error); err != nil {
			m.dirty = true
			m.persistenceErr = err
			m.status = "Could not save session: " + err.Error()
		} else {
			m.lastSession = data
		}
		return nil
	}})
	return nil
}

func (m *model) persist() { m.dirty = true }

// Used after the event loop exits, and by callers that explicitly need a durable snapshot.
func (m *model) saveSession() error {
	m.drainPersistence()
	m.persistenceErr = nil
	for _, c := range m.chats {
		if c.Dirty && c.SaveFailed {
			m.queueChatSave(c)
		}
	}
	if err := m.queueSession(); err != nil {
		return err
	}
	m.drainPersistence()
	return m.persistenceErr
}
