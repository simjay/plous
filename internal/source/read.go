package source

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

type ReadRequest struct {
	Target        string `json:"target,omitempty"`
	Result        string `json:"result,omitempty"`
	Resource      string `json:"resource,omitempty"`
	Field         string `json:"field,omitempty"`
	Other         string `json:"other,omitempty"`
	Operation     string `json:"operation"`
	Path          string `json:"path,omitempty"`
	Query         string `json:"query,omitempty"`
	Glob          string `json:"glob,omitempty"`
	Regex         bool   `json:"regex,omitempty"`
	CaseSensitive *bool  `json:"case_sensitive,omitempty"`
	Identity      string `json:"identity,omitempty"`
	Cursor        string `json:"cursor,omitempty"`
	Offset        int    `json:"offset,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	MaxBytes      int    `json:"max_bytes,omitempty"`
}

type sourceEntry struct {
	Mode      fs.FileMode `json:"-"`
	Path      string      `json:"path"`
	Directory bool        `json:"directory"`
	Size      int64       `json:"size"`
	Modified  time.Time   `json:"modified"`
}
type sourceMatch struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Text      string `json:"text"`
	Identity  string `json:"identity"`
	Offset    int    `json:"offset"`
	Truncated bool   `json:"truncated"`
}
type sourcePage struct {
	Complete     bool           `json:"complete"`
	Notice       string         `json:"notice,omitempty"`
	Continuation map[string]any `json:"continuation,omitempty"`
	Data         sourcePageData `json:"data"`
}
type sourcePageData struct {
	Text       string        `json:"text,omitempty"`
	Identity   string        `json:"identity"`
	TotalBytes int           `json:"total_bytes,omitempty"`
	NextOffset int           `json:"next_offset,omitempty"`
	Omitted    int           `json:"omitted,omitempty"`
	Entries    []sourceEntry `json:"entries,omitempty"`
	Matches    []sourceMatch `json:"matches,omitempty"`
}

func sourcePath(name string) bool {
	if name == "" || name == "." {
		return true
	}
	if !fs.ValidPath(name) || len(name) > 1024 || strings.ContainsAny(name, "\\\x00\r\n") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		lower := strings.ToLower(part)
		if strings.HasPrefix(part, ".") || lower == "node_modules" || lower == "credentials" || lower == "credentials.json" || lower == "kubeconfig" || lower == "config.json" || lower == "id_rsa" || lower == "id_ed25519" || strings.Contains(lower, "secret") || strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") || strings.HasSuffix(lower, ".p12") || strings.HasSuffix(lower, ".pfx") || strings.HasSuffix(lower, ".kubeconfig") {
			return false
		}
	}
	return true
}

// Every component is opened relative to the previous directory descriptor with
// O_NOFOLLOW. Renaming a symlink between validation and open cannot escape the
// captured root or redirect a permitted name to private content inside it.
func openSourcePath(root *os.File, name string, directory bool) (*os.File, error) {
	if !sourcePath(name) {
		return nil, errors.New("source path is outside the permitted read policy")
	}
	return openTreePath(root, name, directory)
}

func openTreePath(root *os.File, name string, directory bool) (*os.File, error) {
	if name != "" && (!fs.ValidPath(name) || strings.ContainsAny(name, "\\\x00\r\n")) {
		return nil, errors.New("invalid source path")
	}
	if name == "" || name == "." {
		fd, err := unix.Dup(int(root.Fd()))
		if err != nil {
			return nil, err
		}
		return os.NewFile(uintptr(fd), "source-root"), nil
	}
	parts := strings.Split(name, "/")
	parent := root
	var owned *os.File
	defer func() {
		if owned != nil {
			owned.Close()
		}
	}()
	for i, part := range parts {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if i < len(parts)-1 || directory {
			flags |= unix.O_DIRECTORY
		}
		fd, err := unix.Openat(int(parent.Fd()), part, flags, 0)
		if err != nil {
			return nil, fmt.Errorf("source path is unavailable or is a symbolic link: %w", err)
		}
		file := os.NewFile(uintptr(fd), part)
		if i == len(parts)-1 {
			info, err := file.Stat()
			if err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
				file.Close()
				return nil, errors.New("source path must be a regular file or directory")
			}
			return file, nil
		}
		if owned != nil {
			owned.Close()
		}
		owned = file
		parent = file
	}
	return nil, errors.New("invalid source path")
}

func openSourceRoot(path string) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("source connection has no registered absolute local root")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != filepath.Clean(path) {
		return nil, errors.New("registered source root is unavailable or contains a symbolic link")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, errors.New("registered source root is unavailable")
	}
	for _, component := range strings.Split(strings.TrimPrefix(canonical, "/"), "/") {
		if component == "" {
			continue
		}
		next, e := unix.Openat(fd, component, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
		unix.Close(fd)
		if e != nil {
			return nil, errors.New("registered source root changed or contains a symbolic link")
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), "source-root"), nil
}

func sourceText(root *os.File, path string) ([]byte, string, error) {
	f, err := openSourcePath(root, path, false)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, "", errors.New("source read requires a regular text file")
	}
	data, err := io.ReadAll(io.LimitReader(f, sourceFileBytes+1))
	if err != nil {
		return nil, "", errors.New("source file read failed")
	}
	if len(data) > sourceFileBytes {
		return nil, "", errors.New("source file exceeds 2 MiB")
	}
	if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return nil, "", errors.New("binary source files are unavailable")
	}
	if err := checkSourceContent(path, string(data)); err != nil {
		return nil, "", err
	}
	return data, fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// ReadScope captures the registered source and profile boundary for an AI read.
// This reader has stricter permissions than snapshots and explicitly saved edits.
type ReadScope struct {
	ScanRules                    func() ([]byte, error)
	View                         *ReadView
	Session                      *ReadSession
	Root, ProfileRoot            string
	ConnectionID, ConnectionName string
}

var ErrReadDenied = errors.New("source read denied")

func denySourceRead(err error) error { return errors.Join(ErrReadDenied, err) }

const (
	MaxReadResults       = 100
	MaxFileReadBytes     = 16384
	DefaultReadResults   = 50
	DefaultFileReadBytes = 4096
)

func validateFileRead(c *ReadRequest) error {
	if err := validateReadArguments(*c); err != nil {
		return err
	}
	if c.Operation != "read" && c.Operation != "list" && c.Operation != "search" {
		return denySourceRead(errors.New("expected a file read operation"))
	}
	if !sourcePath(c.Path) || c.Offset < 0 || c.Limit < 0 || c.Limit > MaxReadResults || c.MaxBytes < 0 || c.MaxBytes > MaxFileReadBytes || len(c.Query) > 1024 || len(c.Cursor) > 4096 || len(c.Identity) > 64 {
		return denySourceRead(errors.New("invalid source read arguments"))
	}
	if c.Operation == "read" && c.Path == "." {
		return denySourceRead(errors.New("source read requires a file path"))
	}

	if c.Limit == 0 {
		c.Limit = DefaultReadResults
	}
	if c.MaxBytes == 0 {
		c.MaxBytes = DefaultFileReadBytes
	}
	return nil
}

func validateReadScope(scope ReadScope) error {
	if scope.ProfileRoot == "" {
		return denySourceRead(errors.New("source reads require a captured profile boundary"))
	}
	profile, err := filepath.EvalSymlinks(scope.ProfileRoot)
	if err != nil {
		return err
	}
	roots := []string{scope.Root}
	if scope.View != nil {
		config := scope.View.inventory.Source.Config
		if config.Repository == "" {
			root, err := filepath.Abs(config.Path)
			if err != nil || filepath.Clean(root) != filepath.Clean(scope.Root) {
				return denySourceRead(errors.New("source snapshot does not match the captured root"))
			}
			roots = append(roots, root)
		} else if u, err := url.Parse(config.Repository); err == nil && u.Scheme == "file" {
			roots = append(roots, u.Path)
		} else if filepath.IsAbs(config.Repository) {
			roots = append(roots, config.Repository)
		}
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		if canonical, err := filepath.EvalSymlinks(root); err == nil {
			root = canonical
		}
		if !filepath.IsAbs(root) {
			return denySourceRead(errors.New("source root must be absolute"))
		}

		for _, pair := range [][2]string{{profile, root}, {root, profile}} {
			relative, err := filepath.Rel(pair[0], pair[1])
			if err != nil || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return denySourceRead(errors.New("registered source overlaps the Plous profile; saved history is available only through recall"))
			}
		}
	}
	return nil
}

func readSourcePage(ctx context.Context, scope ReadScope, c ReadRequest) (*sourcePage, error) {
	if err := validateFileRead(&c); err != nil {
		return nil, err
	}
	if err := validateReadScope(scope); err != nil {
		return nil, err
	}
	if scope.Session != nil {
		return scope.Session.read(ctx, scope, c)
	}
	// Single reads need no retained host session. Paging a search does.
	session := NewReadSession()
	defer session.Close()
	return session.read(ctx, scope, c)
}

func sourceReader(scope ReadScope, policy *scanPolicy) (fileReader, func(), error) {
	if scope.View != nil && scope.View.inventory != nil && scope.View.inventory.Source.Config.Repository != "" {
		return acquiredFiles{source: scope.View.inventory.Source, scan: policy}, func() {}, nil
	}
	root, err := openSourceRoot(scope.Root)
	if err != nil {
		return nil, nil, err
	}
	return localFiles{root: root, scan: policy}, func() { root.Close() }, nil
}

type fileReader interface {
	allowed(string, bool) (bool, error)
	text(string) ([]byte, string, error)
	directory(string) ([]sourceEntry, int, error)
}
type localFiles struct {
	root *os.File
	scan *scanPolicy
}

func (f localFiles) text(path string) ([]byte, string, error) {
	return sourceText(f.root, path)
}
func (f localFiles) allowed(name string, directory bool) (bool, error) {
	if f.scan.excludes(name, directory) {
		return false, nil
	}
	return allowedLocalPath(f.root, name, directory)
}
func (f localFiles) directory(path string) ([]sourceEntry, int, error) {
	allowed, err := f.allowed(path, true)
	if err != nil {
		return nil, 0, err
	}
	if !allowed {
		return nil, 0, nil
	}
	if !sourcePath(path) {
		return nil, 0, denySourceRead(errors.New("source path is unavailable to the assistant"))
	}
	omitted := 0
	entries, err := treeDirectory(f.root, path, func(name string, directory bool) (bool, error) {
		if !sourcePath(name) {
			omitted++
			return false, nil
		}
		return f.allowed(name, directory)
	})
	kept := entries[:0]
	for _, entry := range entries {
		if !entry.Directory && !entry.Mode.IsRegular() {
			omitted++
			continue
		}
		kept = append(kept, entry)
	}
	return kept, omitted, err
}

func readFilePage(ctx context.Context, files fileReader, id string, c ReadRequest) (*sourcePage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := &sourcePage{Complete: true}
	switch c.Operation {
	case "read":
		data, identity, err := files.text(c.Path)
		if err != nil {
			return nil, err
		}
		if c.Identity != "" && c.Identity != identity {
			return nil, denySourceRead(errors.New("source changed since the previous read; restart at offset 0"))
		}
		if c.Offset > len(data) || c.Offset < len(data) && !utf8.RuneStart(data[c.Offset]) {
			return nil, errors.New("source offset is not a valid text position")
		}
		end := min(len(data), c.Offset+c.MaxBytes)
		for end > c.Offset && end < len(data) && !utf8.RuneStart(data[end]) {
			end--
		}
		if end == c.Offset && end < len(data) {
			return nil, errors.New("max_bytes must fit the next UTF-8 character")
		}
		result.Data = sourcePageData{Text: string(data[c.Offset:end]), Identity: identity, TotalBytes: len(data), NextOffset: end}
		result.Complete = end == len(data)
		if end < len(data) {
			result.Continuation = map[string]any{"offset": end, "identity": identity}
		}
	case "list":
		entries, omitted, err := files.directory(c.Path)
		if err != nil {
			return nil, err
		}
		if c.Glob != "" {
			match, err := fileGlob(c.Glob)
			if err != nil {
				return nil, err
			}
			filtered := entries[:0]
			for _, entry := range entries {
				if match(entry.Path) {
					filtered = append(filtered, entry)
				}
			}
			entries = filtered
		}
		identity := fmt.Sprintf("%x", sha256.Sum256(sourceJSON([]any{id, c.Path, c.Glob, entries})))
		if c.Identity != "" && c.Identity != identity {
			return nil, denySourceRead(errors.New("source directory changed; restart at offset 0"))
		}
		if c.Offset > len(entries) {
			return nil, errors.New("source list offset exceeds entries")
		}
		end := min(len(entries), c.Offset+c.Limit)
		result.Data = sourcePageData{Entries: entries[c.Offset:end], NextOffset: end, Identity: identity, Omitted: omitted}
		result.Complete = end == len(entries)
		if end < len(entries) {
			result.Continuation = map[string]any{"offset": end, "identity": identity}
		}
	}
	return result, nil
}

func sourceJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

func utf8Prefix(text string, size int) int {
	end := min(len(text), size)
	for end > 0 && end < len(text) && !utf8.RuneStart(text[end]) {
		end--
	}
	return end
}
