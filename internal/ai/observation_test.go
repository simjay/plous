package ai

import (
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestObserverRejectsCallBeforeDispatch(t *testing.T) {
	o, _ := fixture(t)
	nativeOptions(t, &o)
	script := &scriptedModel{steps: []*schema.Message{answer("should not run")}}
	o.Model = script
	o.Observe = func(e Observation) error {
		if e.Phase == "start" {
			return errors.New("observation rejected")
		}
		return nil
	}
	if _, err := Run(t.Context(), o); err == nil || len(script.inputs) != 0 {
		t.Fatal("pre-dispatch ceiling failed", len(script.inputs), err)
	}
}

func TestCapturedKeyIsRedacted(t *testing.T) {
	o, _ := fixture(t)
	nativeOptions(t, &o)
	o.Key = "private-test-key"
	o.Model = &scriptedModel{steps: []*schema.Message{answer("private-test-key")}}
	r, err := Run(t.Context(), o)
	if err != nil || strings.Contains(r.Text, o.Key) || r.Text != "[redacted]" {
		t.Fatal("Captured credential reached the reply", err)
	}
}
