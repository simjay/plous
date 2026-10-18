package ai

import (
	"context"
	"errors"

	"github.com/simjay/plous/internal/ai/agent"
	aimodel "github.com/simjay/plous/internal/ai/model"
)

type einoBackend struct{ op *operation }

var _ agent.Backend = einoBackend{}

func (b einoBackend) Run(ctx context.Context, turn agent.Turn, tools agent.Tools, emit agent.Emit) (agent.Result, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(context.Canceled)
	progress, contextUsage, text := b.op.o.Progress, b.op.o.ContextUsage, b.op.o.Emit
	defer func() { b.op.o.Progress, b.op.o.ContextUsage, b.op.o.Emit = progress, contextUsage, text }()
	b.op.o.Progress = func(text string) {
		if err := emit(agent.Event{TurnID: turn.ID, Progress: text}); err != nil {
			cancel(err)
		}
	}
	b.op.o.ContextUsage = func(used, limit int) {
		if err := emit(agent.Event{TurnID: turn.ID, ContextUsed: used, ContextLimit: limit}); err != nil {
			cancel(err)
		}
	}
	b.op.o.Emit = func(text string) error { return emit(agent.Event{TurnID: turn.ID, Text: text}) }
	result, err := b.op.runEino(ctx, turn, tools)
	if cause := context.Cause(ctx); cause != nil {
		err = cause
	}
	status := "completed"
	if err != nil {
		status = "failed"
	}
	return agent.Result{Text: result.Text, Status: status, Usage: agent.Usage{Reported: b.op.calls > 0 && b.op.usageReports == b.op.calls, Input: result.Usage.Input, Output: result.Usage.Output, Total: result.Usage.Total}}, err
}

func (op *operation) run(ctx context.Context) (result aimodel.Result, err error) {
	if err = op.setupHistory(); err != nil {
		return result, err
	}
	session := newToolSession(ctx, op)
	defer session.Close()
	system := op.o.System
	if system == "" {
		system = Instructions
	}
	turn := agent.Turn{ID: op.o.TurnID, System: system + "\nCaptured connection and current page:\n" + op.o.ScopeContext + "\n" + string(marshal(op.o.Connections)), Prompt: op.o.Prompt, MaxCalls: op.runtime.MaxSteps, MaxTokens: op.runtime.MaxTokens}
	backend := einoBackend{op}
	emitText, emitProgress, emitContext := op.o.Emit, op.o.Progress, op.o.ContextUsage
	out, err := backend.Run(ctx, turn, session, func(event agent.Event) error {
		if event.TurnID != turn.ID {
			return errors.New("agent emitted an event for another turn")
		}
		if event.Progress != "" {
			if emitProgress != nil {
				emitProgress(event.Progress)
			}
			return nil
		}
		if event.ContextLimit > 0 {
			if emitContext != nil {
				emitContext(event.ContextUsed, event.ContextLimit)
			}
			return nil
		}
		return emitText(op.redact(event.Text))
	})
	result.Text = op.redact(out.Text)
	result.Usage = aimodel.Usage{Input: out.Usage.Input, Output: out.Usage.Output, Total: out.Usage.Total}
	if err == nil && out.Status != "completed" {
		err = errors.New("agent ended without a completed response")
	}
	if err == nil {
		result.Finish = "stop"
	}
	return result, err
}
