package plousbench

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Report struct {
	Version     string    `json:"version"`
	Suite       Suite     `json:"suite"`
	FixtureHash string    `json:"fixture_hash"`
	SourceHash  string    `json:"source_hash"`
	Platform    string    `json:"platform"`
	GoVersion   string    `json:"go_version"`
	Started     time.Time `json:"started"`
	Results     []Result  `json:"results"`
}

func NewReport(s Suite, fixtureHash, sourceHash string) Report {
	return Report{Version: Protocol, Suite: s, FixtureHash: fixtureHash, SourceHash: sourceHash, Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(), Started: time.Now().UTC()}
}

func (r Report) Validate() error {
	if r.Version != Protocol || len(r.FixtureHash) != 64 || len(r.SourceHash) != 64 || r.Started.IsZero() || len(r.Results) == 0 {
		return errors.New("invalid tools-v1 report")
	}
	if err := r.Suite.Validate(); err != nil {
		return err
	}
	cases := map[string]Scenario{}
	for _, c := range r.Suite.Cases {
		cases[c.ID] = c
	}
	for _, result := range r.Results {
		c, ok := cases[result.Case]
		if !ok || len(result.Calls) != len(c.Steps) || len(result.SchemaHash) != 64 || result.Transport != "direct" && result.Transport != "mcp" {
			return errors.New("result does not match the recorded workload")
		}
		for i, call := range result.Calls {
			if call.ID != c.Steps[i].ID || call.Tool != c.Steps[i].Tool || call.DurationNS < 0 || call.APIReads < 0 || call.OutputBytes != len(call.Body) {
				return errors.New("invalid call measurements")
			}
			if !call.Skipped && len(call.Failures) == 0 {
				var err error
				if call.Error != "" {
					err = errors.New(call.Error)
				}
				if failures := Assess(c.Steps[i], call.Body, err, call.APIReads); len(failures) != 0 {
					return errors.New("passing result contradicts its expected facts")
				}
				if checks := c.Steps[i].DisplayChecks; len(checks) != 0 {
					if failures := Assess(Step{Checks: checks}, call.Display, nil, 0); len(failures) != 0 {
						return errors.New("passing result contradicts its expected display")
					}
				}
			}
		}
	}
	return nil
}

func WriteReport(path string, r Report) error {
	if err := r.Validate(); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(JSON(r), '\n'))
	return errors.Join(writeErr, f.Close())
}

func ReadReport(path string) (Report, error) {
	var r Report
	if err := readJSON(path, &r); err != nil {
		return r, err
	}
	return r, r.Validate()
}

func RenderReport(r Report) string {
	type group struct {
		samples, passed, reads, bytes int
		elapsed                       []int64
	}
	groups := map[string]*group{}
	for _, c := range r.Suite.Cases {
		for _, transport := range []string{"direct", "mcp"} {
			groups[c.ID+" / "+transport] = &group{}
		}
	}
	for _, result := range r.Results {
		key := result.Case + " / " + result.Transport
		g := groups[key]
		if g == nil {
			g = &group{}
			groups[key] = g
		}
		g.samples++
		if result.Passed() {
			g.passed++
		}
		var elapsed int64
		for _, c := range result.Calls {
			elapsed += c.DurationNS
			g.reads += c.APIReads
			g.bytes += c.OutputBytes
		}
		if elapsed > 0 {
			g.elapsed = append(g.elapsed, elapsed)
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out strings.Builder
	fmt.Fprintf(&out, "PlousBench tools (%s, %s)\nMeasured tool calls include persistence; fixture setup and audits are excluded.\nFailed attempts remain in timing samples.\n\nCase / transport | Passed | Timed | Median ms | Mean bytes | Mean API reads\n", r.Platform, r.GoVersion)
	for _, key := range keys {
		g := groups[key]
		if g.samples == 0 {
			fmt.Fprintf(&out, "%s | not run | 0 | - | - | -\n", key)
			continue
		}
		if len(g.elapsed) == 0 {
			fmt.Fprintf(&out, "%s | %d/%d | 0 | - | - | -\n", key, g.passed, g.samples)
			continue
		}
		sort.Slice(g.elapsed, func(i, j int) bool { return g.elapsed[i] < g.elapsed[j] })
		median := float64(g.elapsed[len(g.elapsed)/2])
		if len(g.elapsed)%2 == 0 {
			median = (median + float64(g.elapsed[len(g.elapsed)/2-1])) / 2
		}
		fmt.Fprintf(&out, "%s | %d/%d | %d | %.3f | %.0f | %.1f\n", key, g.passed, g.samples, len(g.elapsed), median/1e6, float64(g.bytes)/float64(len(g.elapsed)), float64(g.reads)/float64(len(g.elapsed)))
	}
	for _, result := range r.Results {
		for _, failure := range result.Failures {
			fmt.Fprintf(&out, "Failure: %s / %s: %s\n", result.Case, result.Transport, failure)
		}
		for _, call := range result.Calls {
			for _, failure := range call.Failures {
				fmt.Fprintf(&out, "Failure: %s / %s / %s: %s\n", result.Case, result.Transport, call.ID, failure)
			}
		}
	}
	return out.String()
}
