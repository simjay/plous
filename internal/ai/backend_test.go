package ai

import (
	"errors"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/simjay/plous/internal/ai/agent"
)

func TestEinoBackendUsesCommonEventsAndStopsOnEmissionFailure(t *testing.T) {
	for _, reject := range []string{"", "context", "text"} {
		t.Run("reject-"+reject, func(t *testing.T) {
			options, _ := workspaceFixture(t)
			scripted := &scriptedModel{steps: []*schema.Message{answer("Answer")}}
			op := &operation{o: options, model: scripted, runtime: DefaultRuntimeConfig(), capacity: 200000}
			if err := op.setupHistory(); err != nil {
				t.Fatal(err)
			}
			session := newToolSession(t.Context(), op)
			defer session.Close()
			sentinel := errors.New("host stopped delivery")
			var text string
			contextSeen := false
			turn := agent.Turn{ID: options.TurnID, System: "Captured instructions", Prompt: options.Prompt}
			result, err := (einoBackend{op}).Run(t.Context(), turn, session, func(e agent.Event) error {
				if e.TurnID != turn.ID {
					t.Fatal("foreign event", e)
				}
				if e.ContextLimit > 0 {
					contextSeen = true
					if reject == "context" {
						return sentinel
					}
				} else if e.Progress == "" {
					text = e.Text
					if reject == "text" {
						return sentinel
					}
				}
				return nil
			})
			if !contextSeen {
				t.Fatal("context update bypassed backend events")
			}
			if reject != "" {
				if !errors.Is(err, sentinel) || result.Status != "failed" {
					t.Fatal(result, err)
				}
				if reject == "context" && len(scripted.inputs) != 0 {
					t.Fatal("dispatched after rejected context update")
				}
			} else if err != nil || result.Text != "Answer" || text != "Answer" || scripted.inputs[0][0].Content != turn.System {
				t.Fatal(result, text, err)
			}
		})
	}
}

func TestEinoBackendUsageKeepsUnknownDistinctFromZero(t *testing.T) {
	for _, reported := range []bool{false, true} {
		options, _ := workspaceFixture(t)
		reply := answer("Answer")
		if reported {
			reply.ResponseMeta.Usage = &schema.TokenUsage{}
		}
		scripted := &scriptedModel{steps: []*schema.Message{reply}}
		op := &operation{o: options, model: scripted, runtime: DefaultRuntimeConfig(), capacity: 200000}
		if err := op.setupHistory(); err != nil {
			t.Fatal(err)
		}
		session := newToolSession(t.Context(), op)
		result, err := (einoBackend{op}).Run(t.Context(), agent.Turn{ID: options.TurnID, System: "Instructions", Prompt: options.Prompt}, session, func(agent.Event) error { return nil })
		session.Close()
		if err != nil || result.Usage.Reported != reported || result.Usage.Total != 0 {
			t.Fatal(reported, result, err)
		}
	}
}
