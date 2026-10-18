package workspace

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	MaxResults     = 20
	DefaultResults = 10
	MaxReadBytes   = 16 << 10
)

type Query struct {
	Source string `json:"source,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type Record struct {
	Ref       string `json:"ref"`
	Source    string `json:"source"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated,omitempty"`
}

type Page struct {
	State      string   `json:"state"`
	Complete   bool     `json:"complete"`
	Records    []Record `json:"records"`
	NextCursor string   `json:"next_cursor,omitempty"`
	Truncated  bool     `json:"truncated"`
	Scanned    int      `json:"scanned"`
}

type Chunk struct {
	Ref        string `json:"ref"`
	Text       string `json:"text"`
	NextOffset int    `json:"next_offset"`
	TotalBytes int    `json:"total_bytes"`
	Truncated  bool   `json:"truncated"`
}

type source struct {
	kind     string
	activity string
	data     []byte
}

type Reader struct {
	store       *Store
	workspaceID string
	sources     []source
	nonce       string
}

// NewReader captures workspace revisions and Activity records at operation start.
func NewReader(store *Store, workspaceID string) (*Reader, error) {
	if store == nil {
		return nil, errors.New("workspace reader requires an open profile")
	}
	m := &Reader{store: store, workspaceID: workspaceID, nonce: rand.Text()}
	var activities []source
	if IsCurrentScope(workspaceID) || IsConnectionScope(workspaceID) {
		store.mu.Lock()
		scope, err := store.readScope(workspaceID)
		store.mu.Unlock()
		if err != nil {
			return nil, err
		}
		data, _ := json.Marshal(scope.Connection)
		m.sources = append(m.sources, source{kind: "workspace", activity: "connection", data: data})
		owner := scope.WorkspaceID
		if IsCurrentScope(workspaceID) {
			owner = workspaceID
		}
		ops, err := store.Operations(owner)
		if err != nil {
			return nil, err
		}
		for _, op := range ops {
			if op.ConnectionID == scope.Connection.ID {
				data, _ := json.Marshal(op)
				m.sources = append(m.sources, source{kind: "activity", activity: "operation-" + op.ID, data: data})
			}
		}
		return m, nil
	}
	if workspaceID != "" {
		refs, err := store.ActivityReferences(workspaceID)
		if err != nil {
			return nil, err
		}
		if len(refs) > 0 {
			latest, err := store.ReadActivity(workspaceID, refs[len(refs)-1])
			if err != nil {
				return nil, err
			}
			data, err := json.Marshal(safeWorkspace(latest.After))
			if err != nil {
				return nil, err
			}
			m.sources = append(m.sources, source{kind: "workspace", activity: refs[len(refs)-1], data: data})
		}
		ops, err := store.Operations(workspaceID)
		if err != nil {
			return nil, err
		}
		for _, op := range ops {
			data, _ := json.Marshal(op)
			activities = append(activities, source{kind: "activity", activity: "operation-" + op.ID, data: data})
		}
		for i := len(refs) - 1; i >= 0; i-- {
			activities = append(activities, source{kind: "activity", activity: refs[i]})
		}
	}

	m.sources = append(m.sources, activities...)
	return m, nil
}

func (m *Reader) Read(ctx context.Context, ref string, offset, limit int) (Chunk, error) {
	data, canonical, err := m.record(ctx, ref)
	if err != nil {
		return Chunk{}, err
	}
	return TextChunk(canonical, string(data), offset, limit)
}

func (m *Reader) record(ctx context.Context, ref string) ([]byte, string, error) {
	if len(ref) > 512 {
		return nil, "", errors.New("invalid reference")
	}
	for _, s := range m.sources {
		readFrom := s
		match := false
		switch s.kind {
		case "workspace":
			match = ref == "workspace" || ref == "workspace:"+s.activity
		case "activity":
			match = ref == "activity:"+s.activity
			if ref == "workspace:"+s.activity && s.data == nil {
				match = true
				readFrom.kind = "workspace"
			}
		}
		if match {
			data, canonical, _, _, err := m.readSource(ctx, readFrom, 0)
			return data, canonical, err
		}
	}
	return nil, "", errors.New("workspace reference is outside captured scope")
}

func (m *Reader) readSource(ctx context.Context, s source, offset int64) ([]byte, string, int64, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", offset, false, err
	}
	switch s.kind {
	case "workspace":
		if offset != 0 {
			return nil, "", offset, false, errors.New("invalid workspace cursor")
		}
		data := s.data
		if data == nil {
			change, err := m.store.ReadActivity(m.workspaceID, s.activity)
			if err != nil {
				return nil, "", offset, false, err
			}
			data, _ = json.Marshal(safeWorkspace(change.After))
		}
		return data, "workspace:" + s.activity, 0, true, nil
	case "activity":
		if offset != 0 {
			return nil, "", offset, false, errors.New("invalid activity cursor")
		}
		if s.data != nil {
			return s.data, "activity:" + s.activity, 0, true, nil
		}
		change, err := m.store.ReadActivity(m.workspaceID, s.activity)
		if err != nil {
			return nil, "", offset, false, err
		}
		change.After = safeWorkspace(change.After)
		if change.Before != nil {
			before := safeWorkspace(*change.Before)
			change.Before = &before
		}
		data, err := json.Marshal(change)
		return data, "activity:" + s.activity, 0, true, err
	default:
		return nil, "", offset, false, errors.New("unsupported workspace record source")
	}
}

func safeWorkspace(w Workspace) Workspace {
	w.Connections = append([]Connection(nil), w.Connections...)
	for i, c := range w.Connections {
		settings := make(map[string]string)
		for _, key := range []string{"context", "namespace", "kubeconfig", "path", "workspace", "repo", "server", "revision"} {
			value, ok := c.Settings[key]
			if !ok {
				continue
			}
			if key == "repo" || key == "server" || key == "path" && strings.Contains(value, "://") {
				if u, err := url.Parse(value); err == nil && (u.Host != "" || u.Scheme == "file") {
					u.User, u.RawQuery, u.Fragment = nil, "", ""
					value = u.String()
				} else if strings.Contains(value, "://") || strings.HasPrefix(value, "//") {
					value = "[invalid repository or server reference]"
				} else if at := strings.LastIndexByte(value, '@'); at >= 0 && strings.Contains(value[at+1:], ":") {
					// Git also accepts user@host:path references without a URL scheme.
					value = value[at+1:]
				}
			}
			settings[key] = value
		}
		w.Connections[i].Settings = settings
	}
	return w
}

func TextChunk(ref, text string, offset, limit int) (Chunk, error) {
	if limit == 0 {
		limit = MaxReadBytes
	}
	if offset < 0 || offset > len(text) || limit < 1 || limit > MaxReadBytes || offset < len(text) && !utf8.RuneStart(text[offset]) {
		return Chunk{}, errors.New("invalid workspace chunk bounds")
	}
	end := min(offset+limit, len(text))
	for end > offset && end < len(text) && !utf8.RuneStart(text[end]) {
		end--
	}
	if end == offset && end < len(text) {
		return Chunk{}, errors.New("workspace byte limit is too small for the next character")
	}
	return Chunk{Ref: ref, Text: strings.Clone(text[offset:end]), NextOffset: end, TotalBytes: len(text), Truncated: end < len(text)}, nil
}
