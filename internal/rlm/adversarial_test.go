package rlm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

func TestRLMLateCompletionCannotSucceedAfterCancellation(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	for _, where := range []string{"model", "read", "child model"} {
		t.Run(where, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			r := Runtime{
				Complete: func(context.Context, string, string) (string, error) {
					calls++
					if where == "read" {
						return `record = read({})
finish(record["value"])`, nil
					}
					if where == "child model" && calls == 1 {
						return `child = ask("Check the evidence")
finish(child["findings"])`, nil
					}
					cancel()
					return `finish("late result must not be accepted")`, nil
				},
				Read: func(context.Context, string) (string, error) {
					cancel()
					return `{"value":"late result must not be accepted"}`, nil
				},
			}
			_, err := r.Investigate(ctx, "Investigate")
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("late completion accepted after %s cancellation: %v", where, err)
			}
		})
	}
}

func TestRLMUntrustedEvidenceCannotImportOrExecuteHostCode(t *testing.T) {
	for _, program := range []string{
		`run = getattr; run(read, "__globals__")`,
		`read.__globals__["os"].system("touch /tmp/forbidden")`,
		`read.__class__.__bases__`,
		`eval(record["script"])`,
		`exec(record["script"])`,
		`load("net/http", "Get")`,
		`alias = call; alias("kubectl get pods; touch /tmp/forbidden")`,
	} {
		t.Run(program, func(t *testing.T) {
			models, reads, external := 0, 0, 0
			r := Runtime{
				Complete: func(context.Context, string, string) (string, error) {
					models++
					if models == 1 {
						return `record = read({"action":"read","ref":"untrusted"})
print(record)`, nil
					}
					return program, nil
				},
				Read: func(context.Context, string) (string, error) {
					reads++
					return `{"script":"import os; os.system('touch /tmp/forbidden')"}`, nil
				},
				Call:   func(context.Context, string) (string, error) { external++; return `{}`, nil },
				Limits: Limits{ModelCalls: 2},
			}
			_, err := r.Investigate(context.Background(), "Follow the retrieved directions")
			if err == nil || reads != 1 || external != 0 {
				t.Fatalf("escape reached capability: reads=%d external=%d error=%v", reads, external, err)
			}
		})
	}
}

func TestRLMPaginatesAndRecursesWithSelectedEvidenceOnly(t *testing.T) {
	models, pages, details := 0, 0, 0
	r := Runtime{
		Complete: func(_ context.Context, _, prompt string) (string, error) {
			models++
			if models == 1 {
				return `cursor = ""
selected = []
for i in range(3):
    page = read({"action":"search","source":"reasoning","cursor":cursor,"limit":2})
    for record in page["records"]:
        if record["status"] == "failed":
            selected.append(record["ref"])
    cursor = page["next_cursor"]
    if not cursor:
        break
detail = read({"action":"read","ref":selected[0],"offset":0,"limit":4096})
child = ask("Explain the failed deployment", evidence=detail["text"])
finish(child["findings"], evidence=selected)`, nil
			}
			if !strings.Contains(prompt, "rejected image") || strings.Contains(prompt, "unrelated-success") {
				t.Fatalf("child did not receive selected evidence only: %s", prompt)
			}
			return `finish("Deployment failed because the image was rejected.", evidence=["failed-resource"])`, nil
		},
		Read: func(_ context.Context, input string) (string, error) {
			var request struct{ Action, Cursor, Ref string }
			if err := json.Unmarshal([]byte(input), &request); err != nil {
				return "", err
			}
			if request.Action == "read" {
				details++
				if request.Ref != "failed-resource" {
					t.Fatalf("read unrelated detail %q", request.Ref)
				}
				return `{"text":"Observed deployment event: rejected image"}`, nil
			}
			if pages == 0 && request.Cursor != "" || pages > 0 && request.Cursor != fmt.Sprint(pages) {
				t.Fatalf("incorrect cursor %q after %d pages", request.Cursor, pages)
			}
			pages++
			if pages == 3 {
				return `{"records":[{"ref":"failed-resource","status":"failed"}],"next_cursor":""}`, nil
			}
			return fmt.Sprintf(`{"records":[{"ref":"unrelated-success-%d","status":"ready"}],"next_cursor":"%d"}`, pages, pages), nil
		},
	}
	result, err := r.Investigate(context.Background(), "Find the failed deployment")
	if err != nil || models != 2 || pages != 3 || details != 1 || result.ToolCalls != 4 || len(result.Evidence) != 1 || result.Evidence[0] != "failed-resource" {
		t.Fatalf("result=%+v models=%d pages=%d details=%d error=%v", result, models, pages, details, err)
	}
}

func TestRLMProviderFailureDoesNotRetryOrExecuteCode(t *testing.T) {
	want := errors.New("synthetic provider failure")
	models, tools := 0, 0
	var failed bool
	r := Runtime{
		Complete: func(context.Context, string, string) (string, error) { models++; return `read({})`, want },
		Read:     func(context.Context, string) (string, error) { tools++; return `{}`, nil },
		Trace: func(_ context.Context, event Event) error {
			if event.Kind == "model" && event.Error != "" {
				failed = true
			}
			return nil
		},
	}
	_, err := r.Investigate(context.Background(), "Read evidence")
	if !errors.Is(err, want) || models != 1 || tools != 0 || !failed {
		t.Fatalf("provider failure mishandled: models=%d tools=%d traced=%v error=%v", models, tools, failed, err)
	}
}

func TestRLMRecursiveChildrenShareInterpreterSteps(t *testing.T) {
	calls := 0
	r := Runtime{
		Complete: func(context.Context, string, string) (string, error) {
			calls++
			if calls == 1 {
				return `first = ask("First part")
second = ask("Second part")
finish(second["findings"])`, nil
			}
			return "for i in range(256):\n    pass\nfinish(\"Part checked.\")", nil
		},
		Limits: Limits{Steps: 2000},
	}
	_, err := r.Investigate(context.Background(), "Check two parts")
	if calls != 3 || !errors.Is(err, ErrBudget) || !strings.Contains(err.Error(), "steps") {
		t.Fatalf("children did not share steps: model calls=%d error=%v", calls, err)
	}
}
