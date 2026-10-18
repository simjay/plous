package workspace

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Scope identifies conversation ownership independently of saved configurations.
type Scope struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	WorkspaceID string     `json:"workspace_id,omitempty"`
	Connection  Connection `json:"connection"`
}

func ConnectionScopeID(workspaceID, connectionID string) string {
	return fmt.Sprintf("conn-%x", sha256.Sum256([]byte(workspaceID+"\x00"+connectionID)))
}

func CurrentScopeID(fingerprint string) string { return "ctx-" + fingerprint }
func IsCurrentScope(id string) bool {
	return strings.HasPrefix(id, "ctx-") || strings.HasPrefix(id, "dir-")
}
func IsConnectionScope(id string) bool { return strings.HasPrefix(id, "conn-") }

// CurrentDirectoryScope keeps launch-directory history independent of clusters.
// The caller supplies the absolute, canonical directory captured at launch.
func CurrentDirectoryScope(path string) Scope {
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte("directory-v1\x00"+path)))
	return Scope{ID: "dir-" + fingerprint, Kind: "current_directory", Connection: Connection{
		ID: "source-" + fingerprint, Name: filepath.Base(path), Tool: "code",
		Settings: map[string]string{"path": path, "source_type": "Local directory"},
	}}
}

func validScope(scope Scope) bool {
	if !idPattern.MatchString(scope.ID) || !idPattern.MatchString(scope.Connection.ID) {
		return false
	}
	switch scope.Kind {
	case "connection":
		return idPattern.MatchString(scope.WorkspaceID) && scope.ID == ConnectionScopeID(scope.WorkspaceID, scope.Connection.ID)
	case "current_context":
		return len(scope.ID) == 68 && strings.HasPrefix(scope.ID, "ctx-") && scope.Connection.ID == "kube-"+strings.TrimPrefix(scope.ID, "ctx-") && scope.WorkspaceID == ""
	case "current_directory":
		path := scope.Connection.Settings["path"]
		expected := CurrentDirectoryScope(path)
		return filepath.IsAbs(path) && filepath.Clean(path) == path && scope.ID == expected.ID &&
			scope.Connection.ID == expected.Connection.ID && scope.Connection.Tool == "code" &&
			scope.WorkspaceID == ""
	}
	return false
}

func (s *Store) scopeDir(id string) string {
	area := "workspaces"
	if IsCurrentScope(id) {
		area = "contexts"
	}
	if IsConnectionScope(id) {
		area = "connections"
	}
	if strings.HasPrefix(id, "dir-") {
		area = "directories"
	}
	return filepath.Join(s.root, area, id)
}

func (s *Store) readScope(id string) (Scope, error) {
	var scope Scope
	if !idPattern.MatchString(id) || !IsCurrentScope(id) && !IsConnectionScope(id) {
		return scope, errors.New("invalid connection scope")
	}
	dir := s.scopeDir(id)
	for _, path := range []string{filepath.Dir(dir), dir} {
		info, err := os.Lstat(path)
		if err != nil {
			return scope, err
		}
		if !info.IsDir() {
			return scope, errors.New("history scope must be a directory")
		}
	}
	data, err := readFile(filepath.Join(dir, "scope.json"))
	if err != nil {
		return scope, err
	}
	if json.Unmarshal(data, &scope) != nil || scope.ID != id || !validScope(scope) {
		return Scope{}, errors.New("invalid saved connection scope")
	}
	return scope, nil
}

func (s *Store) EnsureScope(scope Scope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return ErrClosed
	}
	if !validScope(scope) {
		return errors.New("invalid connection scope")
	}
	// Persist the same credential-free settings allowed in workspace evidence.
	scope.Connection = safeWorkspace(Workspace{Connections: []Connection{scope.Connection}}).Connections[0]
	dir := s.scopeDir(scope.ID)
	if err := privateDir(filepath.Dir(dir)); err != nil {
		return err
	}
	if err := privateDir(dir); err != nil {
		return err
	}
	if _, err := s.readScope(scope.ID); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.Marshal(scope)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, ".scope-"+newID())
	defer os.Remove(path)
	if err = writeFile(path, data); err != nil {
		return err
	}
	if err = os.Rename(path, filepath.Join(dir, "scope.json")); err != nil {
		return err
	}
	return syncDir(dir)
}

// ConversationScopeDir is the history store's validated routing boundary.
func (s *Store) ConversationScopeDir(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return "", ErrClosed
	}
	if id == "" {
		return s.root, nil
	}
	if !idPattern.MatchString(id) {
		return "", errors.New("invalid history scope")
	}
	if IsCurrentScope(id) || IsConnectionScope(id) {
		if _, err := s.readScope(id); err != nil {
			return "", err
		}
	}
	dir := s.scopeDir(id)
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("history scope must be a directory")
	}
	return dir, nil
}

func (s *Store) Scopes() ([]Scope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, ErrClosed
	}
	var result []Scope
	for _, area := range []string{"contexts", "directories", "connections"} {
		entries, err := readDirBounded(filepath.Join(s.root, area), 10000)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				return nil, errors.New("invalid history scope directory")
			}
			scope, err := s.readScope(entry.Name())
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			result = append(result, scope)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Connection.Name < result[j].Connection.Name })
	return result, nil
}
