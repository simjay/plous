package source

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	netrc "github.com/jdx/go-netrc"
	sshagent "github.com/xanzy/ssh-agent"
)

type gitSource struct {
	tree    *object.Tree
	prefix  string
	store   *gitStore
	cleanup runtime.Cleanup
}

// The store lock keeps shutdown and garbage collection from deleting objects
// while a selected blob is still being read.
type gitStore struct {
	mu        sync.RWMutex
	directory string
	closed    bool
}

func (s *gitStore) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.directory != "" {
		return os.RemoveAll(s.directory)
	}
	return nil
}

type gitFile struct {
	io.ReadCloser
	release func()
	once    sync.Once
	err     error
}

func (f *gitFile) Close() error {
	f.once.Do(func() { f.err = f.ReadCloser.Close(); f.release() })
	return f.err
}
func (g *gitSource) open(name string) (io.ReadCloser, error) {
	g.store.mu.RLock()
	if g.store.closed {
		g.store.mu.RUnlock()
		return nil, errors.New("acquired Git source has closed; open or refresh it")
	}
	file, err := g.tree.File(g.prefix + name)
	if err != nil {
		g.store.mu.RUnlock()
		return nil, err
	}
	if file.Mode != filemode.Regular && file.Mode != filemode.Executable {
		g.store.mu.RUnlock()
		return nil, errors.New("Git input is not a regular file")
	}
	reader, err := file.Reader()
	if err != nil {
		g.store.mu.RUnlock()
		return nil, err
	}
	return &gitFile{ReadCloser: reader, release: g.store.mu.RUnlock}, nil
}

// Cleanup waits for active blob readers without retaining the source handle.
func (s *Snapshot) Cleanup() func() error {
	if s.git != nil {
		return s.git.store.close
	}
	return func() error { return nil }
}

func (s *Snapshot) loadGit(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	endpoint, err := transport.NewEndpoint(s.Config.Repository)
	if err != nil {
		return err
	}
	var repository *git.Repository
	var acquired string
	complete := false
	defer func() {
		if !complete && acquired != "" {
			os.RemoveAll(acquired)
		}
	}()
	revision := s.Config.Revision
	if revision == "" {
		revision = "HEAD"
	}
	if endpoint.Protocol == "file" {
		// Read objects directly. go-git's file transport otherwise starts git-upload-pack.
		repository, err = git.PlainOpen(endpoint.Path)
	} else {
		if endpoint.Protocol != "https" && endpoint.Protocol != "ssh" {
			return errors.New("unsupported Git transport")
		}
		directory, e := os.MkdirTemp("", "plous-source-git-")
		if e != nil {
			return e
		}
		acquired = directory
		repository, err = git.PlainInit(directory, true)
		if err != nil {
			return err
		}
		var auth transport.AuthMethod
		if endpoint.Protocol == "ssh" {
			agent, connection, err := sshagent.New()
			if err != nil {
				return fmt.Errorf("Git SSH requires an authenticated SSH agent: %w", err)
			}
			defer connection.Close()
			stop := context.AfterFunc(ctx, func() { connection.Close() })
			defer stop()
			user := endpoint.User
			if user == "" {
				user = "git"
			}
			auth = &gitssh.PublicKeysCallback{User: user, Callback: agent.Signers}
		}

		if endpoint.Protocol == "https" {
			auth, err = httpsAuth(endpoint.Host)
			if err != nil {
				return err
			}
		}
		_, err = repository.CreateRemote(&config.RemoteConfig{Name: "source", URLs: []string{s.Config.Repository}})
		if err != nil {
			return err
		}
		err = repository.FetchContext(ctx, &git.FetchOptions{RemoteName: "source", Auth: auth, Depth: 1, Tags: git.NoTags,
			RefSpecs: []config.RefSpec{config.RefSpec("+" + revision + ":refs/plous/source")}})
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return err
		}
		revision = "refs/plous/source"
	}
	if err != nil {
		return err
	}
	hash, err := repository.ResolveRevision(plumbing.Revision(revision))
	if err != nil {
		return err
	}
	commit, err := repository.CommitObject(*hash)
	if err != nil {
		return err
	}
	s.Revision = commit.Hash.String()
	tree, err := commit.Tree()
	if err != nil {
		return err
	}
	prefix := strings.Trim(s.Config.Subdirectory, "/")
	if s.Config.RepositoryRoot {
		prefix = ""
	}
	if prefix == "." {
		prefix = ""
	}
	if prefix != "" {
		prefix += "/"
	}
	s.git = &gitSource{tree: tree, prefix: prefix, store: &gitStore{directory: acquired}}
	if acquired != "" {
		s.git.cleanup = runtime.AddCleanup(s.git, func(store *gitStore) { _ = store.close() }, s.git.store)
	}
	work := newSourceWork(ctx)
	ignoreCache := map[string][]gitignore.Pattern{}
	var visit func(string) error
	visit = func(directory string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := s.inputDirectory(directory, func(name string, directory bool) (bool, error) {
			if s.scan.excludes(name, directory) {
				return false, nil
			}
			return s.git.allowed(name, directory, ignoreCache)
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
			if !entry.Mode.IsRegular() {
				s.Gaps = append(s.Gaps, entry.Path+": symlink or submodule omitted")
				continue
			}
			if err := work.retain(128 + len(entry.Path)); err != nil {
				return err
			}
			if entry.Size > maxFile {
				s.Gaps = append(s.Gaps, entry.Path+": exceeds 2 MiB")
			}
			s.entries[entry.Path] = entry
		}
		return nil
	}
	if err := visit("."); err != nil {
		return err
	}
	complete = true
	return nil
}

