package workspace

import (
	"context"
	"encoding/json"
	"github.com/simjay/plous/internal/integrations/observation"
	"time"
)

func (m *Reader) Project(ctx context.Context, q Query) (observation.Result, error) {
	r := observation.New("workspace.metadata.v1", q.Source, time.Now().UTC())
	p, err := m.Traverse(ctx, q)
	if err != nil {
		return r, err
	}
	// Only documented metadata fields cross the assistant boundary.
	for _, record := range p.Records {
		var fields map[string]any
		if err = json.Unmarshal([]byte(record.Text), &fields); err != nil {
			return r, err
		}
		facts := map[string]observation.Value{}
		for _, key := range []string{"id", "name", "title", "kind", "status", "tool", "operation", "namespace", "created", "updated", "release", "chart", "version", "source_time", "observed_at"} {
			if text, ok := fields[key].(string); ok {
				facts[key] = observation.Text(text)
			}
		}
		if revision, ok := fields["revision"].(float64); ok {
			facts["revision"] = observation.Number(int64(revision))
		}
		r.Rows = append(r.Rows, observation.Row{Identity: observation.Identity{Kind: record.Source, Name: record.Ref}, Facts: facts})
	}
	r.Coverage.Observed = len(r.Rows)
	r.Facts["next_cursor"] = observation.Text(p.NextCursor)
	if p.Complete && q.Cursor == "" {
		r.Complete()
	} else {
		r.Partial("selected workspace metadata page")
	}
	return r, r.Validate()
}
