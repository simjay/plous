package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/simjay/plous/internal/ai/agent"
	aimodel "github.com/simjay/plous/internal/ai/model"
	"github.com/simjay/plous/internal/integrations/observation"
)

// toolSession keeps the host's lifetime and authority behind the agent interface.
// Serial calls also protect source continuations and delivery accounting when a
// transport receives concurrent tool requests.
type toolSession struct {
	op     *operation
	ctx    context.Context
	cancel context.CancelFunc
	gate   chan struct{}
}

var _ agent.Tools = (*toolSession)(nil)

func newToolSession(ctx context.Context, op *operation) *toolSession {
	ctx, cancel := context.WithCancel(ctx)
	return &toolSession{op: op, ctx: ctx, cancel: cancel, gate: make(chan struct{}, 1)}
}

func (s *toolSession) Definitions() []agent.Tool { return s.op.toolDefinitions() }

func (s *toolSession) Call(ctx context.Context, call agent.ToolCall, maxBytes int) (result json.RawMessage, err error) {
	acquired := false
	defer func() {
		if err != nil {
			s.cancel()
		}
		if acquired {
			<-s.gate
		}
	}()
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	select {
	case s.gate <- struct{}{}:
		acquired = true
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxBytes <= 0 {
		return nil, errors.New("no tool delivery allowance remains")
	}
	if strings.TrimSpace(call.ID) == "" {
		return nil, errors.New("tool call identity is required")
	}
	s.op.deliveryBytes = min(65536, maxBytes)
	safe, redactErr := s.op.redactArguments(call.Args)
	if redactErr != nil {
		// Invalid JSON must also be safe to serialize in execution records.
		safe = []byte("null")
	}
	call.Args = safe
	body, err := s.op.toolResult(ctx, call)
	return json.RawMessage(body), err
}

func (s *toolSession) Close() {
	s.cancel()
	s.gate <- struct{}{}
	defer func() { <-s.gate }()
	for key, session := range s.op.sourceReads {
		session.Close()
		delete(s.op.sourceReads, key)
	}
}

func (op *operation) toolResult(ctx context.Context, call aimodel.ToolCall) (body string, err error) {
	var receipt string
	defer func() {
		err = errors.Join(err, op.observe(Observation{Phase: "tool", Component: call.Name, CallID: call.ID, TurnID: op.o.TurnID, Input: string(marshal(map[string]any{"arguments": call.Args, "result": body, "ref": receipt})), Failed: err != nil}))
	}()
	allowed := false
	for _, d := range op.definitions() {
		if d.Name == call.Name {
			allowed = true
			break
		}
	}
	if call.Name == "history_read" {
		allowed = true
	}
	show, isShow := showDefinition(call.Name)
	if op.o.UI != nil && (call.Name == "ui_read" || isShow && (show.Owner == "" || op.hasTool(show.Owner))) {
		allowed = true
	}
	if !allowed {
		return `{"error":"tool is not available in captured scope"}`, nil
	}
	args, parseErr := toolArguments(call.Args)
	if parseErr != nil {
		return `{"error":"tool arguments must be an object"}`, nil
	}
	var purpose string
	if json.Unmarshal(args["purpose"], &purpose) != nil || strings.TrimSpace(purpose) == "" {
		return `{"error":"purpose is required"}`, nil
	}
	delete(args, "purpose")
	if call.Name == "ui_read" || isShow {
		return op.uiTool(ctx, call.Name, args)
	}
	var fields []string
	if call.Name != "history_read" {
		if raw, exists := args["fields"]; exists {
			if json.Unmarshal(raw, &fields) != nil {
				return `{"error":"fields must be an array of saved row fact names"}`, nil
			}
			delete(args, "fields")
		}
	}
	var reply any
	var r observation.Result
	var readErr error
	if call.Name == "history_read" {
		reply, r, readErr = op.historyTool(ctx, marshal(args))
		if readErr != nil && !errors.Is(readErr, errReadDenied) {
			return "", readErr
		}
	} else {
		value, err := op.externalRead(ctx, call.Name, string(marshal(args)))
		readErr = err
		var ok bool
		r, ok = value.(observation.Result)
		if readErr == nil && !ok {
			return "", fmt.Errorf("%s returned an unprojected result", call.Name)
		}
	}
	if readErr != nil {
		if errors.Is(readErr, errReadDenied) {
			r = observation.New(call.Name+".v1", "read", time.Now().UTC())
			r.Outcome = "denied"
			r.Facts["error"] = observation.Text(op.redact(readErr.Error()))
		} else if r.Validate() != nil {
			return "", readErr
		}
	}
	projected, err := op.redactJSON(marshal(r))
	if err != nil {
		return "", err
	}
	if err = json.Unmarshal(projected, &r); err != nil {
		return "", err
	}
	// Persist before exposing facts. Storage failure is fatal, never a successful tool response.
	saved, err := op.o.History.SaveObservation(op.o.WorkspaceID, op.o.ConversationID, op.o.TurnID, call.Name, call.Args, r)
	if err != nil {
		return "", err
	}
	receipt = saved.Ref
	op.historyAccess.PermitSaved(op.o.ConversationID, saved.Sequence)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if readErr != nil && !errors.Is(readErr, errReadDenied) {
		return "", readErr
	}
	if reply != nil && readErr == nil {
		safe, err := op.redactJSON(marshal(reply))
		return string(safe), err
	}

	page, err := r.Deliver(saved.Ref, observation.Selection{Fields: fields}, op.toolBytes())
	if err != nil {
		return string(marshal(map[string]any{"ref": saved.Ref, "error": err.Error(), "instruction": "Read this saved reference with fewer fields."})), nil
	}
	return string(marshal(page)), nil
}
func toolArguments(raw json.RawMessage) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(strings.NewReader(string(raw)))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("arguments must be an object")
	}
	args := map[string]json.RawMessage{}
	for d.More() {
		token, err = d.Token()
		name, ok := token.(string)
		if err != nil || !ok || args[name] != nil {
			return nil, errors.New("duplicate or invalid argument")
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil || string(value) == "null" {
			return nil, errors.New("invalid argument")
		}
		args[name] = value
	}
	if _, err = d.Token(); err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, errors.New("expected one argument object")
	}
	return args, nil
}

func (op *operation) toolBytes() int {
	return op.deliveryBytes
}

func (op *operation) redactArguments(raw []byte) ([]byte, error) {
	// Validate the original object before JSON normalization can discard
	// duplicate keys or a trailing value.
	if _, err := toolArguments(raw); err != nil {
		return nil, err
	}
	return op.redactJSON(raw)
}
