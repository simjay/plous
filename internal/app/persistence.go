package app

import (
	tea "charm.land/bubbletea/v2"
)

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
