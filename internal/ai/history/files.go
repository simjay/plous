package history

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func newID() string {
	var value [16]byte
	// crypto/rand.Read cannot return an error on supported Go platforms.
	rand.Read(value[:])
	return hex.EncodeToString(value[:])
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
