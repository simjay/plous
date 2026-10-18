package ai

import (
	aimodel "github.com/simjay/plous/internal/ai/model"
	"time"
)

// Observation describes one provider boundary. Start may reject a request before
// dispatch. Finish runs once for each dispatched call, including errors. Inputs
// are redacted with the operation's captured credentials before observation.
type Observation struct {
	TurnID          string
	ConversationID  string
	CallID          string
	Component       string
	Phase           string
	Input           string
	MaxOutputTokens int
	Usage           aimodel.CallUsage
	Elapsed         time.Duration
	Failed          bool
}

func (op *operation) observe(e Observation) error {
	if op.o.Observe == nil {
		return nil
	}
	e.Input = op.redact(e.Input)
	return op.o.Observe(e)
}
