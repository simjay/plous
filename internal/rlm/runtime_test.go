package rlm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
)

func TestRLMRecursiveExplorationReadsOnlyRequestedEvidence(t *testing.T) {
	var prompts []string
	var reads []string
	var events []Event
	var readParent, callParent string
	r := Runtime{
		Complete: func(_ context.Context, system, prompt string) (string, error) {
			if !strings.Contains(system, "test memory schema") {
				t.Fatal("missing injected instructions")
			}
			prompts = append(prompts, prompt)
			if len(prompts) == 1 {
				return `page = read({"action": "search", "source": "reasoning", "query": "restart", "limit": 2})
selected = [record for record in page["records"] if record["restarts"] > 0]
child = ask("Check this restart", evidence=text(selected))
finish(child["findings"], evidence=child["evidence"])`, nil
			}
			if !strings.Contains(prompt, "pod-1") || strings.Contains(prompt, "never-read-secret-history") {
				t.Fatalf("wrong child context: %s", prompt)
			}
			return `observed = call({"operation": "get", "resource": "pods", "name": "pod-1"})
finish(observed["status"], evidence=["record-1", "observation-1"])`, nil
		},
		Read: func(ctx context.Context, query string) (string, error) {
			readParent = CallParent(ctx)
			reads = append(reads, query)
			return `{"records":[{"name":"pod-1","restarts":2},{"name":"pod-2","restarts":0}]}`, nil
		},
		Call: func(ctx context.Context, query string) (string, error) {
			callParent = CallParent(ctx)
			reads = append(reads, query)
			return `{"status":"pod-1 is Running"}`, nil
		},
		Trace:        func(_ context.Context, event Event) error { events = append(events, event); return nil },
		Instructions: "test memory schema",
	}
	result, err := r.Investigate(context.Background(), "Investigate restarts")
	if err != nil {
		t.Fatal(err)
	}
	if result.Findings != "pod-1 is Running" || len(result.Evidence) != 2 || result.ModelCalls != 2 || result.ToolCalls != 2 {
		t.Fatalf("result = %+v", result)
	}
	if len(reads) != 2 {
		t.Fatalf("unexpected reads: %v", reads)
	}
	childFound := false
	for _, event := range events {
		if event.Kind == "investigation" && event.Depth == 1 && event.ParentID == readParent && event.ID == callParent {
			childFound = true
		}
	}
	if !childFound {
		t.Fatal("recursive trace missing parent")
	}
	if CallParent(context.Background()) != "" || readParent == "" || callParent == readParent {
		t.Fatal("invalid callback parent scope")
	}
}

func TestRLMPersistentREPLAndDeniedCommands(t *testing.T) {
	calls := 0
	guarded := 0
	var denied Event
	r := Runtime{
		Complete: func(_ context.Context, _, prompt string) (string, error) {
			calls++
			if calls == 1 {
				return `response = call({"operation":"apply"})
print(response)`, nil
			}
			if !strings.Contains(prompt, "response = call") {
				t.Fatal("previous program missing from feedback")
			}
			return `finish(response["error"])`, nil
		},
		Call: func(_ context.Context, input string) (string, error) {
			guarded++
			if !strings.Contains(input, "apply") {
				t.Fatal(input)
			}
			return "", errors.New("write operation denied")
		},
		Trace: func(_ context.Context, event Event) error {
			if event.Kind == "call" {
				denied = event
			}
			return nil
		},
	}
	result, err := r.Investigate(context.Background(), "try a write")
	if err != nil {
		t.Fatal(err)
	}
	if result.Findings != "write operation denied" || guarded != 1 || denied.Error != "write operation denied" {
		t.Fatalf("result=%+v guarded=%d trace=%+v", result, guarded, denied)
	}
}

func TestRLMChildEmptyEvidenceRemainsIterable(t *testing.T) {
	for _, program := range []string{`finish("Checked")`, `finish("Checked", evidence=[])`} {
		t.Run(program, func(t *testing.T) {
			calls := 0
			r := Runtime{Complete: func(context.Context, string, string) (string, error) {
				calls++
				switch calls {
				case 1:
					return `child = ask("Check selected evidence", evidence="synthetic observation")
refs = []
for ref in child["evidence"]:
    refs.append(ref)
finish(child["findings"], evidence=refs)`, nil
				case 2:
					return program, nil
				default:
					return "", errors.New("empty evidence must not require another model call")
				}
			}}
			result, err := r.Investigate(context.Background(), "Check the observation")
			if err != nil || result.Findings != "Checked" || result.Evidence == nil || len(result.Evidence) != 0 || calls != 2 {
				t.Fatalf("empty evidence did not remain a list: result=%+v calls=%d error=%v", result, calls, err)
			}
		})
	}
}

