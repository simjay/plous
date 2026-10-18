package model

import (
	"errors"
	"github.com/cloudwego/eino/schema"
	"github.com/simjay/plous/internal/ai/agent"
)

// ErrNotDispatched marks a local rejection before a provider request starts.
var ErrNotDispatched = errors.New("model request was not dispatched")

type Tool = agent.Tool
type ToolCall = agent.ToolCall
type RequestMessage struct {
	Native     *schema.Message `json:"native,omitempty"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
}
type CallUsage struct {
	Reported bool     `json:"reported"`
	Input    int64    `json:"input_tokens"`
	Output   int64    `json:"output_tokens"`
	Total    int64    `json:"total_tokens"`
	Cost     *float64 `json:"cost_usd"`
}
type Response struct {
	Native    *schema.Message
	CallID    string `json:"call_id,omitempty"`
	Text      string
	Thinking  string
	ToolCalls []ToolCall
	Usage     CallUsage
}
