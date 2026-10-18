package ai

import (
	"context"
	"errors"
	"github.com/simjay/plous/internal/ai/agent"
	aimodel "github.com/simjay/plous/internal/ai/model"
)

func (op *operation) runEino(ctx context.Context, turn agent.Turn, tools agent.Tools) (result aimodel.Result, err error) {
	defer func() { op.mu.Lock(); defer op.mu.Unlock(); result.Usage = op.usage }()
	messages, err := op.o.History.AgentMessages(ctx, op.o.WorkspaceID, op.o.ConversationID)
	if err != nil {
		return result, err
	}
	// The host owns the system prefix. Checkpoints hold exchanges only.
	messages = append(messages, aimodel.RequestMessage{Role: "user", Content: turn.Prompt})
	definitions := tools.Definitions()
	for {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		messages, err = op.fitContext(ctx, turn.System, messages, definitions)
		if err != nil {
			return result, err
		}
		input := append([]aimodel.RequestMessage{{Role: "system", Content: turn.System}}, messages...)
		response, err := op.chat(ctx, input, definitions, true)
		if err != nil {
			return result, err
		}
		messages = append(messages, aimodel.RequestMessage{Role: "assistant", Content: response.Text, ToolCalls: response.ToolCalls, Native: response.Native})
		if len(response.ToolCalls) == 0 {
			result.Text, result.Finish = response.Text, "stop"
			return result, op.o.History.SaveMessages(op.o.WorkspaceID, op.o.ConversationID, op.o.TurnID, messages)
		}
		seen := map[string]bool{}
		for _, call := range response.ToolCalls {
			if seen[call.ID] {
				return result, errors.New("model reused a tool-call identity")
			}
			seen[call.ID] = true
			deliveryBytes := min(65536, max(0, op.capacity-aimodel.MaxOutputTokens-requestTokens(turn.System, messages, definitions)))
			body, err := tools.Call(ctx, call, deliveryBytes)
			if err != nil {
				return result, err
			}
			messages = append(messages, aimodel.RequestMessage{Role: "tool", ToolCallID: call.ID, Content: string(body)})
		}
	}
}
