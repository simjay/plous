package source

import (
	"context"
	"fmt"
	"github.com/simjay/plous/internal/integrations/observation"
	"time"
)

// GuardedRead delivers selected source excerpts and file metadata, never an archive.
func GuardedRead(ctx context.Context, scope ReadScope, c ReadRequest) (any, error) {
	if c.Operation != "list" && c.Operation != "read" && c.Operation != "search" {
		return readView(ctx, scope, c)
	}
	page, err := readSourcePage(ctx, scope, c)
	if err != nil {
		return nil, err
	}
	r := observation.New("source.v1", c.Operation, time.Now().UTC())
	r.Connection = scope.ConnectionID
	if scope.View != nil && scope.View.inventory.Source.Config.Repository != "" {
		s := scope.View.inventory.Source
		r.ObservedAt, r.Coverage.Snapshot = s.Observed, s.Identity
		r.Facts["source_identity"], r.Facts["revision"] = observation.Text(s.Identity), observation.Text(s.Revision)
		if len(s.Gaps) > 0 {
			r.Partial("captured source contains omitted content")
		}
	}
	r.Selection = map[string]string{"path": c.Path, "query": c.Query, "glob": c.Glob, "regex": fmt.Sprint(c.Regex), "case_sensitive": fmt.Sprint(c.CaseSensitive == nil || *c.CaseSensitive)}
	r.Facts["path"] = observation.Text(c.Path)
	r.Facts["identity"] = observation.Text(page.Data.Identity)
	r.Facts["omitted"] = observation.Number(int64(page.Data.Omitted))
	switch c.Operation {
	case "read":
		r.Facts["excerpt"] = observation.Text(page.Data.Text)
		r.Facts["offset"] = observation.Number(int64(c.Offset))
		r.Facts["next_offset"] = observation.Number(int64(page.Data.NextOffset))
		r.Facts["total_bytes"] = observation.Number(int64(page.Data.TotalBytes))
	case "list":
		r.Facts["next_offset"] = observation.Number(int64(page.Data.NextOffset))
		for _, e := range page.Data.Entries {
			r.Rows = append(r.Rows, observation.Row{Identity: observation.Identity{Kind: "File", Name: e.Path}, Facts: map[string]observation.Value{"directory": observation.Boolean(e.Directory), "bytes": observation.Number(e.Size), "modified": observation.Text(e.Modified.Format(time.RFC3339Nano))}})
		}
	case "search":
		r.Facts["traversal_finished"] = observation.Boolean(page.Continuation["cursor"] == nil)
		r.Facts["query"] = observation.Text(c.Query)
		for _, m := range page.Data.Matches {
			r.Rows = append(r.Rows, observation.Row{Identity: observation.Identity{Kind: "Match", Name: m.Path, UID: fmt.Sprintf("%s/%s/%d", m.Path, m.Identity, m.Offset), Revision: m.Identity}, Facts: map[string]observation.Value{"line": observation.Number(int64(m.Line)), "excerpt": observation.Text(m.Text), "offset": observation.Number(int64(m.Offset)), "truncated": observation.Boolean(m.Truncated)}})
		}
	}
	for k, v := range page.Continuation {
		switch x := v.(type) {
		case string:
			r.Facts["continue_"+k] = observation.Text(x)
		case int:
			r.Facts["continue_"+k] = observation.Number(int64(x))
		}
	}
	r.Coverage.Observed = len(r.Rows)
	if page.Complete && c.Offset == 0 && c.Cursor == "" && page.Data.Omitted == 0 && r.Outcome == "completed" {
		r.Complete()
	} else {
		r.Partial("selected source page; excluded or unvisited content is not covered")
	}
	if page.Notice != "" {
		r.Coverage.Limitations = append(r.Coverage.Limitations, page.Notice)
	}
	return r, r.Validate()
}
