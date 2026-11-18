package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	op       *operation
	ctx      context.Context
	cancel   context.CancelFunc
	gate     chan struct{}
	external bool
}

var _ agent.Tools = (*toolSession)(nil)

func newToolSession(ctx context.Context, op *operation) *toolSession {
	ctx, cancel := context.WithCancel(ctx)
	return &toolSession{op: op, ctx: ctx, cancel: cancel, gate: make(chan struct{}, 1)}
}

func (s *toolSession) Definitions() []agent.Tool { return s.op.toolDefinitions() }

func (s *toolSession) Call(requestCtx context.Context, call agent.ToolCall, maxBytes int) (result json.RawMessage, err error) {
	acquired, mayHaveChangedState := false, false
	defer func() {
		if s.external && requestCtx.Err() != nil && s.ctx.Err() == nil &&
			!mayHaveChangedState &&
			agent.OnlyCancellation(err, requestCtx.Err()) {
			err = fmt.Errorf("%w: %w", agent.ErrRequestCancelled, requestCtx.Err())
		} else if err != nil {
			s.cancel()
		}
		if acquired {
			<-s.gate
		}
	}()
	ctx, cancel := context.WithCancel(requestCtx)
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
	body, err := s.op.toolResult(ctx, call, func() { mayHaveChangedState = true })
	return json.RawMessage(body), err
}

func (s *toolSession) Close() {
	s.cancel()
	s.gate <- struct{}{}
	defer func() { <-s.gate }()
}

func (op *operation) toolResult(ctx context.Context, call aimodel.ToolCall, beforeEffect func()) (body string, err error) {
	args, parseErr := op.toolInput(call.Args)
	// Keep the full sanitized request separate from the arguments consumed by routing.
	call.Args = marshal(args)
	var receipt string
	defer func() {
		err = errors.Join(err, op.observe(Observation{Phase: "tool", Component: call.Name, CallID: call.ID, TurnID: op.o.TurnID, Input: string(marshal(map[string]any{"arguments": call.Args, "result": body, "ref": receipt})), Failed: err != nil}))
	}()
	tool, available := op.tool(call.Name)
	if !available {
		return `{"error":"tool is not available in captured scope"}`, nil
	}
	if parseErr != nil {
		return `{"error":"tool arguments must be an object"}`, nil
	}
	var purpose string
	if json.Unmarshal(args["purpose"], &purpose) != nil || strings.TrimSpace(purpose) == "" {
		return `{"error":"purpose is required"}`, nil
	}
	delete(args, "purpose")
	if tool.kind == viewTool {
		if stringArgument(args, "operation") == "open" {
			beforeEffect()
		}
		return op.uiTool(ctx, args)
	}
	var fields []string
	if tool.kind != historyTool {
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
	if tool.kind == historyTool {
		reply, r, readErr = op.historyTool(ctx, marshal(args))
		if readErr != nil && !errors.Is(readErr, errReadDenied) {
			return "", readErr
		}
	} else {
		if tool.kind == runTool && op.o.Debug.Allows(stringArgument(args, "operation")) {
			// A durable intent must precede dispatch of an active operation.
			if err := op.o.History.SaveToolIntent(op.o.WorkspaceID, op.o.ConversationID, op.o.TurnID, call); err != nil {
				return "", err
			}
			beforeEffect()
		}
		r, readErr = op.dispatchRead(ctx, tool, args)
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
	if readErr != nil && !errors.Is(readErr, errReadDenied) {
		return "", readErr
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
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

func (op *operation) toolBytes() int {
	return op.deliveryBytes
}

func (op *operation) redactArguments(raw []byte) ([]byte, error) {
	args, err := op.toolInput(raw)
	if err != nil {
		return nil, err
	}
	return marshal(args), nil
}

func (op *operation) toolInput(raw json.RawMessage) (map[string]json.RawMessage, error) {
	args, err := observation.ParseArguments(raw)
	if err != nil || len(op.keys) == 0 {
		return args, err
	}
	safe := make(map[string]json.RawMessage, len(args))
	for name, value := range args {
		key := op.redact(name)
		if _, exists := safe[key]; exists {
			return nil, errors.New("duplicate sanitized argument")
		}
		safe[key], err = op.redactJSON(value)
		if err != nil {
			return nil, err
		}
	}
	return safe, nil
}

func (op *operation) dispatchRead(ctx context.Context, tool toolDefinition, args map[string]json.RawMessage) (observation.Result, error) {
	var connection Connection
	if tool.connection != "" {
		id := stringArgument(args, "connection_id")
		if id == "" {
			return observation.Result{}, denyRead(errors.New("connection_id must identify a connection in the captured workspace"))
		}
		captured, err := op.connection(id, tool.connection)
		if err != nil {
			return observation.Result{}, denyRead(err)
		}
		connection = captured
		delete(args, "connection_id")
	}
	return tool.read(ctx, connection, args)
}
