package plousbench

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/ai/agent"
)

type testTools struct {
	calls int
	run   func(agent.ToolCall) (json.RawMessage, error)
}

func (s *testTools) Definitions() []agent.Tool {
	return []agent.Tool{{Name: "read", Parameters: json.RawMessage(`{"type":"object"}`)}}
}
func (s *testTools) Call(_ context.Context, c agent.ToolCall, _ int) (json.RawMessage, error) {
	s.calls++
	return s.run(c)
}

func TestAssessmentRejectsFalseEvidence(t *testing.T) {
	step := Step{Checks: []Check{{Path: "/coverage/complete", Equals: JSON(false)}, {Path: "/coverage/total", Absent: true}, {Path: "/rows", Length: new(int)}}}
	if failed := Assess(step, JSON(map[string]any{"coverage": map[string]any{"complete": false}, "rows": []any{}}), nil, 0); len(failed) != 0 {
		t.Fatal(failed)
	}
	for _, bad := range []string{`{"coverage":{"complete":true},"rows":[]}`, `{"coverage":{"complete":false,"total":0},"rows":[]}`, `{"coverage":{"complete":false},"rows":null}`, `{"rows":[]}`, `not JSON`} {
		if len(Assess(step, json.RawMessage(bad), nil, 0)) == 0 {
			t.Fatal("false evidence passed", bad)
		}
	}
	if len(Assess(Step{Checks: []Check{{Path: "/a~1b/~0", Equals: JSON(3)}}}, json.RawMessage(`{"a/b":{"~":3}}`), nil, 0)) != 0 {
		t.Fatal("JSON pointer escaping")
	}
}

func TestRunnerCapturesReferencesAndRetainsFailedAndSkippedCalls(t *testing.T) {
	tool := &testTools{run: func(c agent.ToolCall) (json.RawMessage, error) {
		if c.ID == "first" {
			return json.RawMessage(`{"ref":"saved:original","count":5}`), nil
		}
		if !strings.Contains(string(c.Args), "saved:original") {
			t.Fatal("reference was not bound", string(c.Args))
		}
		return json.RawMessage(`{"count":4}`), nil
	}}
	c := Scenario{ID: "counts", Fixture: "fixture", Steps: []Step{
		{ID: "first", Tool: "read", Args: JSON(map[string]string{}), Capture: map[string]string{"original": "/ref"}, Checks: []Check{{Path: "/count", Equals: JSON(5)}}},
		{ID: "second", Tool: "read", Args: JSON(map[string]string{"ref": "$original"}), Checks: []Check{{Path: "/count", Equals: JSON(5)}}},
		{ID: "third", Tool: "read", Args: JSON(map[string]string{}), Checks: []Check{{Path: "/count", Equals: JSON(5)}}},
	}}
	audits := 0
	r := Run(t.Context(), c, "direct", Harness{Tools: tool, Reads: func() int { return tool.calls }, Verify: func(context.Context, Step, json.RawMessage, error) error { audits++; return nil }})
	if r.Passed() || tool.calls != 2 || audits != 2 || len(r.Calls) != 3 || !r.Calls[2].Skipped || r.Calls[0].APIReads != 1 || r.Calls[1].OutputBytes != len(r.Calls[1].Body) {
		t.Fatal(r, tool.calls, audits)
	}
	report := NewReport(Suite{Version: Protocol, Cases: []Scenario{c}}, Hash("fixture"), Hash("source"))
	report.Results = []Result{r}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(RenderReport(report), "0/1") || !strings.Contains(RenderReport(report), "not run") || !strings.Contains(RenderReport(report), "Failure: counts / direct / second") {
		t.Fatal("failed attempt disappeared")
	}
	report.Results[0].Calls[1].Failures = nil
	if report.Validate() == nil {
		t.Fatal("incorrect output recertified as passing")
	}
}

func TestRunnerDoesNotDispatchAfterSetupOrBindingFailure(t *testing.T) {
	for _, before := range []bool{false, true} {
		tool := &testTools{run: func(agent.ToolCall) (json.RawMessage, error) {
			t.Fatal("dispatched after setup failure")
			return nil, nil
		}}
		h := Harness{Tools: tool, Reads: func() int { return 0 }}
		if before {
			h.Before = func(context.Context, Step) error { return errors.New("restart failed") }
		}
		c := Scenario{ID: "broken", Steps: []Step{{ID: "read", Tool: "read", Args: json.RawMessage(`{"ref":"$missing"}`)}}}
		r := Run(t.Context(), c, "direct", h)
		if r.Passed() || len(r.Calls[0].Failures) == 0 || tool.calls != 0 {
			t.Fatal(r)
		}
	}
}

