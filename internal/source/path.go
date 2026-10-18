package source

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveDirectory records relative paths against a visible base before saving.
func ResolveDirectory(value, base string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '\'' && value[len(value)-1] == '\'' || value[0] == '"' && value[len(value)-1] == '"') {
		value = value[1 : len(value)-1]
	}
	if value == "" {
		return "", errors.New("choose a local directory")
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("directory must contain one line")
	}
	if strings.HasPrefix(value, "~") {
		if value != "~" && !strings.HasPrefix(value, "~/") {
			return "", errors.New("use ~/ for your home directory, or an absolute path")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if value == "~" {
			value = home
		} else {
			value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
	}
	if !filepath.IsAbs(value) {
		if base == "" {
			var err error
			base, err = os.Getwd()
			if err != nil {
				return "", err
			}
		}
		value = filepath.Join(base, value)
	}
	return filepath.Abs(value)
}

func CheckDirectory(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return errors.New("directory does not exist; edit the path or browse folders")
	}
	if err != nil {
		return fmt.Errorf("cannot open directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("this is a file; choose its containing directory")
	}
	return nil
}
