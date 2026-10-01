package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/simjay/plous/internal/ai"
)

type Settings struct {
	Revision  uint64      `json:"revision"`
	Agent     ai.Config   `json:"agent"`
	Providers []ai.Config `json:"providers,omitempty"`
}

// Agent is the selected model; Providers contains the saved connections.
func (s Settings) Configs() []ai.Config {
	configs := append([]ai.Config(nil), s.Providers...)
	for i := range configs {
		configs[i] = configs[i].Connection()
	}
	return configs
}

func (s Settings) validate() error {
	if err := s.Agent.Validate(); err != nil {
		return err
	}
	names := map[string]bool{}
	active := len(s.Providers) == 0 && s.Agent == ai.DefaultConfig()
	for _, c := range s.Providers {
		if err := c.Validate(); err != nil {
			return err
		}
		name := strings.ToLower(strings.TrimSpace(c.Name))
		if names[name] {
			return errors.New("AI connection names must be unique")
		}
		names[name] = true
		active = active || c.Connection() == s.Agent.Connection()
	}
	if !active {
		return errors.New("selected AI connection must match a saved connection")
	}
	return nil
}

type SettingsChange struct {
	Time   time.Time `json:"time"`
	Before Settings  `json:"before"`
	After  Settings  `json:"after"`
}

func (s *Store) ReadSettings() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return Settings{}, ErrClosed
	}
	return s.settings()
}
func (s *Store) settings() (Settings, error) {
	current := Settings{Agent: ai.DefaultConfig()}
	dir := filepath.Join(s.root, "settings")
	if err := privateDir(dir); err != nil {
		return current, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return current, nil
	}
	if err != nil {
		return current, err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".pending-") {
			continue
		}
		if !entry.IsDir() {
			return current, errors.New("unexpected settings storage entry")
		}
		var change SettingsChange
		if err := readJSON(filepath.Join(dir, entry.Name(), "change.json"), &change); err != nil {
			return current, err
		}
		if !reflect.DeepEqual(change.Before, current) || change.After.Revision != current.Revision+1 || change.Time.IsZero() {
			return current, errors.New("corrupt application settings revision chain")
		}
		if err := change.After.validate(); err != nil {
			return current, err
		}
		current = change.After
	}
	return current, nil
}
func (s *Store) SaveSettings(v Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return v, ErrClosed
	}
	if err := v.validate(); err != nil {
		return v, err
	}
	before, err := s.settings()
	if err != nil {
		return v, err
	}
	if before.Revision != v.Revision {
		return v, ErrConflict
	}
	v.Revision++
	dir := filepath.Join(s.root, "settings")
	if err := privateDir(dir); err != nil {
		return before, err
	}
	now := time.Now().UTC()
	name := fmt.Sprintf("%020d-%s-%s", v.Revision, now.Format("20060102T150405.000000000Z"), newID())
	err = publishJSONDir(dir, name, map[string]any{"change.json": SettingsChange{Time: now, Before: before, After: v}})
	if err != nil {
		return before, err
	}
	return v, nil
}

func readJSON(path string, v any) error {
	data, err := readFile(path)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("corrupt %s; preserved for recovery: %w", path, err)
	}
	return nil
}
func replaceJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	temp := filepath.Join(filepath.Dir(path), ".pending-"+newID())
	defer os.Remove(temp)
	if err = writeFile(temp, append(data, '\n')); err != nil {
		return err
	}
	if err = os.Rename(temp, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}
func publishJSONDir(parent, name string, files map[string]any) error {
	stage, err := os.MkdirTemp(parent, ".pending-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for name, v := range files {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		if err = writeFile(filepath.Join(stage, name), append(data, '\n')); err != nil {
			return err
		}
	}
	if err = syncDir(stage); err != nil {
		return err
	}
	if err = os.Rename(stage, filepath.Join(parent, name)); err != nil {
		return err
	}
	return syncDir(parent)
}
