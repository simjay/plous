package source

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type Config struct {
	ScanRules                                func() ([]byte, error) `json:"-"`
	Path, Repository, Revision, Subdirectory string
	BrowsePath                               string
	RepositoryRoot                           bool
}

func (c Config) Fingerprint() string {
	identity := c.Path + "\x00" + c.Repository + "\x00" + c.Revision + "\x00" + c.Subdirectory
	if c.RepositoryRoot {
		identity += "\x00repository-root"
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
}
func (c Config) Validate() error {
	if c.Repository == "" && c.Path == "" {
		return errors.New("enter a directory or Git repository")
	}
	if strings.ContainsAny(c.Repository+c.Revision+c.Path+c.Subdirectory+c.BrowsePath, "\x00\r\n") {
		return errors.New("source fields must contain one line")
	}
	if c.Repository != "" {
		if strings.HasPrefix(c.Repository, "-") || strings.HasPrefix(c.Revision, "-") {
			return errors.New("repository and revision cannot start with a dash")
		}
		if strings.Contains(c.Repository, "://") {
			u, err := url.Parse(c.Repository)
			if err != nil || u.RawQuery != "" || u.Fragment != "" {
				return errors.New("use a repository URL without query parameters or fragments")
			}
			if u.Scheme != "https" && u.Scheme != "ssh" && u.Scheme != "file" {
				return errors.New("use an HTTPS or SSH Git repository")
			}
			if u.User != nil && (u.Scheme != "ssh" || hasPassword(u)) {
				return errors.New("use HTTPS netrc credentials or SSH agent authentication; saved URLs cannot contain credentials")
			}
		} else if strings.Contains(c.Repository, "::") {
			return errors.New("Git remote helpers are not supported")
		}
	}
	if c.BrowsePath != "" && !fs.ValidPath(c.BrowsePath) {
		return errors.New("starting path must stay inside the repository")
	}
	if c.Subdirectory != "" && !fs.ValidPath(c.Subdirectory) {
		return errors.New("repository directory must be a relative path without parent traversal")
	}
	return nil
}
func hasPassword(u *url.URL) bool { _, ok := u.User.Password(); return ok }

type Snapshot struct {
	Config             Config
	Identity, Revision string
	Observed           time.Time
	Files              map[string][]byte
	entries            map[string]sourceEntry
	git                *gitSource
	Gaps               []string
	scan               *scanPolicy
}

func Load(ctx context.Context, config Config) (*Snapshot, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	s := &Snapshot{Config: config, Observed: time.Now().UTC(), entries: map[string]sourceEntry{}}
	var err error
	s.scan, err = loadScanPolicy(config.ScanRules)
	if err != nil {
		return nil, err
	}
	if config.Repository != "" {
		err = s.loadGit(ctx)
	} else {
		err = s.loadLocal(ctx)
	}
	if err != nil {
		return nil, err
	}
	s.identify()
	return s, nil
}

func (s *Snapshot) FileCount() int {
	count := len(s.entries)
	for name := range s.Files {
		if _, exists := s.entries[name]; !exists {
			count++
		}
	}
	return count
}

func (s *Snapshot) Paths() []string {
	names := make([]string, 0, len(s.entries)+len(s.Files))
	for name := range s.entries {
		names = append(names, name)
	}
	for name := range s.Files {
		if _, ok := s.entries[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
func (s *Snapshot) loadLocal(ctx context.Context) error {
	directory, err := ResolveDirectory(s.Config.Path, "")
	if err != nil {
		return err
	}
	if err := CheckDirectory(directory); err != nil {
		return err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return err
	}
	s.Config.Path = directory
	root, err := openSourceRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	work := newSourceWork(ctx)
	var visit func(string) error
	visit = func(directory string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := s.inputDirectory(directory, func(name string, directory bool) (bool, error) {
			if s.scan.excludes(name, directory) {
				return false, nil
			}
			return allowedLocalPath(root, name, directory)
		})
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Directory {
				if err := visit(entry.Path); err != nil {
					return err
				}
				continue
			}
			if err := work.retain(128 + len(entry.Path)); err != nil {
				return err
			}
			if entry.Mode&fs.ModeSymlink != 0 {
				s.Gaps = append(s.Gaps, entry.Path+": symbolic link omitted")
				continue
			}
			if !entry.Mode.IsRegular() {
				s.Gaps = append(s.Gaps, entry.Path+": special file omitted")
				continue
			}
			if entry.Size > maxFile {
				s.Gaps = append(s.Gaps, entry.Path+": exceeds 2 MiB")
			}
			s.entries[entry.Path] = entry
		}
		return nil
	}
	return visit(".")
}

func (s *Snapshot) identify() {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%s:", s.Config.Fingerprint(), s.Revision)
	for _, name := range s.Paths() {
		if data, ok := s.Files[name]; ok {
			fmt.Fprintf(h, "%d:%s:%d:", len(name), name, len(data))
			h.Write(data)
		} else {
			entry := s.entries[name]
			fmt.Fprintf(h, "%s:%d:%d:", name, entry.Size, entry.Modified.UnixNano())
		}
	}
	s.Identity = fmt.Sprintf("%x", h.Sum(nil))
}

// RefreshFile replaces only metadata after an explicit successful editor save.
func (s *Snapshot) RefreshFile(name string) (*Snapshot, error) {
	next := *s
	next.entries = maps.Clone(s.entries)
	root, err := os.OpenRoot(s.Config.Path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := regularPath(root, name)
	if err != nil {
		return nil, err
	}
	if next.entries == nil {
		next.entries = map[string]sourceEntry{}
	}
	next.Files = nil
	next.entries[name] = sourceEntry{Path: name, Size: info.Size(), Modified: info.ModTime()}
	next.Observed = time.Now().UTC()
	next.identify()
	return &next, nil
}

func (s *Snapshot) Has(name string) bool {
	_, entry := s.entries[name]
	_, captured := s.Files[name]
	return entry || captured
}

// ReadFile opens a selected source file; discovery does not cache its body.
func (s *Snapshot) ReadFile(name string) ([]byte, error) {
	data, err := s.readBytes(name)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return nil, fmt.Errorf("binary source file unavailable: %s", name)
	}
	return data, nil
}
func (s *Snapshot) readBytes(name string) ([]byte, error) {
	defer runtime.KeepAlive(s)
	if !inputPath(name) {
		return nil, fmt.Errorf("invalid source file: %s", name)
	}
	if data, ok := s.Files[name]; ok {
		return data, nil
	}
	if s.Files != nil {
		return nil, fmt.Errorf("source file unavailable: %s", name)
	}
	var reader io.ReadCloser
	var err error
	if s.git != nil {
		reader, err = s.git.open(name)
	} else {
		root, openErr := openSourceRoot(s.Config.Path)
		if openErr != nil {
			return nil, openErr
		}
		reader, err = openTreePath(root, name, false)
		root.Close()
	}
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFile {
		return nil, fmt.Errorf("source file exceeds 2 MiB: %s", name)
	}
	return data, nil
}
