package source

import (
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

func parseIgnore(data []byte, domain []string) []gitignore.Pattern {
	var patterns []gitignore.Pattern
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, gitignore.ParsePattern(line, domain))
		}
	}
	return patterns
}

// A policy is fixed for a scan, including every page of a search.
type scanPolicy struct{ patterns []gitignore.Pattern }

func loadScanPolicy(load func() ([]byte, error)) (*scanPolicy, error) {
	if load == nil {
		return &scanPolicy{}, nil
	}
	data, err := load()
	if err != nil {
		return nil, err
	}
	return &scanPolicy{patterns: parseIgnore(data, nil)}, nil
}
func (p *scanPolicy) excludes(name string, directory bool) bool {
	parts := strings.Split(name, "/")
	for n, part := range parts {
		if part == ".git" {
			return true
		}
		if p != nil && gitignore.NewMatcher(p.patterns).Match(parts[:n+1], n < len(parts)-1 || directory) {
			return true
		}
	}
	return false
}
