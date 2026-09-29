// Package workspace stores local configuration as immutable, audited revisions.
package workspace

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/sys/unix"
)

type Connection struct {
	ID       string            `json:"id"`
	Tool     string            `json:"tool"`
	Name     string            `json:"name"`
	Settings map[string]string `json:"settings"`
}

type Workspace struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Revision    uint64       `json:"revision"`
	Connections []Connection `json:"connections"`
}

// Change contains both sides of a save. After is also the authoritative current
// state, so publishing a configuration never leaves its audit record behind.
type Change struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	Actor       string     `json:"actor"`
	Revision    uint64     `json:"revision"`
	Time        time.Time  `json:"time"`
	Before      *Workspace `json:"before"`
	After       Workspace  `json:"after"`
}

var (
	ErrConflict = errors.New("workspace changed since it was opened; reload before saving")
	ErrClosed   = errors.New("workspace store is closed")
	idPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
)

type Store struct {
	mu   sync.Mutex
	root string
	lock *os.File
}

// Open takes a process-lifetime lock. Call Close when the application exits.
func Open(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("workspace storage location is empty")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return nil, err
	}
	if err := privateDir(root); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(root, "store.lock")
	fd, err := unix.Open(lockPath, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fmt.Errorf("open workspace lock: %w", err)
	}
	lock := os.NewFile(uintptr(fd), lockPath)
	info, err := lock.Stat()
	if err != nil || !info.Mode().IsRegular() {
		lock.Close()
		return nil, fmt.Errorf("workspace lock %s must be a regular file", lockPath)
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lock.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, fmt.Errorf("another Plous process is already using %s", root)
		}
		return nil, fmt.Errorf("lock workspace storage: %w", err)
	}
	s := &Store{root: root, lock: lock}
	if err := lock.Chmod(0600); err != nil {
		s.Close()
		return nil, err
	}
	if err := privateDir(filepath.Join(root, "workspaces")); err != nil {
		s.Close()
		return nil, err
	}
	if _, err := s.List(); err != nil {
		s.Close()
		return nil, err
	}
	if _, err := s.ReadSession(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil
	}
	err := s.lock.Close() // Closing the descriptor releases flock, even on error.
	s.lock = nil
	return err
}

func (s *Store) List() ([]Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, ErrClosed
	}
	entries, err := os.ReadDir(filepath.Join(s.root, "workspaces"))
	if err != nil {
		return nil, err
	}
	var result []Workspace
	for _, entry := range entries {
		if !entry.IsDir() || !idPattern.MatchString(entry.Name()) {
			return nil, fmt.Errorf("unexpected workspace storage entry %q", entry.Name())
		}
		history, err := s.history(entry.Name())
		if err != nil {
			return nil, err
		}
		if len(history) > 0 {
			result = append(result, history[len(history)-1].After)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (s *Store) Save(w Workspace, actor string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return Workspace{}, ErrClosed
	}
	if !validName(actor) {
		return Workspace{}, errors.New("a nonempty actor without control characters is required")
	}
	// Detach the caller's maps/slices before assigning IDs or revisions.
	w = clone(w)
	if w.ID == "" {
		if w.Revision != 0 {
			return Workspace{}, ErrConflict
		}
		w.ID = newID()
	}
	for i := range w.Connections {
		if w.Connections[i].ID == "" {
			w.Connections[i].ID = newID()
		}
	}
	if err := validate(w); err != nil {
		return Workspace{}, err
	}
	history, err := s.history(w.ID)
	if err != nil {
		return Workspace{}, err
	}
	var before *Workspace
	var revision uint64
	if len(history) > 0 {
		previous := history[len(history)-1].After
		before, revision = &previous, previous.Revision
	}
	if w.Revision != revision || revision == ^uint64(0) {
		return Workspace{}, ErrConflict
	}
	w.Revision++
	change := Change{
		ID: newID(), WorkspaceID: w.ID, Actor: actor, Revision: w.Revision,
		Time: time.Now().UTC(), Before: before, After: w,
	}
	dir := filepath.Join(s.root, "workspaces", w.ID)
	if err := privateDir(dir); err != nil {
		return Workspace{}, err
	}
	dir = filepath.Join(dir, "revisions")
	if err := privateDir(dir); err != nil {
		return Workspace{}, err
	}
	stage, err := os.MkdirTemp(dir, ".pending-")
	if err != nil {
		return Workspace{}, err
	}
	defer os.RemoveAll(stage)
	data, err := json.MarshalIndent(change, "", "  ")
	if err != nil {
		return Workspace{}, err
	}
	if err := writeFile(filepath.Join(stage, "change.json"), append(data, '\n')); err != nil {
		return Workspace{}, err
	}
	if err := syncDir(stage); err != nil {
		return Workspace{}, err
	}
	if err := os.Rename(stage, filepath.Join(dir, revisionName(change))); err != nil {
		return Workspace{}, err
	}
	if err := syncDir(dir); err != nil {
		return w, fmt.Errorf("workspace revision %d was published, but syncing storage failed: %w", w.Revision, err)
	}
	return w, nil
}

func (s *Store) History(workspaceID string) ([]Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, ErrClosed
	}
	return s.history(workspaceID)
}

