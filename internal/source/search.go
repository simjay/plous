package source

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"path"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

// ReadSession owns continuations for one host operation, never a global file cache.
type ReadSession struct {
	mu       sync.Mutex
	searches map[string]*fileSearch
	closed   bool
}

func NewReadSession() *ReadSession { return &ReadSession{searches: map[string]*fileSearch{}} }
func (s *ReadSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.searches = nil
}

type searchDirectory struct {
	entries []sourceEntry
	next    int
}
type fileSearch struct {
	key                           string
	scan                          *scanPolicy
	stack                         []searchDirectory
	file                          *sourceEntry
	identity                      string
	line, offset, omitted         int
	started, complete, incomplete bool
}

func fileGlob(pattern string) (func(string) bool, error) {
	if len(pattern) > 1024 || strings.ContainsAny(pattern, "\\\x00\r\n") {
		return nil, denySourceRead(errors.New("invalid filename glob"))
	}
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, denySourceRead(errors.New("invalid filename glob"))
	}
	return func(name string) bool {
		if pattern == "" {
			return true
		}
		if !strings.Contains(pattern, "/") {
			name = path.Base(name)
		}
		matched, _ := path.Match(pattern, name)
		return matched
	}, nil
}

func (s *ReadSession) read(ctx context.Context, scope ReadScope, c ReadRequest) (*sourcePage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.closed {
		return nil, errors.New("source read operation has closed")
	}
	var policy *scanPolicy
	var err error
	if c.Operation != "read" {
		if pending := s.searches[c.Cursor]; c.Cursor != "" && pending != nil {
			policy = pending.scan
		} else {
			policy, err = loadScanPolicy(scope.ScanRules)
			if err != nil {
				return nil, err
			}
		}
	}
	reader, close, err := sourceReader(scope, policy)
	if err != nil {
		return nil, err
	}
	defer close()
	if c.Operation != "search" {
		return readFilePage(ctx, reader, scope.ConnectionID, c)
	}
	glob, err := fileGlob(c.Glob)
	if err != nil {
		return nil, err
	}
	pattern := c.Query
	if !c.Regex {
		pattern = regexp.QuoteMeta(pattern)
	}
	if c.CaseSensitive != nil && !*c.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	matcher, err := regexp.Compile(pattern)
	if err != nil {
		return nil, denySourceRead(errors.New("invalid search regular expression"))
	}
	identity := scope.Root
	if scope.View != nil && scope.View.inventory.Source.Config.Repository != "" {
		identity = scope.View.inventory.Source.Identity
	}
	key := string(sourceJSON([]any{scope.ConnectionID, identity, c.Path, c.Query, c.Glob, c.Regex, c.CaseSensitive}))
	token := c.Cursor
	search := s.searches[token]
	if token != "" && (search == nil || search.key != key) {
		return nil, denySourceRead(errors.New("search continuation is invalid for this source, query or operation"))
	}
	if search == nil {
		token = rand.Text()
		search = &fileSearch{key: key, scan: policy}
		s.searches[token] = search
	}
	result, err := search.page(ctx, reader, c, glob, matcher)
	retained := 0
	for _, pending := range s.searches {
		retained += pending.retained()
	}
	if retained > maxSource {
		err = denySourceRead(errors.New("source search continuations exceed 32 MiB; finish existing searches first"))
	}
	if err != nil || search.complete {
		delete(s.searches, token)
	}
	if err != nil {
		return nil, err
	}
	result.Data.Identity = token
	if !search.complete {
		result.Continuation = map[string]any{"cursor": token}
	}
	return result, nil
}

