package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/simjay/plous/internal/ai/agent"
)

type fixtureTools struct {
	calls  atomic.Int32
	listed atomic.Int32
	fail   bool
}

func (f *fixtureTools) Definitions() []agent.Tool {
	f.listed.Add(1)
	return []agent.Tool{{Name: "history_read", Description: "Saved evidence", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}}
}
func (f *fixtureTools) Call(ctx context.Context, call agent.ToolCall, maxBytes int) (json.RawMessage, error) {
	f.calls.Add(1)
	if f.fail {
		return nil, errors.New("evidence save failed")
	}
	return json.RawMessage(`{"ref":"record:fixture:saved","count":3}`), nil
}
func TestBridgeAuthorityAndFatalFailure(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(context.Canceled)
	tools := &fixtureTools{}
	b, err := OpenBridge(ctx, tools, "turn")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	call := func(token, id, method string) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": map[string]any{"name": "history_read", "arguments": map[string]any{}}})
		r, _ := http.NewRequest("POST", b.URL, bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+token)
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	if status, _ := call("foreign", "1", "tools/call"); status != 401 || tools.calls.Load() != 0 {
		t.Fatal("foreign authority acquired evidence")
	}
	if _, out := call(b.token, "1", "tools/list"); out["result"] == nil {
		t.Fatal(out)
	}
	if _, out := call(b.token, "2", "tools/call"); out["result"] == nil || tools.calls.Load() != 1 {
		t.Fatal(out)
	}
	if _, out := call(b.token, "2", "tools/call"); out["error"] == nil || tools.calls.Load() != 1 {
		t.Fatal("duplicate reacquired evidence", out)
	}
	tools.fail = true
	if _, out := call(b.token, "3", "tools/call"); out["error"] == nil || b.ctx.Err() == nil || ctx.Err() != nil {
		t.Fatal("fatal failure did not revoke only tool access", out)
	}
	if status, _ := call(b.token, "4", "tools/call"); status != 410 || tools.calls.Load() != 2 {
		t.Fatal("closed turn acquired evidence")
	}
}

func TestBridgeRequestIdentityWithoutSessionCallCap(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(context.Canceled)
	tools := &fixtureTools{}
	b, err := OpenBridge(ctx, tools, "turn")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	call := func(id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", b.URL, strings.NewReader(`{"jsonrpc":"2.0","id":`+id+`,"method":"tools/call","params":{"name":"history_read","arguments":{}}}`))
		r.Header.Set("Authorization", "Bearer "+b.token)
		w := httptest.NewRecorder()
		b.ServeHTTP(w, r)
		return w
	}
	for _, id := range []string{`null`, `true`, `{}`, `[]`, `1.5`} {
		if w := call(id); w.Code != 400 || tools.calls.Load() != 0 {
			t.Fatal(id, w.Code, tools.calls.Load())
		}
	}
	if w := call(`"a"`); w.Code != 200 || tools.calls.Load() != 1 {
		t.Fatal(w, tools.calls.Load())
	}
	if w := call(`"\u0061"`); !strings.Contains(w.Body.String(), "already used") || tools.calls.Load() != 1 {
		t.Fatal(w, tools.calls.Load())
	}
	for i := 0; i < 256; i++ {
		if w := call(fmt.Sprint(i)); w.Code != 200 || strings.Contains(w.Body.String(), `"error"`) || ctx.Err() != nil {
			t.Fatal(w, tools.calls.Load(), ctx.Err())
		}
	}
	if tools.calls.Load() != 257 {
		t.Fatal(tools.calls.Load())
	}
}
