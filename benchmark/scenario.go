// Package plousbench tests production tool contracts without running an agent.
package plousbench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const Protocol = "plousbench-tools-v1"

type Suite struct {
	Version string     `json:"version"`
	Cases   []Scenario `json:"cases"`
}

type Scenario struct {
	ID          string `json:"id"`
	Fixture     string `json:"fixture"`
	HistorySize int    `json:"history_size,omitempty"`
	Steps       []Step `json:"steps"`
}

type Step struct {
	ID            string            `json:"id"`
	Tool          string            `json:"tool"`
	Args          json.RawMessage   `json:"args"`
	Before        string            `json:"before,omitempty"`
	Capture       map[string]string `json:"capture,omitempty"`
	Checks        []Check           `json:"checks,omitempty"`
	DisplayChecks []Check           `json:"display_checks,omitempty"`
	Error         bool              `json:"error,omitempty"`
	MinReads      *int              `json:"min_reads,omitempty"`
	MaxReads      *int              `json:"max_reads,omitempty"`
}

// Checks address JSON fields, not prose produced by a model.
type Check struct {
	Path     string          `json:"path"`
	Equals   json.RawMessage `json:"equals,omitempty"`
	Contains *string         `json:"contains,omitempty"`
	Length   *int            `json:"length,omitempty"`
	Absent   bool            `json:"absent,omitempty"`
}

func LoadSuite(path string) (Suite, error) {
	var s Suite
	if err := readJSON(path, &s); err != nil {
		return s, err
	}
	return s, s.Validate()
}

func (s Suite) Validate() error {
	if s.Version != Protocol || len(s.Cases) == 0 {
		return errors.New("expected a nonempty tools-v1 suite")
	}
	ids := map[string]bool{}
	for _, c := range s.Cases {
		if c.Fixture == "" || strings.ContainsAny(c.Fixture, "/\\.") || c.ID == "" || strings.ContainsAny(c.ID, "/\\") || ids[c.ID] || len(c.Steps) == 0 || c.HistorySize < 0 || c.HistorySize > 10000 {
			return fmt.Errorf("invalid or duplicate case %q", c.ID)
		}
		ids[c.ID] = true
		steps := map[string]bool{}
		for _, step := range c.Steps {
			if step.ID == "" || steps[step.ID] || step.Tool == "" || !json.Valid(step.Args) || !step.Error && len(step.Checks) == 0 {
				return fmt.Errorf("case %s has an invalid step %q", c.ID, step.ID)
			}
			steps[step.ID] = true
			if step.MinReads != nil && *step.MinReads < 0 || step.MaxReads != nil && *step.MaxReads < 0 || step.MinReads != nil && step.MaxReads != nil && *step.MinReads > *step.MaxReads {
				return errors.New("invalid API read bounds")
			}
			for _, check := range append(append([]Check(nil), step.Checks...), step.DisplayChecks...) {
				n := 0
				if check.Equals != nil {
					n++
				}
				if check.Contains != nil {
					n++
				}
				if check.Length != nil {
					n++
				}
				if check.Absent {
					n++
				}
				if n != 1 || check.Path != "" && !strings.HasPrefix(check.Path, "/") || check.Length != nil && *check.Length < 0 {
					return errors.New("each check needs one expectation and a JSON pointer")
				}
			}
		}
	}
	return nil
}

func JSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func Hash(v any) string { sum := sha256.Sum256(JSON(v)); return hex.EncodeToString(sum[:]) }

func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 32<<20))
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("expected one JSON document")
	}
	return nil
}