func (s *Store) history(id string) ([]Change, error) {
	if !idPattern.MatchString(id) {
		return nil, errors.New("invalid workspace ID")
	}
	dir := filepath.Join(s.root, "workspaces", id)
	for _, path := range []string{dir, filepath.Join(dir, "revisions")} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil // An interrupted first save may leave an empty directory.
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("workspace storage %s is not a directory", path)
		}
	}
	dir = filepath.Join(dir, "revisions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var result []Change
	var previous *Workspace
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".pending-") {
			continue // Only a fully published revision is authoritative.
		}
		if !entry.IsDir() {
			return nil, fmt.Errorf("unexpected revision entry %s", filepath.Join(dir, entry.Name()))
		}
		path := filepath.Join(dir, entry.Name(), "change.json")
		data, err := readFile(path)
		if err != nil {
			return nil, fmt.Errorf("read workspace revision %s: %w", path, err)
		}
		var change Change
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&change); err != nil {
			return nil, fmt.Errorf("corrupt workspace revision %s: %w", path, err)
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return nil, fmt.Errorf("corrupt workspace revision %s: trailing data", path)
		}
		if change.WorkspaceID != id || change.After.ID != id ||
			change.Revision != uint64(len(result))+1 || change.After.Revision != change.Revision ||
			!idPattern.MatchString(change.ID) || !validName(change.Actor) || change.Time.IsZero() ||
			entry.Name() != revisionName(change) || !reflect.DeepEqual(change.Before, previous) {
			return nil, fmt.Errorf("corrupt workspace revision chain at %s", path)
		}
		if err := validate(change.After); err != nil {
			return nil, fmt.Errorf("corrupt workspace revision %s: %w", path, err)
		}
		result = append(result, change)
		previous = &result[len(result)-1].After
	}
	return result, nil
}

func (s *Store) ReadSession() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, ErrClosed
	}
	return s.readSession()
}

func (s *Store) readSession() ([]byte, error) {
	data, err := readFile(filepath.Join(s.root, "session.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !json.Valid(data) {
		return nil, errors.New("session.json is corrupt; preserved for recovery")
	}
	return data, nil
}

func (s *Store) WriteSession(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return ErrClosed
	}
	if !json.Valid(data) {
		return errors.New("session must be valid JSON")
	}
	if _, err := s.readSession(); err != nil {
		return err // Do not replace a corrupt session with a new blank one.
	}
	path := filepath.Join(s.root, ".session-"+newID())
	defer os.Remove(path)
	if err := writeFile(path, data); err != nil {
		return err
	}
	if err := os.Rename(path, filepath.Join(s.root, "session.json")); err != nil {
		return err
	}
	return syncDir(s.root)
}

func validate(w Workspace) error {
	if !idPattern.MatchString(w.ID) || !validName(w.Name) {
		return errors.New("workspace requires a valid ID and nonempty name without control characters")
	}
	ids := make(map[string]bool)
	for _, c := range w.Connections {
		if !idPattern.MatchString(c.ID) || ids[c.ID] || !validName(c.Name) {
			return errors.New("connections require unique valid IDs and nonempty names without control characters")
		}
		ids[c.ID] = true
		switch c.Tool {
		case "kubernetes", "helm", "kustomize", "terraform", "argo", "kargo":
		default:
			return fmt.Errorf("unsupported tool %q", c.Tool)
		}
		for key := range c.Settings {
			if !validName(key) {
				return errors.New("connection setting names cannot be empty or contain control characters")
			}
		}
	}
	return nil
}

func validName(name string) bool {
	return strings.TrimSpace(name) != "" && strings.IndexFunc(name, unicode.IsControl) < 0
}

func clone(w Workspace) Workspace {
	if w.Connections != nil {
		w.Connections = append([]Connection{}, w.Connections...)
	}
	for i, c := range w.Connections {
		if c.Settings != nil {
			w.Connections[i].Settings = make(map[string]string, len(c.Settings))
			for key, value := range c.Settings {
				w.Connections[i].Settings[key] = value
			}
		}
	}
	return w
}

func newID() string {
	var value [16]byte
	// crypto/rand.Read cannot return an error on supported Go platforms.
	rand.Read(value[:])
	return hex.EncodeToString(value[:])
}

func revisionName(c Change) string {
	return fmt.Sprintf("%020d-%s-%s", c.Revision, c.Time.UTC().Format("20060102T150405.000000000Z"), c.ID)
}

func privateDir(path string) error {
	err := os.Mkdir(path, 0700)
	created := err == nil
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("storage path %s is not a directory", path)
	}
	if err := os.Chmod(path, 0700); err != nil {
		return err
	}
	if created {
		return syncDir(filepath.Dir(path))
	}
	return nil
}

func readFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("storage file %s is not a regular file", path)
	}
	return os.ReadFile(path)
}

func writeFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
