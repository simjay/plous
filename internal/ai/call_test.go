package ai

import (
	aimodel "github.com/simjay/plous/internal/ai/model"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestIncompleteToolCallsNeverReachDispatch(t *testing.T) {
	for _, finish := range []string{"length", "max_tokens", "content_filter", "stop", ""} {
		t.Run(finish, func(t *testing.T) {
			message := call("workspace_read", `{"purpose":"inspect","operation":"workspace"}`)
			message.ResponseMeta.FinishReason = finish
			op := &operation{model: &scriptedModel{steps: []*schema.Message{message}}}
			if _, err := op.chat(t.Context(), nil, nil, false); err == nil {
				t.Fatal("accepted incomplete tool call")
			}
		})
	}
}

func TestFailedModelCompletionRetainsReportedUsageAndRedactsObservation(t *testing.T) {
	message := answer("partial answer")
	message.ResponseMeta.FinishReason = "length"
	message.ResponseMeta.Usage = &schema.TokenUsage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}
	var events []Observation
	op := &operation{model: &scriptedModel{steps: []*schema.Message{message}}, keys: []string{"test-private-key"}, o: Options{Observe: func(e Observation) error { events = append(events, e); return nil }}}
	response, err := op.chat(t.Context(), []aimodel.RequestMessage{{Role: "user", Content: "test-private-key"}}, nil, false)
	if err == nil || !response.Usage.Reported || response.Usage.Total != 18 || op.usage.Total != 18 {
		t.Fatalf("failed usage lost: %+v %v", response, err)
	}
	if len(events) != 2 || events[0].Phase != "start" || events[1].Phase != "finish" || !events[1].Failed || strings.Contains(events[0].Input, "test-private-key") {
		t.Fatal("invalid or unsafe observation", events)
	}
}