func TestReportsRejectLegacyFormatsAndNeverOverwrite(t *testing.T) {
	tool := &testTools{run: func(agent.ToolCall) (json.RawMessage, error) { return json.RawMessage(`{"count":5}`), nil }}
	c := Scenario{ID: "count", Fixture: "fixture", Steps: []Step{{ID: "read", Tool: "read", Args: JSON(map[string]string{}), Checks: []Check{{Path: "/count", Equals: JSON(5)}}}}}
	r := NewReport(Suite{Version: Protocol, Cases: []Scenario{c}}, Hash("fixture"), Hash("source"))
	r.Results = []Result{Run(t.Context(), c, "direct", Harness{Tools: tool, Reads: func() int { return 0 }})}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := WriteReport(path, r); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if WriteReport(path, r) == nil {
		t.Fatal("overwrote prior results")
	}
	if _, err = ReadReport(path); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("changed existing report")
	}
	r.Version = "plousbench-direct-v1"
	if r.Validate() == nil {
		t.Fatal("accepted retired report")
	}
	if err := Toolkit([]string{"generate"}, os.Stderr); err == nil {
		t.Fatal("retired CLI accepted")
	}
}

func TestSuiteRejectsAmbiguousChecksAndDuplicateCases(t *testing.T) {
	s, err := LoadSuite("fixtures/tools.json")
	if err != nil {
		t.Fatal(err)
	}
	s.Cases = append(s.Cases, s.Cases[0])
	if s.Validate() == nil {
		t.Fatal("duplicate case")
	}
	s.Cases = s.Cases[:1]
	s.Cases[0].Steps[0].Checks[0].Absent = true
	if s.Validate() == nil {
		t.Fatal("ambiguous expectation")
	}
}

func TestIntegerFactsAndArgumentsKeepTheirPrecision(t *testing.T) {
	step := Step{Checks: []Check{{Path: "/count", Equals: json.RawMessage(`9007199254740992`)}}}
	if len(Assess(step, json.RawMessage(`{"count":9007199254740993}`), nil, 0)) == 0 {
		t.Fatal("different integers received the same score")
	}
	args, err := resolve(json.RawMessage(`{"number":9007199254740993}`), nil)
	if err != nil || string(args) != `{"number":9007199254740993}` {
		t.Fatal("rounded a tool argument", string(args), err)
	}
	value, err := decodeValue([]byte(`{"count":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	captured, _ := Pointer(value, "/count")
	args, err = resolve(json.RawMessage(`{"number":"$count"}`), map[string]any{"count": captured})
	if err != nil || string(args) != `{"number":9007199254740993}` {
		t.Fatal("rounded a captured fact", string(args), err)
	}
}

func TestDisplayChecksAreRetainedAndRevalidated(t *testing.T) {
	tool := &testTools{run: func(agent.ToolCall) (json.RawMessage, error) { return JSON(map[string]string{"status": "opened"}), nil }}
	c := Scenario{ID: "display", Fixture: "fixture", Steps: []Step{{ID: "show", Tool: "helm_show", Args: JSON(map[string]string{}),
		Checks:        []Check{{Path: "/status", Equals: JSON("opened")}},
		DisplayChecks: []Check{{Path: "/before", Equals: JSON("replicas: 2")}, {Path: "/after", Equals: JSON("replicas: 3")}},
	}}}
	for _, before := range []string{"replicas: 2", "replicas: 3", ""} {
		h := Harness{Tools: tool, Reads: func() int { return 0 }}
		if before != "" {
			h.Display = func() json.RawMessage { return JSON(map[string]string{"before": before, "after": "replicas: 3"}) }
		}
		result := Run(t.Context(), c, "direct", h)
		if result.Passed() != (before == "replicas: 2") {
			t.Fatal(result)
		}
		report := NewReport(Suite{Version: Protocol, Cases: []Scenario{c}}, Hash("fixture"), Hash("source"))
		report.Results = []Result{result}
		if err := report.Validate(); err != nil {
			t.Fatal(err)
		}
		if result.Passed() {
			report.Results[0].Calls[0].Display = JSON(map[string]string{"before": "replicas: 3", "after": "replicas: 2"})
			if report.Validate() == nil {
				t.Fatal("reversed display recertified as passing")
			}
		}
	}
}