func TestRLMRepairReusesSuccessfulChildAfterInvalidEvidenceList(t *testing.T) {
	calls, reads := 0, 0
	r := Runtime{
		Complete: func(_ context.Context, _, prompt string) (string, error) {
			calls++
			switch calls {
			case 1:
				return `observed = read({"source":"fixture"})
child = ask("Check the selected observation", evidence=text(observed))
finish(child["findings"], evidence=[child["evidence"]])`, nil
			case 2:
				if !strings.Contains(prompt, "saved-value-42") {
					t.Fatal("child did not receive selected evidence")
				}
				return `finish("Checked saved-value-42", evidence=["record-1"])`, nil
			case 3:
				if !strings.Contains(prompt, "finish evidence must be a flat list") {
					t.Fatal("repair did not receive the evidence format error")
				}
				return `finish(child["findings"], evidence=child["evidence"])`, nil
			default:
				return "", errors.New("successful child must not be repeated")
			}
		},
		Read: func(context.Context, string) (string, error) {
			reads++
			return `{"value":"saved-value-42"}`, nil
		},
	}
	result, err := r.Investigate(context.Background(), "Verify the observation")
	if err != nil || result.Findings != "Checked saved-value-42" || len(result.Evidence) != 1 || result.Evidence[0] != "record-1" || calls != 3 || reads != 1 {
		t.Fatalf("repair failed or repeated completed work: result=%+v calls=%d reads=%d error=%v", result, calls, reads, err)
	}
}

func TestRLMUnavailableIOAndAllocationAmplification(t *testing.T) {
	for _, code := range []string{
		`load("os", "system"); system("touch /tmp/forbidden")`,
		`open("/etc/passwd")`,
		`__import__("subprocess")`,
		`getattr("x", "format")()`,
		`print("x" * 1000000000)`,
		`print([0] * 1000000000)`,
		`print("%1000000000s" % "x")`,
		`print("x".format())`,
		`x = [1]; x.extend(x)`,
		`x = [1]; y = x[:]`,
		`x = list(range(1000000000))`,
		`x = [1]; x += x`,
		`x = (1, 1); x = (x, x); y = {x: 1}`,
		`x = [1]; y = [1]; print(x == y)`,
		`__scalar = lambda x: x; print([1] == [1])`,
	} {
		t.Run(code, func(t *testing.T) {
			r := Runtime{Complete: func(context.Context, string, string) (string, error) { return code, nil }, Limits: Limits{ModelCalls: 1}}
			if _, err := r.Investigate(context.Background(), "test"); err == nil {
				t.Fatal("unsafe program accepted")
			}
		})
	}
}

