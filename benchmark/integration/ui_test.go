package integration

import (
	"context"
	"encoding/json"

	bench "github.com/simjay/plous/benchmark"

	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/app/navigation"
)

// The display endpoint substitutes for a terminal. Acquisition is the same
// production navigation loader; application tests verify the actual widgets.
type fixtureUI struct {
	fixture  *fixture
	state    agent.UIResult
	blocked  bool
	prepared *navigation.Prepared
}

func (u *fixtureUI) Read(ctx context.Context) (agent.UIResult, error) { return u.state, ctx.Err() }
func (u *fixtureUI) Show(ctx context.Context, q agent.ViewRequest) (agent.UIResult, error) {
	if u.blocked {
		return agent.UIResult{Status: "blocked", Reason: "unfinished editor", Focus: "assistant"}, nil
	}
	profile, err := u.fixture.options.Store.DataRoot()
	if err != nil {
		return agent.UIResult{}, err
	}
	p, err := navigation.Load(ctx, q, u.fixture.options.Connections, profile)
	if err != nil {
		return agent.UIResult{Status: "failed", Reason: err.Error()}, nil
	}
	u.prepared = p
	view := p.Request
	view.Evidence = nil
	u.state = agent.UIResult{Status: "ready", View: &view, Focus: "assistant", Visible: true, Origin: p.Origin}
	result := u.state
	result.Status = "opened"
	return result, nil
}

func (f *fixture) display() json.RawMessage {
	if f.ui == nil || f.ui.prepared == nil {
		return nil
	}
	p := f.ui.prepared
	return bench.JSON(map[string]any{"before": p.Before, "after": p.After, "text": p.Text})
}
