package monitor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/validation"
)

type storedState struct {
	State
	Control, Kubeconfig string
}

const (
	stateFile = "watch.json"
	lockFile  = "watch.lock"
)

func Root(dataDir string) (string, error) {
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataDir = filepath.Join(home, ".plous")
	}
	return filepath.Abs(filepath.Join(dataDir, "monitor"))
}

func validateName(name string) error {
	if len(validation.IsDNS1123Label(name)) != 0 {
		return errors.New("monitor name must be 1 to 63 lowercase letters, digits, or hyphens, starting and ending with a letter or digit")
	}
	return nil
}

func directory(root, name string) (string, error) {
	if root == "" {
		return "", errors.New("monitor storage is unavailable")
	}
	if err := validateName(name); err != nil {
		return "", err
	}
	dir := filepath.Join(root, name)
	info, err := os.Lstat(dir)
	switch {
	case err == nil && !info.IsDir():
		return "", errors.New("monitor path must be a directory")
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return "", err
	case errors.Is(err, os.ErrNotExist) && name == "default":
		// An earlier release stored the default monitor at the root.
		if _, legacyErr := os.Stat(filepath.Join(root, lockFile)); legacyErr == nil {
			return root, nil
		} else if !errors.Is(legacyErr, os.ErrNotExist) {
			return "", legacyErr
		}
	}
	return dir, nil
}

func List(root string) ([]State, error) {
	if root == "" {
		return nil, errors.New("monitor storage is unavailable")
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	legacy := false
	for _, entry := range entries {
		switch {
		case entry.IsDir() && validateName(entry.Name()) == nil:
			names = append(names, entry.Name())
		case entry.Name() == lockFile:
			legacy = true
		}
	}
	if legacy && !slices.Contains(names, "default") {
		names = append(names, "default")
		slices.Sort(names)
	}
	var states []State
	for _, name := range names {
		state, readErr := status(root, name)
		switch {
		case readErr == nil:
			states = append(states, state)
		case !errors.Is(readErr, os.ErrNotExist):
			err = errors.Join(err, fmt.Errorf("read monitor %q: %w", name, readErr))
		}
	}
	return states, err
}

func readStored(root string) (storedState, error) {
	data, err := os.ReadFile(filepath.Join(root, stateFile))
	if err != nil {
		return storedState{}, err
	}
	var state storedState
	err = json.Unmarshal(data, &state)
	return state, err
}

func lockMonitor(dir, name string) (*os.File, error) {
	lock, err := os.OpenFile(filepath.Join(dir, lockFile), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lock.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, fmt.Errorf("monitor %q is already running or being changed", name)
		}
		return nil, err
	}
	return lock, nil
}

func trySharedLock(lock *os.File) (bool, error) {
	err := unix.Flock(int(lock.Fd()), unix.LOCK_SH|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func status(root, name string) (State, error) {
	dir, err := directory(root, name)
	if err != nil {
		return State{}, err
	}
	state, err := readStored(dir)
	if err != nil {
		return State{}, err
	}
	state.Name = name
	if !state.Running {
		return state.State, nil
	}
	lock, err := os.Open(filepath.Join(dir, lockFile))
	if err != nil {
		return State{}, err
	}
	defer lock.Close()
	available, err := trySharedLock(lock)
	if err != nil {
		return State{}, err
	}
	state.Running = !available
	return state.State, nil
}

func remove(root, name string, confirm func() error) error {
	dir, err := directory(root, name)
	if err != nil {
		return err
	}
	lock, err := lockMonitor(dir, name)
	if err != nil {
		return err
	}
	defer lock.Close()
	path := filepath.Join(dir, stateFile)
	if _, err := os.Lstat(path); err != nil {
		return err
	}
	if err := confirm(); err != nil {
		return err
	}
	// Retain the lock inode so a later start and a concurrent command use the same lock.
	return os.Remove(path)
}

func save(root string, state storedState) error {
	file, err := os.CreateTemp(root, "watch-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(state)
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(root, stateFile))
}
