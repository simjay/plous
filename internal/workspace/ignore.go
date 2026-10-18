package workspace

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const defaultSourceIgnore = `# Global exclusions for Plous browsing, search and configuration discovery.
# Patterns are relative to each Code source root. Exact reads are unaffected.
.git/
node_modules/
.venv/
venv/
__pycache__/
.pytest_cache/
.mypy_cache/
`

// SourceIgnore preserves existing policy, including an intentionally empty file.
func (s *Store) SourceIgnore() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, ErrClosed
	}
	name := filepath.Join(s.root, ".plousignore")
	fd, err := unix.Open(name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err == nil {
		f := os.NewFile(uintptr(fd), name)
		_, err = f.WriteString(defaultSourceIgnore)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	} else if errors.Is(err, os.ErrExist) {
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("create .plousignore: %w", err)
	}
	fd, err = unix.Open(name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open .plousignore: %w", err)
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New(".plousignore must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("read .plousignore: %w", err)
	}
	if len(data) > 2<<20 {
		return nil, errors.New(".plousignore exceeds 2 MiB")
	}
	return data, nil
}
