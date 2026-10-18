package ai

import (
	"errors"
	aimodel "github.com/simjay/plous/internal/ai/model"
)

// Main calls and context compaction share the turn allowance.
func (op *operation) reserve(input, output int) (func(aimodel.Response) error, error) {
	op.mu.Lock()
	defer op.mu.Unlock()
	if op.runtime.MaxSteps > 0 && op.calls >= op.runtime.MaxSteps {
		return nil, errors.New("turn model-call budget reached")
	}
	amount := input + output
	if op.runtime.MaxTokens > 0 && op.spent+op.reserved+amount > op.runtime.MaxTokens {
		return nil, errors.New("turn token budget cannot cover another model call")
	}
	op.calls++
	op.reserved += amount
	return func(r aimodel.Response) error {
		used := input + estimateText(r.Text) + estimateText(r.Thinking) + estimateText(string(marshal(r.ToolCalls)))
		if r.Usage.Reported {
			used = max(used, int(r.Usage.Total), int(r.Usage.Input+r.Usage.Output))
		}
		op.mu.Lock()
		op.reserved -= amount
		op.spent += used
		over := op.runtime.MaxTokens > 0 && op.spent > op.runtime.MaxTokens
		op.mu.Unlock()
		if over {
			return errors.New("turn token budget exceeded by provider usage")
		}
		return nil
	}, nil
}
