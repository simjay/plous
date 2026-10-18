package ai

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	aimodel "github.com/simjay/plous/internal/ai/model"
)

type chunkedResponseModel struct {
	parts []*schema.Message
	calls int
}

func (m *chunkedResponseModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func (m *chunkedResponseModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("fixture requires streaming")
}

func (m *chunkedResponseModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.calls++
	return schema.StreamReaderFromArray(m.parts), nil
}

func TestStreamingCredentialsStayHiddenAcrossChunkBoundaries(t *testing.T) {
	client := &chunkedResponseModel{parts: []*schema.Message{
		{Role: schema.Assistant, Content: "prefix test-"},
		{Role: schema.Assistant, Content: "private-"},
		{Role: schema.Assistant, Content: "key then second-"},
		{Role: schema.Assistant, Content: "private-key done", ResponseMeta: &schema.ResponseMeta{FinishReason: "stop"}},
	}}
	var emitted []string
	op := &operation{model: client, keys: []string{"test-private-key", "second-private-key"}, o: Options{Emit: func(text string) error {
		emitted = append(emitted, text)
		return nil
	}}}
	response, err := op.chat(t.Context(), nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"prefix ", "prefix ", "prefix [redacted] then ", "prefix [redacted] then [redacted] done"}
	if !reflect.DeepEqual(emitted, expected) || response.Text != expected[len(expected)-1] || client.calls != 1 {
		t.Fatalf("stream exposed a credential prefix or lost safe text: %#v, %+v", emitted, response)
	}
}

func TestEmitFailurePreservesUsageAndFinishesObservation(t *testing.T) {
	client := &chunkedResponseModel{parts: []*schema.Message{
		{Role: schema.Assistant, Content: "partial", ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}}},
		{Role: schema.Assistant, Content: " never emitted", ResponseMeta: &schema.ResponseMeta{FinishReason: "stop"}},
	}}
	stopped := errors.New("display stopped")
	var events []Observation
	op := &operation{model: client, o: Options{
		TurnID: "turn",
		Emit:   func(string) error { return stopped },
		Observe: func(event Observation) error {
			events = append(events, event)
			return nil
		},
	}}
	response, err := op.chat(t.Context(), nil, nil, true)
	if !errors.Is(err, stopped) || errors.Is(err, aimodel.ErrNotDispatched) || !response.Usage.Reported || response.Usage.Total != 18 || op.usage.Total != 18 {
		t.Fatalf("emission failure changed dispatch or usage: %+v, %+v, %v", response, op.usage, err)
	}
	if client.calls != 1 || len(events) != 2 || events[0].Phase != "start" || events[1].Phase != "finish" || !events[1].Failed || events[1].Usage != response.Usage || events[1].CallID != response.CallID || !strings.HasPrefix(response.CallID, "turn/main-") {
		t.Fatalf("dispatched failure lost its completion observation: %+v", events)
	}
}
