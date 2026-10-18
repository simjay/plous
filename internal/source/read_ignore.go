package source

import (
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

func allowedLocalPath(root *os.File, name string, directory bool) (bool, error) {
	var patterns []gitignore.Pattern
	parts := strings.Split(name, "/")
	for n := range parts {
		parent := strings.Join(parts[:n], "/")
		f, err := openTreePath(root, path.Join(parent, ".gitignore"), false)
		if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, unix.ELOOP) {
			return false, err
		}
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(f, sourceFileBytes+1))
			f.Close()
			if readErr != nil || len(data) > sourceFileBytes {
				return false, errors.New("cannot read bounded source ignore rules")
			}
			patterns = append(patterns, parseIgnore(data, parts[:n])...)
		}
		if gitignore.NewMatcher(patterns).Match(parts[:n+1], n < len(parts)-1 || directory) {
			return false, nil
		}
	}
	return true, nil
}
