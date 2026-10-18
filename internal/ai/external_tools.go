package ai

import (
	"context"
	"errors"
	"sync"

	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/workspace"
)

// ExternalTools binds a native terminal session to one evidence journal and scope.
// Native terminal output is never interpreted as conversation or tool protocol.
type ExternalTools struct {
	agent.Tools
	Context context.Context
	ID      string
	close   func(error) error
}

func (s *ExternalTools) Close() error             { return s.close(s.Context.Err()) }
func (s *ExternalTools) Finish(cause error) error { return s.close(cause) }

func OpenExternalTools(ctx context.Context, o Options) (*ExternalTools, error) {
	if o.Store == nil || o.History == nil || o.Agent.Effective() == agent.Plous || !o.Agent.Valid() {
		return nil, errors.New("external tool session requires storage and an installed agent")
	}
	o.Connections = append([]Connection(nil), o.Connections...)
	op := &operation{o: o}
	var err error
	op.runtime, err = loadRuntimeConfig(o.Store, o.Runtime)
	if err != nil {
		return nil, err
	}
	op.records, err = workspace.NewReader(o.Store, o.WorkspaceID)
	if err != nil {
		return nil, err
	}
	conversation, err := o.History.NewConversation(o.WorkspaceID, o.Agent.Label()+" tool evidence")
	if err != nil {
		return nil, err
	}
	turn, err := o.History.StartTurn(o.WorkspaceID, conversation.ID, history.Request{Backend: o.Agent, Role: "external-tools", Prompt: "Plous opened a " + o.Agent.Label() + " tool session.", Context: o.ScopeContext})
	if err != nil {
		return nil, err
	}
	op.o.ConversationID, op.o.TurnID = conversation.ID, turn.Request.ID
	finish := func(cause error) error {
		status, detail := "completed", ""
		if errors.Is(cause, context.Canceled) {
			status = "cancelled"
		} else if cause != nil {
			status, detail = "error", op.redact(cause.Error())
		}
		return o.History.SaveResponse(o.WorkspaceID, conversation.ID, turn.Request.ID, history.Response{Status: status, Error: detail, Text: "Tool session closed. The conversation is managed by " + o.Agent.Label() + ". Sources contains saved Plous tool observations only."})
	}
	if err = op.setupHistory(); err != nil {
		_ = finish(err)
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	session := newToolSession(ctx, op)
	var once sync.Once
	var saveMu sync.Mutex
	var saved bool
	var originalCause error
	return &ExternalTools{Tools: session, Context: ctx, ID: turn.Request.ID, close: func(cause error) error {
		saveMu.Lock()
		defer saveMu.Unlock()
		once.Do(func() { originalCause = cause; cancel(); session.Close() })
		if saved {
			return nil
		}
		err := finish(originalCause)
		saved = err == nil
		return err
	}}, nil
}
