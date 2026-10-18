package ai

import (
	"context"
	"errors"
	"fmt"
	journal "github.com/simjay/plous/internal/ai/history"
	"strings"
	"time"

	aimodel "github.com/simjay/plous/internal/ai/model"
)

func (op *operation) chat(ctx context.Context, messages []aimodel.RequestMessage, definitions []aimodel.Tool, emit bool) (result aimodel.Response, err error) {
	request, err := aimodel.Prepare(messages, definitions)
	if err != nil {
		return result, err
	}
	input := request.Messages
	if emit && op.o.ContextUsage != nil {
		op.o.ContextUsage(estimateText(string(marshal(request.Messages)))+EstimateTools(definitions), op.capacity)
	}
	if err := ctx.Err(); err != nil {
		return result, errors.Join(aimodel.ErrNotDispatched, err)
	}
	component := "compaction"
	if emit {
		component = "main"
	}
	callID := fmt.Sprintf("%s/%s-%d", op.o.TurnID, component, op.callSequence.Add(1))
	result.CallID = callID
	payload := op.redact(string(marshal(map[string]any{"messages": input, "tools": definitions})))
	outputLimit := aimodel.MaxOutputTokens
	capacity := op.capacity
	if capacity > 0 && estimateText(payload)+outputLimit > capacity {
		return result, errors.Join(aimodel.ErrNotDispatched, errors.New("request exceeds selected model context capacity"))
	}
	release, budgetErr := op.reserve(estimateText(payload), outputLimit)
	if budgetErr != nil {
		return result, errors.Join(aimodel.ErrNotDispatched, budgetErr)
	}
	defer func() { err = errors.Join(err, release(result)) }()
	if op.o.Observe != nil {
		if err = op.observe(Observation{CallID: callID, TurnID: op.o.TurnID, ConversationID: op.o.ConversationID, Component: component, Phase: "start", Input: payload, MaxOutputTokens: outputLimit}); err != nil {
			return result, errors.Join(aimodel.ErrNotDispatched, err)
		}
	}
	started := time.Now()
	defer func() {
		elapsed := time.Since(started)
		if op.o.History != nil {
			err = errors.Join(err, op.o.History.SaveUsage(op.o.WorkspaceID, op.o.ConversationID, op.o.TurnID, journal.CallRecord{CallID: callID, Component: component, Usage: result.Usage, ElapsedMS: elapsed.Milliseconds(), Failed: err != nil}))
		}
		op.mu.Lock()
		op.usage.Input += result.Usage.Input
		op.usage.Output += result.Usage.Output
		op.usage.Total += result.Usage.Total
		if result.Usage.Reported {
			op.usageReports++
		}
		op.mu.Unlock()
		err = errors.Join(err, op.observe(Observation{CallID: callID, TurnID: op.o.TurnID, ConversationID: op.o.ConversationID, Component: component, Phase: "finish", Usage: result.Usage, Elapsed: elapsed, Failed: err != nil}))
	}()

	var emitText func(string) error
	if emit {
		emitText = func(output string) error {
			safe := op.redact(output)
			for _, key := range op.keys {
				for n := min(len(safe), len(key)-1); n > 0; n-- {
					if strings.HasSuffix(safe, key[:n]) {
						safe = safe[:len(safe)-n]
						break
					}
				}
			}
			return op.o.Emit(safe)
		}
	}
	result, err = request.Stream(ctx, op.model, emitText)
	result.CallID = callID
	result.Text, result.Thinking = op.redact(result.Text), op.redact(result.Thinking)
	for i := range result.ToolCalls {
		safe, redactErr := op.redactArguments(result.ToolCalls[i].Args)
		if redactErr != nil {
			safe = []byte("null")
		}
		result.ToolCalls[i].Args = safe
	}
	if result.Native != nil {
		result.Native.Content, result.Native.ReasoningContent = result.Text, result.Thinking
		for i := range result.Native.ToolCalls {
			safe, redactErr := op.redactArguments([]byte(result.Native.ToolCalls[i].Function.Arguments))
			if redactErr != nil {
				safe = []byte("null")
			}
			result.Native.ToolCalls[i].Function.Arguments = string(safe)
		}
		serialized := marshal(result.Native.Extra)
		safe, redactErr := op.redactJSON(serialized)
		if redactErr != nil || string(safe) != string(serialized) {
			return result, errors.New("provider continuation contains a captured credential or invalid JSON")
		}
	}
	return result, err
}