func TestRLMScalarComparisonAndEnumerate(t *testing.T) {
	r := Runtime{Complete: func(context.Context, string, string) (string, error) {
		return `selected = []
for i, value in enumerate(["a", "b"]):
    if i == 1 and value in ["b", "c"]:
        selected.append(value)
finish(join(selected))`, nil
	}}
	result, err := r.Investigate(context.Background(), "root")
	if err != nil || result.Findings != "b" {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func TestRLMLengthDoesNotCopyLargeEvidence(t *testing.T) {
	r := Runtime{
		Complete: func(context.Context, string, string) (string, error) {
			return `record = read({})
finish(text(len(record["log"])))`, nil
		},
		Read: func(context.Context, string) (string, error) {
			return `{"log":"` + strings.Repeat("x", 7000) + `"}`, nil
		},
	}
	result, err := r.Investigate(context.Background(), "measure evidence")
	if err != nil || result.Findings != "7000" {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func TestRLMRunawayProgramStops(t *testing.T) {
	r := Runtime{
		Complete: func(context.Context, string, string) (string, error) {
			return "for i in range(4096):\n    for j in range(4096):\n        pass", nil
		},
		Limits: Limits{Steps: 1000},
	}
	start := time.Now()
	_, err := r.Investigate(context.Background(), "test")
	if !errors.Is(err, ErrBudget) || !strings.Contains(err.Error(), "steps") {
		t.Fatalf("got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("runaway program did not stop promptly")
	}
}

func TestRLMSharedBudgets(t *testing.T) {
	t.Run("model", func(t *testing.T) {
		calls := 0
		r := Runtime{Complete: func(context.Context, string, string) (string, error) { calls++; return `ask("child")`, nil }, Limits: Limits{ModelCalls: 2}}
		result, err := r.Investigate(context.Background(), "root")
		if !errors.Is(err, ErrBudget) || calls != 2 || result.ModelCalls != 2 {
			t.Fatalf("calls=%d result=%+v error=%v", calls, result, err)
		}
	})
	t.Run("tool", func(t *testing.T) {
		calls := 0
		r := Runtime{
			Complete: func(context.Context, string, string) (string, error) { return "read({})\nask(\"child\")", nil },
			Read:     func(context.Context, string) (string, error) { calls++; return `{}`, nil },
			Limits:   Limits{ToolCalls: 1},
		}
		result, err := r.Investigate(context.Background(), "root")
		if !errors.Is(err, ErrBudget) || calls != 1 || result.ToolCalls != 1 {
			t.Fatalf("calls=%d result=%+v error=%v", calls, result, err)
		}
	})
	t.Run("depth", func(t *testing.T) {
		r := Runtime{Complete: func(context.Context, string, string) (string, error) { return `ask("child")`, nil }, Limits: Limits{MaxDepth: 1}}
		_, err := r.Investigate(context.Background(), "root")
		if !errors.Is(err, ErrBudget) || !strings.Contains(err.Error(), "depth") {
			t.Fatal(err)
		}
	})
	t.Run("allocation", func(t *testing.T) {
		r := Runtime{Complete: func(context.Context, string, string) (string, error) {
			return "for i in range(100):\n    x = list(range(100))", nil
		}, Limits: Limits{AllocationBytes: 10000}}
		_, err := r.Investigate(context.Background(), "root")
		if !errors.Is(err, ErrBudget) || !strings.Contains(err.Error(), "allocation") {
			t.Fatal(err)
		}
	})
}

func TestRLMCancellationAndTraceFailure(t *testing.T) {
	t.Run("computation deadline", func(t *testing.T) {
		r := Runtime{Complete: func(context.Context, string, string) (string, error) {
			return "for i in range(4096):\n    for j in range(4096):\n        pass", nil
		}, Limits: Limits{Steps: 1 << 40, Timeout: 10 * time.Millisecond}}
		start := time.Now()
		_, err := r.Investigate(context.Background(), "root")
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
			t.Fatalf("deadline did not stop computation: %v", err)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		r := Runtime{
			Complete: func(context.Context, string, string) (string, error) { return `read({})`, nil },
			Read:     func(ctx context.Context, _ string) (string, error) { cancel(); <-ctx.Done(); return "", ctx.Err() },
		}
		_, err := r.Investigate(ctx, "root")
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
	t.Run("trace", func(t *testing.T) {
		failure := errors.New("storage unavailable")
		r := Runtime{Complete: func(context.Context, string, string) (string, error) { return `finish("ok")`, nil }, Trace: func(context.Context, Event) error { return failure }}
		_, err := r.Investigate(context.Background(), "root")
		if !errors.Is(err, failure) {
			t.Fatal(err)
		}
	})
}

func TestRLMEncodingRejectsCyclesAndLargeOutput(t *testing.T) {
	value := starlark.NewList(nil)
	if err := value.Append(value); err != nil {
		t.Fatal(err)
	}
	if _, err := encodeValue(value, 1024); err == nil {
		t.Fatal("cycle expanded")
	}
	r := Runtime{
		Complete: func(context.Context, string, string) (string, error) { return `read({})`, nil },
		Read:     func(context.Context, string) (string, error) { return strings.Repeat("x", 1025), nil },
		Limits:   Limits{ResultBytes: 1024},
	}
	if _, err := r.Investigate(context.Background(), "root"); !errors.Is(err, ErrBudget) {
		t.Fatal(err)
	}
}
