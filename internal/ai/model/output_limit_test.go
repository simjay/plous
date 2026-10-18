package model

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestProviderOutputLimitsUseSupportedWireFields(t *testing.T) {
	for _, provider := range Providers() {
		for _, method := range []string{"Stream", "Generate"} {
			for _, bound := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/bound=%v", provider, method, bound), func(t *testing.T) {
					bodies := make(chan map[string]any, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
						}
						bodies <- body
						writeLimitResponse(w, provider, method == "Stream")
					}))
					defer server.Close()
					config := DefaultConfig()
					config.Provider, config.Model, config.Endpoint, config.KeyEnv = provider, Models(provider)[0].ID, server.URL, ""
					client, err := NewChatModel(t.Context(), config, "")
					if err != nil {
						t.Fatal(err)
					}
					if bound {
						tool, err := toolInfo(Tool{Name: "synthetic_read", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)})
						if err != nil {
							t.Fatal(err)
						}
						client, err = client.WithTools([]*schema.ToolInfo{tool})
						if err != nil {
							t.Fatal(err)
						}
					}
					// The final default call checks that overrides never change the captured model.
					for _, limit := range []int{0, 8192, 137, 0} {
						backing := []einomodel.Option{einomodel.WithMaxTokens(limit), einomodel.WithTemperature(0.25), einomodel.WithTopP(0.75)}
						var options []einomodel.Option
						if limit != 0 {
							options = backing[:1]
						}
						messages := []*schema.Message{{Role: schema.User, Content: "Synthetic request."}}
						if method == "Generate" {
							_, err = client.Generate(t.Context(), messages, options...)
						} else {
							var stream *schema.StreamReader[*schema.Message]
							stream, err = client.Stream(t.Context(), messages, options...)
							if err == nil {
								for {
									_, err = stream.Recv()
									if err != nil {
										break
									}
								}
								stream.Close()
								if err == io.EOF {
									err = nil
								}
							}
						}
						if err != nil {
							t.Fatal(err)
						}
						body := <-bodies
						field, absent := "max_completion_tokens", "max_tokens"
						if provider == "anthropic" {
							field, absent = absent, field
						}
						want := limit
						if want == 0 {
							want = MaxOutputTokens
						}
						if body[field] != float64(want) {
							t.Errorf("override %d: %s = %v, want %d", limit, field, body[field], want)
						}
						if value, ok := body[absent]; ok {
							t.Errorf("override %d: unexpected %s = %v", limit, absent, value)
						}
						if bound {
							tools, ok := body["tools"].([]any)
							if !ok || len(tools) != 1 {
								t.Error("output limit translation lost bound tools")
							}
						}
						original := einomodel.GetCommonOptions(nil, backing...)
						if original.MaxTokens == nil || *original.MaxTokens != limit || original.Temperature == nil || *original.Temperature != 0.25 || original.TopP == nil || *original.TopP != 0.75 {
							t.Error("request option slice was mutated")
						}
					}
				})
			}
		}
	}
}

func writeLimitResponse(w http.ResponseWriter, provider string, stream bool) {
	if !stream {
		w.Header().Set("Content-Type", "application/json")
		if provider == "anthropic" {
			fmt.Fprint(w, `{"id":"fixture","type":"message","role":"assistant","model":"fixture","content":[{"type":"text","text":"Done."}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`)
		} else {
			fmt.Fprint(w, `{"id":"fixture","object":"chat.completion","created":1,"model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"Done."},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
		}
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	if provider == "anthropic" {
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"fixture\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"fixture\",\"content\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	} else {
		chunk(w, map[string]any{"role": "assistant", "content": "Done."}, nil)
		chunk(w, map[string]any{}, "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}
}
