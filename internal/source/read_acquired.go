package source

import (
	"crypto/sha256"
	"errors"
	"fmt"
)

type acquiredFiles struct {
	source *Snapshot
	scan   *scanPolicy
}

func (f acquiredFiles) text(name string) ([]byte, string, error) {
	if !sourcePath(name) {
		return nil, "", denySourceRead(errors.New("source path is unavailable to the assistant"))
	}
	data, err := f.source.ReadFile(name)
	if err != nil {
		return nil, "", err
	}
	if len(data) > sourceFileBytes {
		return nil, "", errors.New("source file exceeds 2 MiB")
	}
	if err := checkSourceContent(name, string(data)); err != nil {
		return nil, "", err
	}
	return data, fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func (f acquiredFiles) allowed(name string, directory bool) (bool, error) {
	if f.scan.excludes(name, directory) {
		return false, nil
	}
	if f.source.git == nil {
		return true, nil
	}
	return f.source.git.allowed(name, directory, nil)
}
func (f acquiredFiles) directory(name string) ([]sourceEntry, int, error) {
	if !sourcePath(name) {
		return nil, 0, denySourceRead(errors.New("source path is unavailable to the assistant"))
	}
	allowed, err := f.allowed(name, true)
	if err != nil {
		return nil, 0, err
	}
	if !allowed {
		return nil, 0, nil
	}
	entries, err := f.source.inputDirectory(name, f.allowed)
	if err != nil {
		return nil, 0, err
	}
	rows := entries[:0]
	omitted := 0
	for _, entry := range entries {
		if !sourcePath(entry.Path) || (!entry.Directory && !entry.Mode.IsRegular()) {
			omitted++
			continue
		}
		rows = append(rows, entry)
	}
	return rows, omitted, nil
}
