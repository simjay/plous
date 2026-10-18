package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Operation records identities and outcomes, never values, manifests, or credentials.
type Operation struct {
	ID                    string    `json:"id"`
	WorkspaceID           string    `json:"workspace_id"`
	ConnectionID          string    `json:"connection_id"`
	ConnectionFingerprint string    `json:"connection_fingerprint,omitempty"`
	Tool                  string    `json:"tool"`
	Context               string    `json:"context"`
	Namespace             string    `json:"namespace"`
	Release               string    `json:"release"`
	Kind                  string    `json:"kind"`
	Status                string    `json:"status"`
	Revision              int       `json:"revision,omitempty"`
	Chart                 string    `json:"chart,omitempty"`
	Version               string    `json:"version,omitempty"`
	SourceTime            string    `json:"source_time,omitempty"`
	ObservedAt            time.Time `json:"observed_at"`
}

func (s *Store) RecordOperation(op Operation) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return op, ErrClosed
	}
	if !idPattern.MatchString(op.WorkspaceID) || !idPattern.MatchString(op.ConnectionID) || op.Tool != "helm" {
		return op, errors.New("invalid operation scope")
	}
	history, err := s.history(op.WorkspaceID)
	validScope := len(history) > 0
	if IsCurrentScope(op.WorkspaceID) {
		var scope Scope
		scope, err = s.readScope(op.WorkspaceID)
		validScope = err == nil && scope.Connection.ID == op.ConnectionID
	}
	if err != nil || !validScope {
		return op, errors.New("operation workspace is unavailable")
	}
	if op.ID == "" {
		op.ID = newID()
	}
	if !idPattern.MatchString(op.ID) {
		return op, errors.New("invalid operation identity")
	}
	if op.ObservedAt.IsZero() {
		op.ObservedAt = time.Now().UTC()
	}
	dir := filepath.Join(s.scopeDir(op.WorkspaceID), "operations")
	if err = privateDir(dir); err != nil {
		return op, err
	}
	data, err := json.Marshal(op)
	if err != nil {
		return op, err
	}
	if len(data) > 16<<10 {
		return op, errors.New("operation metadata exceeds limit")
	}
	name := newID()
	stage := filepath.Join(dir, ".pending-"+name)
	defer os.Remove(stage)
	if err = writeFile(stage, append(data, '\n')); err != nil {
		return op, err
	}
	if err = os.Rename(stage, filepath.Join(dir, name+".json")); err != nil {
		return op, err
	}
	return op, syncDir(dir)
}
func (s *Store) Operations(workspaceID string) ([]Operation, error) {
	return s.OperationsContext(context.Background(), workspaceID)
}

func (s *Store) OperationsContext(ctx context.Context, workspaceID string) ([]Operation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	closed := s.lock == nil
	s.mu.Unlock()
	if closed {
		return nil, ErrClosed
	}
	if !idPattern.MatchString(workspaceID) {
		return nil, errors.New("invalid workspace identity")
	}
	dir := filepath.Join(s.scopeDir(workspaceID), "operations")
	entries, err := readDirBounded(dir, 100000)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	latest := map[string]Operation{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if strings.HasPrefix(entry.Name(), ".pending-") {
			continue
		}
		data, e := readFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			return nil, e
		}
		if len(data) > 16<<10 {
			return nil, errors.New("operation record exceeds limit")
		}
		var op Operation
		if e = json.Unmarshal(data, &op); e != nil || op.WorkspaceID != workspaceID || op.ID == "" {
			return nil, errors.New("invalid operation record")
		}
		old, ok := latest[op.ID]
		if !ok || op.ObservedAt.After(old.ObservedAt) {
			latest[op.ID] = op
		}
	}
	result := make([]Operation, 0, len(latest))
	for _, op := range latest {
		result = append(result, op)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ObservedAt.After(result[j].ObservedAt) })
	return result, nil
}
