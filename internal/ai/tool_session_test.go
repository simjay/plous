package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/integrations/observation"
)

func TestToolSessionSharedScopeAndSavedEvidence(t *testing.T) {
	var reads atomic.Int32
	op := deliveryFixture(t, func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if r.URL.Path == "/api/v1" {
			deliveryDiscovery(w)
			return
		}
		deliveryPods(w, 3, "")
	})
	session := newToolSession(t.Context(), op)
	t.Cleanup(session.Close)
	var tools agent.Tools = session
	definitions := tools.Definitions()
	definitions[0].Name = "changed"
	definitions[0].Parameters[0] = '!'
	for _, definition := range tools.Definitions() {
		if definition.Name == "changed" || !json.Valid(definition.Parameters) {
			t.Fatal("caller mutated host schemas")
		}
	}
	op.o.Observe = func(e Observation) error {
		if e.Phase == "tool" && !json.Valid([]byte(e.Input)) {
			t.Error("tool execution record is not valid JSON")
		}
		return nil
	}
	for _, args := range []string{`{`, `{"purpose":"one","purpose":"two"}`, `{"purpose":null}`, `{} {}`} {
		call := deliveryRead()
		call.Args = json.RawMessage(args)
		body, err := tools.Call(t.Context(), call, 65536)
		if err != nil || !strings.Contains(string(body), "tool arguments must be an object") || reads.Load() != 0 {
			t.Fatal("malformed arguments escaped corrective feedback", string(body), err)
		}
	}
	denied := deliveryRead()
	denied.Args = json.RawMessage(strings.ReplaceAll(string(denied.Args), "east", "other"))
	body, err := tools.Call(t.Context(), denied, 65536)
	if err != nil || reads.Load() != 0 || !strings.Contains(string(body), `"outcome":"denied"`) {
		t.Fatal("scope check did not precede acquisition", string(body), err)
	}
	body, err = tools.Call(t.Context(), deliveryRead(), 65536)
	if err != nil {
		t.Fatal(err)
	}
	var delivery observation.Delivery
	if err = json.Unmarshal(body, &delivery); err != nil || delivery.Ref == "" || delivery.Page.Returned != 3 {
		t.Fatal(string(body), err)
	}
	// The reply is useful only if Sources can already read its saved projection.
	saved, err := op.o.History.TurnEvidence(t.Context(), op.o.WorkspaceID, op.o.ConversationID, op.o.TurnID)
	if err != nil || len(saved) != 2 || len(saved[1].Projection.Rows) != 3 {
		t.Fatal("reply preceded durable evidence", saved, err)
	}
}

func TestToolSessionRejectsCallsAfterLifetimeEnds(t *testing.T) {
	for _, end := range []string{"cancel", "close", "failure"} {
		t.Run(end, func(t *testing.T) {
			var reads atomic.Int32
			op := deliveryFixture(t, func(http.ResponseWriter, *http.Request) { reads.Add(1) })
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			session := newToolSession(ctx, op)
			t.Cleanup(session.Close)
			switch end {
			case "cancel":
				cancel()
			case "close":
				session.Close()
			case "failure":
				if _, err := session.Call(t.Context(), deliveryRead(), 0); err == nil {
					t.Fatal("exhausted delivery allowance accepted")
				}
			}
			if _, err := session.Call(t.Context(), deliveryRead(), 65536); !errors.Is(err, context.Canceled) || reads.Load() != 0 {
				t.Fatal("expired session reached live tools", err, reads.Load())
			}
		})
	}
}

func TestToolSessionSerializesCallsAndCancelsWaitingRead(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var reads atomic.Int32
	op := deliveryFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1" {
			deliveryDiscovery(w)
			return
		}
		if reads.Add(1) == 1 {
			close(started)
			<-release
		}
		deliveryPods(w, 1, "")
	})
	session := newToolSession(t.Context(), op)
	t.Cleanup(session.Close)
	first := make(chan error, 1)
	go func() {
		_, err := session.Call(t.Context(), deliveryRead(), 65536)
		first <- err
	}()
	<-started
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	_, err := session.Call(ctx, deliveryRead(), 2000)
	close(release)
	<-first
	if !errors.Is(err, context.DeadlineExceeded) || reads.Load() != 1 {
		t.Fatal("cancelled waiting call reached acquisition", err, reads.Load())
	}
}
