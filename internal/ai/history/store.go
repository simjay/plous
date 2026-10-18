// Package history owns saved conversations, evidence and native agent records.
package history

import (
	"errors"
	"regexp"
	"sync"
)

var ErrClosed = errors.New("history store is closed")
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

// Profile keeps the process lock until registered resources have closed.
type Profile interface {
	DataRoot() (string, error)
	AddCloser(func() error) error
}

type Store struct {
	searchMu        sync.Mutex
	searches        map[string]*SearchIndex
	scopeDir        func(string) (string, error)
	historyChanges  map[string][]chan struct{}
	histories       map[string]*historyState
	historyDBs      map[string]*historyDB
	closers         []func() error
	closeOnce       sync.Once
	closeErr        error
	closing, closed bool
	mu              sync.Mutex
	root            string
}

// Open shares the profile lifetime without taking a second process lock.
func Open(profile Profile) (*Store, error) {
	if profile == nil {
		return nil, errors.New("history requires an open profile")
	}
	root, err := profile.DataRoot()
	if err != nil {
		return nil, err
	}
	s := &Store{root: root}
	if p, ok := profile.(interface{ ConversationScopeDir(string) (string, error) }); ok {
		s.scopeDir = p.ConversationScopeDir
	}
	if err := profile.AddCloser(s.Close); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closing = true
		closers := append([]func() error(nil), s.closers...)
		s.mu.Unlock()
		for i := len(closers) - 1; i >= 0; i-- {
			s.closeErr = errors.Join(s.closeErr, closers[i]())
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, db := range s.historyDBs {
			s.closeErr = errors.Join(s.closeErr, db.Close())
		}
		s.closed = true
	})
	return s.closeErr
}

func (s *Store) AddCloser(close func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.closing {
		return ErrClosed
	}
	s.closers = append(s.closers, close)
	return nil
}
