package workspace

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/simjay/plous/internal/ai"
	"golang.org/x/sys/unix"
)

// Credentials are mutable secrets, separate from chronological settings records.
func (s *Store) credentials() (map[string]string, error) {
	path := filepath.Join(s.root, "credentials.json")
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, errors.New("cannot open credentials.json")
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("credentials.json must be a regular file")
	}
	if err := f.Chmod(0600); err != nil {
		return nil, errors.New("cannot protect credentials.json")
	}
	data, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("cannot read credentials.json")
	}
	var keys map[string]string
	if json.Unmarshal(data, &keys) != nil || keys == nil {
		return nil, errors.New("credentials.json is corrupt; preserved for recovery")
	}
	return keys, nil
}

func (s *Store) Credential(c ai.Config) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return "", ErrClosed
	}
	if c.CredentialRef == "" {
		return os.Getenv(c.KeyEnv), nil
	}
	keys, err := s.credentials()
	if err != nil {
		return "", err
	}
	key := keys[c.CredentialRef]
	if key == "" {
		return "", errors.New("saved API key is missing; enter it again in Settings")
	}
	return key, nil
}

func (s *Store) SaveCredential(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return "", ErrClosed
	}
	if key == "" || len(key) > 8192 || strings.ContainsAny(key, "\r\n\x00") {
		return "", errors.New("API key is empty or invalid")
	}
	keys, err := s.credentials()
	if err != nil {
		return "", err
	}
	id := newID()
	keys[id] = key
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil || len(data)+1 > 1<<20 {
		return "", errors.New("credential storage exceeds the local size limit")
	}
	if err := replaceJSON(filepath.Join(s.root, "credentials.json"), keys); err != nil {
		return "", errors.New("could not save credentials.json")
	}
	return id, nil
}

// Keep keys referenced by current connections. Historical records keep only IDs.
func (s *Store) PruneCredentials() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return ErrClosed
	}
	settings, err := s.settings()
	if err != nil {
		return err
	}
	keys, err := s.credentials()
	if err != nil {
		return err
	}
	used := map[string]bool{}
	for _, config := range settings.Configs() {
		used[config.CredentialRef] = true
	}
	changed := false
	for id := range keys {
		if !used[id] {
			delete(keys, id)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := replaceJSON(filepath.Join(s.root, "credentials.json"), keys); err != nil {
		return errors.New("could not remove unused API keys")
	}
	return nil
}
