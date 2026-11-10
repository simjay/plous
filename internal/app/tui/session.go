package tui

import (
	"bytes"
	"encoding/json"
	"maps"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/activity"
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

// Update owns the queue and applies acknowledgments. Workers see only captured values.
type persistenceJob struct {
	key    string
	write  func() any
	apply  func(any) tea.Cmd
	done   chan struct{}
	result any
}

type persistenceResult struct{ job *persistenceJob }

func (m *model) enqueuePersistence(job *persistenceJob) {
	if job.key != "" {
		for i, queued := range m.persistenceQueue {
			if queued.key == job.key {
				m.persistenceQueue[i] = job
				return
			}
		}
	}
	m.persistenceQueue = append(m.persistenceQueue, job)
}

func (m *model) syncPersistence() tea.Cmd {
	if m.persistenceActive != nil || len(m.persistenceQueue) == 0 {
		return nil
	}
	job := m.persistenceQueue[0]
	m.persistenceQueue = m.persistenceQueue[1:]
	m.persistenceActive = job
	job.done = make(chan struct{})
	go func() { job.result = job.write(); close(job.done) }()
	return func() tea.Msg { <-job.done; return persistenceResult{job} }
}

func (m *model) applyPersistence(result persistenceResult) tea.Cmd {
	if m.persistenceActive != result.job {
		return nil
	}
	m.persistenceActive = nil
	if result.job.apply != nil {
		return result.job.apply(result.job.result)
	}
	return nil
}

// Shutdown waits for the same ordered writes and acknowledgments as the UI.
func (m *model) drainPersistence() {
	for m.persistenceActive != nil || len(m.persistenceQueue) > 0 {
		m.syncPersistence()
		job := m.persistenceActive
		<-job.done
		m.applyPersistence(persistenceResult{job})
	}
}

func (m *model) syncQuit() tea.Cmd {
	if !m.quitting || m.persistenceActive != nil || len(m.persistenceQueue) > 0 {
		return nil
	}
	if m.persistenceErr != nil {
		m.quitting = false
		m.status = "Cannot save before exit: " + m.persistenceErr.Error()
		return nil
	}
	if m.run != nil || m.chatRestore.Worker != nil || m.nativeAgent != nil && !m.nativeAgent.Finished {
		return nil
	}
	if m.dirty {
		if err := m.queueSession(); err != nil {
			m.quitting = false
			m.status = "Cannot save before exit: " + err.Error()
		}
		return m.syncPersistence()
	}
	m.stopViews()
	return tea.Quit
}
