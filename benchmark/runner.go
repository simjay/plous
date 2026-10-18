package plousbench

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/simjay/plous/internal/ai/agent"
)

// Harness supplies the production session and fixture lifecycle. Audits and
// fixture transitions run outside the measured tool call, including MCP I/O.
type Harness struct {
	Tools     agent.Tools
	Variables map[string]any
	Reads     func() int
	Before    func(context.Context, Step) error
	Verify    func(context.Context, Step, json.RawMessage, error) error
	Display   func() json.RawMessage
}

type CallResult struct {
	ID          string          `json:"id"`
	Tool        string          `json:"tool"`
	Args        json.RawMessage `json:"args,omitempty"`
	Body        json.RawMessage `json:"body,omitempty"`
	Display     json.RawMessage `json:"display,omitempty"`
	Error       string          `json:"error,omitempty"`
	Failures    []string        `json:"failures,omitempty"`
	Skipped     bool            `json:"skipped,omitempty"`
	DurationNS  int64           `json:"duration_ns"`
	OutputBytes int             `json:"output_bytes"`
	APIReads    int             `json:"api_reads"`
}

type Result struct {
	Case       string       `json:"case"`
	Transport  string       `json:"transport"`
	SchemaHash string       `json:"schema_hash"`
	Calls      []CallResult `json:"calls"`
	Failures   []string     `json:"failures,omitempty"`
}

func (r Result) Passed() bool {
	if len(r.Calls) == 0 || len(r.Failures) > 0 {
		return false
	}
	for _, c := range r.Calls {
		if c.Skipped || len(c.Failures) > 0 {
			return false
		}
	}
	return true
}

func Run(ctx context.Context, scenario Scenario, transport string, h Harness) Result {
	r := Result{Case: scenario.ID, Transport: transport, SchemaHash: Hash(h.Tools.Definitions())}
	vars := maps.Clone(h.Variables)
	if vars == nil {
		vars = map[string]any{}
	}
	blocked := false
	for _, step := range scenario.Steps {
		c := CallResult{ID: step.ID, Tool: step.Tool}
		if blocked {
			c.Skipped = true
			r.Calls = append(r.Calls, c)
			continue
		}
		var err error
		if ctx.Err() != nil {
			err = ctx.Err()
		} else if h.Before != nil {
			err = h.Before(ctx, step)
		}
		if err == nil {
			c.Args, err = resolve(step.Args, vars)
		}
		if err != nil {
			c.Failures = []string{err.Error()}
			blocked = true
			r.Calls = append(r.Calls, c)
			continue
		}
		reads := h.Reads()
		start := time.Now()
		c.Body, err = h.Tools.Call(ctx, agent.ToolCall{ID: step.ID, Name: step.Tool, Args: c.Args}, 65536)
		c.DurationNS = time.Since(start).Nanoseconds()
		c.OutputBytes, c.APIReads = len(c.Body), h.Reads()-reads
		if err != nil {
			c.Error = err.Error()
		}
		c.Failures = Assess(step, c.Body, err, c.APIReads)
		if len(step.DisplayChecks) != 0 {
			if h.Display != nil {
				c.Display = h.Display()
			}
			c.Failures = append(c.Failures, Assess(Step{Checks: step.DisplayChecks}, c.Display, nil, 0)...)
		}
		if h.Verify != nil {
			if auditErr := h.Verify(ctx, step, c.Body, err); auditErr != nil {
				c.Failures = append(c.Failures, auditErr.Error())
			}
		}
		var value any
		if len(step.Capture) > 0 {
			value, _ = decodeValue(c.Body)
			for key, path := range step.Capture {
				v, ok := Pointer(value, path)
				if !ok {
					c.Failures = append(c.Failures, fmt.Sprintf("cannot capture %s at %s", key, path))
				} else {
					vars[key] = v
				}
			}
		}
		blocked = len(c.Failures) > 0
		r.Calls = append(r.Calls, c)
	}
	return r
}
