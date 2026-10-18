package model

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

// PreparedRequest contains the exact provider input used for dispatch and observation.
type PreparedRequest struct {
	Messages []*schema.Message
	Tools    []*schema.ToolInfo
}

func Prepare(messages []RequestMessage, definitions []Tool) (*PreparedRequest, error) {
	input := make([]*schema.Message, 0, len(messages))
	for _, m := range messages {
		message := &schema.Message{Role: schema.RoleType(m.Role), Content: m.Content, ToolCallID: m.ToolCallID}
		for _, call := range m.ToolCalls {
			message.ToolCalls = append(message.ToolCalls, schema.ToolCall{ID: call.ID, Type: "function", Function: schema.FunctionCall{Name: call.Name, Arguments: string(call.Args)}})
		}
		if m.Native != nil {
			message = m.Native
		}
		input = append(input, message)
	}
	var tools []*schema.ToolInfo
	for _, d := range definitions {
		info, err := toolInfo(d)
		if err != nil {
			return nil, err
		}
		tools = append(tools, info)
	}

	return &PreparedRequest{Messages: input, Tools: tools}, nil
}

func toolInfo(d Tool) (*schema.ToolInfo, error) {
	var params jsonschema.Schema
	if err := json.Unmarshal(d.Parameters, &params); err != nil {
		return nil, err
	}
	return &schema.ToolInfo{Name: d.Name, Desc: d.Description, ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&params)}, nil
}

// Stream validates completion and retains reported usage even when a stream fails.
// emit receives accumulated text; the caller owns redaction before displaying it.
func (r *PreparedRequest) Stream(ctx context.Context, client model.ToolCallingChatModel, emit func(string) error, options ...model.Option) (result Response, err error) {
	var parts []*schema.Message
	defer func() {
		if err != nil && len(parts) > 0 {
			if merged, e := schema.ConcatMessages(parts); e == nil && merged.ResponseMeta != nil && merged.ResponseMeta.Usage != nil {
				u := merged.ResponseMeta.Usage
				result.Usage = CallUsage{Reported: true, Input: int64(u.PromptTokens), Output: int64(u.CompletionTokens), Total: int64(u.TotalTokens)}
			}
		}
	}()
	s, err := client.Stream(ctx, r.Messages, append(options, model.WithTools(r.Tools))...)
	if err != nil {
		return result, SafeError(ctx, err)
	}
	defer s.Close()
	var output strings.Builder
	for {
		msg, err := s.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, SafeError(ctx, err)
		}
		parts = append(parts, msg)
		output.WriteString(msg.Content)
		if emit != nil {
			if err = emit(output.String()); err != nil {
				return result, err
			}
		}
	}
	if len(parts) == 0 {
		return result, errors.New("model returned no response")
	}
	msg, err := schema.ConcatMessages(parts)
	if err != nil {
		return result, errors.New("invalid streamed model response")
	}
	if msg.ResponseMeta == nil {
		return result, errors.New("model response did not complete")
	}
	finish := msg.ResponseMeta.FinishReason
	if len(msg.ToolCalls) > 0 {
		if finish != "tool_calls" && finish != "tool_use" {
			return result, errors.New("model tool calls did not complete")
		}
	} else if !CompletedFinish(finish) {
		return result, errors.New("model response did not complete")
	}
	result.Native = msg
	result.Text, result.Thinking = msg.Content, msg.ReasoningContent
	for _, c := range msg.ToolCalls {
		if c.ID == "" || c.Function.Name == "" {
			return result, errors.New("model tool call is missing its ID or name")
		}
		result.ToolCalls = append(result.ToolCalls, ToolCall{ID: c.ID, Name: c.Function.Name, Args: []byte(c.Function.Arguments)})
	}
	if msg.ResponseMeta.Usage != nil {
		u := msg.ResponseMeta.Usage
		result.Usage = CallUsage{Reported: true, Input: int64(u.PromptTokens), Output: int64(u.CompletionTokens), Total: int64(u.TotalTokens)}
	}

	return result, nil
}