func (s *fileSearch) page(ctx context.Context, reader fileReader, c ReadRequest, glob func(string) bool, matcher *regexp.Regexp) (*sourcePage, error) {
	out := &sourcePage{}
	work := newSourceWork(ctx)
	if !s.started {
		entries, omitted, err := reader.directory(c.Path)
		if err != nil {
			// An exact file scope avoids walking its parent or unrelated directories.
			if c.Path == "" || c.Path == "." {
				return nil, err
			}
			allowed, allowErr := reader.allowed(c.Path, false)
			if allowErr != nil {
				return nil, allowErr
			}
			if !allowed {
				s.complete = true
				out.Complete = true
				return out, nil
			}
			if _, _, fileErr := reader.text(c.Path); fileErr != nil {
				return nil, err
			}
			entries = []sourceEntry{{Path: c.Path}}
		}
		s.stack = []searchDirectory{{entries: entries}}
		s.omitted += omitted
		s.incomplete = omitted > 0
		s.started = true
	}
	for len(out.Data.Matches) < c.Limit {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if work.exhausted() {
			out.Notice = "search work limit reached; continue with the returned cursor"
			break
		}
		if s.file == nil {
			if len(s.stack) == 0 {
				s.complete = true
				break
			}
			directory := &s.stack[len(s.stack)-1]
			if directory.next == len(directory.entries) {
				s.stack = s.stack[:len(s.stack)-1]
				continue
			}
			entry := directory.entries[directory.next]
			directory.next++
			work.entries++
			if entry.Directory {
				entries, omitted, err := reader.directory(entry.Path)
				if err != nil {
					s.incomplete = true
					s.omitted++
					continue
				}
				s.omitted += omitted
				s.incomplete = s.incomplete || omitted > 0
				if s.retained()+entryBytes(entries) > maxSource {
					return nil, denySourceRead(errors.New("source traversal metadata exceeds 32 MiB; narrow the search path"))
				}
				s.stack = append(s.stack, searchDirectory{entries: entries})
				continue
			}
			if !glob(entry.Path) {
				continue
			}
			s.file, s.line, s.offset, s.identity = &entry, 0, 0, ""
		}
		estimated := int(min(s.file.Size, int64(sourceFileBytes+1)))
		if work.bytes > 0 && work.bytes+estimated > maxSource {
			out.Notice = "search work limit reached; continue with the returned cursor"
			break
		}
		data, identity, err := reader.text(s.file.Path)
		work.bytes += max(estimated, len(data))
		if err != nil {
			if s.identity != "" {
				return nil, denySourceRead(errors.New("source file changed or became unavailable during search; start a new search"))
			}
			s.incomplete = true
			s.omitted++
			s.file = nil
			continue
		}
		if s.identity != "" && s.identity != identity {
			return nil, denySourceRead(errors.New("source file changed during search; start a new search"))
		}
		s.identity = identity
		for s.offset < len(data) && len(out.Data.Matches) < c.Limit {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			lineOffset := s.offset
			end := bytes.IndexByte(data[s.offset:], '\n')
			if end < 0 {
				end = len(data) - s.offset
			}
			line := string(data[s.offset : s.offset+end])
			s.offset += end + 1
			s.line++
			match := matcher.FindStringIndex(line)
			if match == nil {
				continue
			}
			start := max(0, match[0]-256)
			for start > 0 && !utf8.RuneStart(line[start]) {
				start--
			}
			end = utf8Prefix(line, max(start+1024, match[0]+min(match[1]-match[0], 1024)))
			out.Data.Matches = append(out.Data.Matches, sourceMatch{Path: s.file.Path, Line: s.line, Text: line[start:end], Identity: identity, Offset: lineOffset + start, Truncated: start > 0 || end < len(line)})
		}
		if s.offset >= len(data) {
			s.file = nil
		}
	}
	for len(s.stack) > 0 && s.stack[len(s.stack)-1].next == len(s.stack[len(s.stack)-1].entries) {
		s.stack = s.stack[:len(s.stack)-1]
	}
	if len(s.stack) == 0 && s.file == nil {
		s.complete = true
	}
	out.Data.Omitted = s.omitted
	out.Complete = s.complete && !s.incomplete
	if s.incomplete {
		out.Notice = "some files were excluded, unavailable, binary, credential-bearing or oversized"
	}
	return out, nil
}

func (s *fileSearch) retained() int {
	total := len(s.key) + 256
	for _, directory := range s.stack {
		total += entryBytes(directory.entries)
	}
	return total
}
