package workspace

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
)

type traversalCursor struct {
	Nonce  string `json:"nonce"`
	Source string `json:"source"`
	Index  int    `json:"index"`
}

// Traverse exposes captured workspace and Activity records. Conversation recall
// belongs to the scoped history tools.
func (m *Reader) Traverse(ctx context.Context, q Query) (Page, error) {
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	if q.Limit == 0 {
		q.Limit = DefaultResults
	}
	if q.Source != "workspace" && q.Source != "activity" {
		return Page{}, errors.New("workspace traversal requires workspace or activity")
	}
	if q.Limit < 1 || q.Limit > MaxResults || len(q.Cursor) > 2048 {
		return Page{}, errors.New("invalid chronological read limits")
	}
	c := traversalCursor{Nonce: m.nonce, Source: q.Source}
	if q.Cursor != "" {
		encoded, err := base64.RawURLEncoding.DecodeString(q.Cursor)
		if err != nil || json.Unmarshal(encoded, &c) != nil || c.Nonce != m.nonce || c.Source != q.Source || c.Index < 0 {
			return Page{}, errors.New("invalid chronological cursor")
		}
	}
	var sources []source
	for _, s := range m.sources {
		if s.kind == q.Source {
			sources = append(sources, s)
		}
	}
	if q.Source == "activity" {
		for i, j := 0, len(sources)-1; i < j; i, j = i+1, j-1 {
			sources[i], sources[j] = sources[j], sources[i]
		}
	}
	if c.Index > len(sources) {
		return Page{}, errors.New("invalid chronological cursor")
	}
	p := Page{Records: []Record{}, State: "ready"}
	for c.Index < len(sources) && len(p.Records) < q.Limit {
		data, ref, _, _, err := m.readSource(ctx, sources[c.Index], 0)
		if err != nil {
			return Page{}, err
		}
		p.Records = append(p.Records, Record{Ref: ref, Source: q.Source, Text: string(data)})
		p.Scanned++
		c.Index++
	}
	p.Complete = c.Index == len(sources)
	p.Truncated = !p.Complete
	if !p.Complete {
		encoded, _ := json.Marshal(c)
		p.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	return p, nil
}