func (g *gitSource) allowed(name string, directory bool, cache map[string][]gitignore.Pattern) (bool, error) {
	g.store.mu.RLock()
	defer g.store.mu.RUnlock()
	if g.store.closed {
		return false, errors.New("acquired Git source has closed")
	}
	if cache == nil {
		cache = map[string][]gitignore.Pattern{}
	}
	parts := strings.Split(name, "/")
	var patterns []gitignore.Pattern
	for n, part := range parts {
		if part == ".git" {
			return false, nil
		}
		parent := strings.Join(parts[:n], "/")
		local, ok := cache[parent]
		if !ok {
			file, err := g.tree.File(g.prefix + path.Join(parent, ".gitignore"))
			if err != nil && !errors.Is(err, object.ErrFileNotFound) {
				return false, err
			}
			if err == nil && (file.Mode == filemode.Regular || file.Mode == filemode.Executable) {
				if file.Size > maxFile {
					return false, errors.New("source ignore rules exceed 2 MiB")
				}
				data, err := file.Contents()
				if err != nil {
					return false, err
				}
				local = parseIgnore([]byte(data), parts[:n])
			}
			cache[parent] = local
		}
		patterns = append(patterns, local...)
		if gitignore.NewMatcher(patterns).Match(parts[:n+1], n < len(parts)-1 || directory) {
			return false, nil
		}
	}
	return true, nil
}

// Read only host-specific netrc entries; never invoke Git credential helpers.
func httpsAuth(host string) (transport.AuthMethod, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	filename := ".netrc"
	if runtime.GOOS == "windows" {
		filename = "_netrc"
	}
	file, err := os.Open(filepath.Join(home, filename))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("cannot read Git HTTPS credentials")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("Git netrc must be a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("cannot read Git HTTPS credentials")
	}
	config, err := netrc.ParseString(string(data))
	if err != nil {
		return nil, errors.New("cannot parse Git netrc credentials")
	}
	machine := config.Machine(host)
	if machine == nil {
		return nil, nil
	}
	return &githttp.BasicAuth{Username: machine.Get("login"), Password: machine.Get("password")}, nil
}
