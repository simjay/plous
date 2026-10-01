// Package memory provides bounded reads of records in one captured workspace.
package memory

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/simjay/plous/internal/workspace"
)

const (
	MaxResults   = 20
	MaxReadBytes = 16 << 10
	ScanBytes    = 1 << 20
	ScanRecords  = 64
)

type Query struct {
	Source         string `json:"source,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	Text           string `json:"text,omitempty"`
	Cursor         string `json:"cursor,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

type Record struct {
	Ref            string `json:"ref"`
	ID             string `json:"id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	Source         string `json:"source"`
	Text           string `json:"text"`
	Truncated      bool   `json:"truncated,omitempty"`
}

type Page struct {
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
	kind         string
	conversation string
	boundary     int64
	activity     string
	data         []byte
}

type Memory struct {
	store       *workspace.Store
	workspaceID string
	sources     []source
}

// New captures complete record prefixes. Later appends are not historical input
// to this investigation, including its own execution trace.
func New(store *workspace.Store, workspaceID, conversationID string) (*Memory, error) {
	if store == nil {
		return nil, errors.New("memory requires a workspace store")
	}
	boundaries, err := store.RecordBoundaries(workspaceID, conversationID)
	if err != nil {
		return nil, err
	}
	m := &Memory{store: store, workspaceID: workspaceID}
	var activities []source
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
		for i := len(refs) - 1; i >= 0; i-- {
			activities = append(activities, source{kind: "activity", activity: refs[i]})
		}
	}
	for i := len(boundaries) - 1; i >= 0; i-- {
		b := boundaries[i]
		m.sources = append(m.sources,
			source{kind: "conversation", conversation: b.Conversation.ID, boundary: b.Messages},
			source{kind: "reasoning", conversation: b.Conversation.ID, boundary: b.Reasoning})
	}
	m.sources = append(m.sources, activities...)
	return m, nil
}

type cursor struct {
	Source int   `json:"s"`
	Offset int64 `json:"o"`
}

func (m *Memory) Search(ctx context.Context, q Query) (Page, error) {
	if q.Source != "" && q.Source != "conversation" && q.Source != "reasoning" && q.Source != "workspace" && q.Source != "activity" {
		return Page{}, errors.New("source must be conversation, reasoning, activity, or workspace")
	}
	if q.Limit == 0 {
		q.Limit = 10
	}
	if q.Limit < 1 || q.Limit > MaxResults || len(q.Text) > 4096 || len(q.Cursor) > 256 {
		return Page{}, errors.New("invalid memory search limits")
	}
	if q.ConversationID != "" {
		found := false
		for _, s := range m.sources {
			found = found || s.conversation == q.ConversationID
		}
		if !found {
			return Page{}, errors.New("conversation is outside captured scope")
		}
	}
	c := cursor{}
	if q.Cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(q.Cursor)
		if err != nil || json.Unmarshal(data, &c) != nil || c.Source < 0 || c.Source > len(m.sources) || c.Offset < 0 {
			return Page{}, errors.New("invalid memory cursor")
		}
	}
	page := Page{Records: []Record{}}
	pattern, err := regexp.Compile("(?i)" + regexp.QuoteMeta(q.Text))
	if err != nil {
		return Page{}, errors.New("memory search text must be valid UTF-8")
	}
	bytesScanned := 0
	for c.Source < len(m.sources) {
		if err := ctx.Err(); err != nil {
			return Page{}, err
		}
		s := m.sources[c.Source]
		if q.Source != "" && q.Source != s.kind || q.ConversationID != "" && q.ConversationID != s.conversation {
			c.Source++
			c.Offset = 0
			continue
		}
		data, ref, next, done, err := m.readSource(ctx, s, c.Offset)
		if err != nil {
			return Page{}, err
		}
		if done {
			c.Source++
			c.Offset = 0
		} else {
			c.Offset = next
		}
		if len(data) == 0 {
			continue
		}
		page.Scanned++
		bytesScanned += len(data)
		text := string(data)
		match := pattern.FindStringIndex(text)
		if match != nil {
			var identity struct {
				ID string `json:"id"`
			}
			json.Unmarshal(data, &identity)
			preview := excerpt(text, match[0], 1024)
			page.Records = append(page.Records, Record{Ref: ref, ID: identity.ID, ConversationID: s.conversation, Source: s.kind, Text: preview, Truncated: len(preview) < len(text)})
		}
		if len(page.Records) >= q.Limit || page.Scanned >= ScanRecords || bytesScanned >= ScanBytes {
			break
		}
	}
	if c.Source < len(m.sources) {
		data, _ := json.Marshal(c)
		page.NextCursor = base64.RawURLEncoding.EncodeToString(data)
		page.Truncated = true
	}
	return page, nil
}

func (m *Memory) Read(ctx context.Context, ref string, offset, limit int) (Chunk, error) {
	if limit == 0 {
		limit = MaxReadBytes
	}
	if len(ref) > 512 || offset < 0 || limit < 1 || limit > MaxReadBytes {
		return Chunk{}, errors.New("invalid memory read limits")
	}
	parts := strings.Split(ref, ":")
	for _, s := range m.sources {
		readFrom := s
		var recordOffset int64
		match := false
		switch s.kind {
		case "workspace":
			match = ref == "workspace" || ref == "workspace:"+s.activity
		case "activity":
			match = ref == "activity:"+s.activity
			if ref == "workspace:"+s.activity {
				match = true
				readFrom.kind = "workspace"
			}
		default:
			if len(parts) == 3 && parts[0] == s.kind && parts[1] == s.conversation {
				var err error
				recordOffset, err = strconv.ParseInt(parts[2], 10, 64)
				match = err == nil && recordOffset >= 0 && recordOffset < s.boundary
			}
		}
		if !match {
			continue
		}
		data, canonical, _, _, err := m.readSource(ctx, readFrom, recordOffset)
		if err != nil {
			return Chunk{}, err
		}
		if offset > len(data) || offset < len(data) && !utf8.RuneStart(data[offset]) {
			return Chunk{}, errors.New("memory offset must be a character boundary within the record")
		}
		end := min(offset+limit, len(data))
		for end > offset && end < len(data) && !utf8.RuneStart(data[end]) {
			end--
		}
		if end == offset && end < len(data) {
			return Chunk{}, errors.New("memory byte limit is too small for the next character")
		}
		return Chunk{Ref: canonical, Text: string(data[offset:end]), NextOffset: end, TotalBytes: len(data), Truncated: end < len(data)}, nil
	}
	return Chunk{}, errors.New("memory reference is outside captured scope")
}

func (m *Memory) readSource(ctx context.Context, s source, offset int64) ([]byte, string, int64, bool, error) {
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
		if offset > s.boundary {
			return nil, "", offset, false, errors.New("memory cursor exceeds captured boundary")
		}
		lines, next, err := m.store.ReadRecords(ctx, m.workspaceID, s.conversation, s.kind, offset, s.boundary, 1, ScanBytes)
		if err != nil || len(lines) == 0 {
			return nil, "", next, true, err
		}
		return lines[0].Data, fmt.Sprintf("%s:%s:%d", s.kind, s.conversation, lines[0].Offset), next, next >= s.boundary, nil
	}
}

func excerpt(text string, match, limit int) string {
	start := max(0, match-limit/4)
	for start > 0 && !utf8.RuneStart(text[start]) {
		start--
	}
	end := min(len(text), start+limit)
	for end > start && end < len(text) && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[start:end]
}

func safeWorkspace(w workspace.Workspace) workspace.Workspace {
	w.Connections = append([]workspace.Connection(nil), w.Connections...)
	for i, c := range w.Connections {
		settings := make(map[string]string)
		for _, key := range []string{"context", "namespace", "kubeconfig", "path", "workspace", "repo", "server", "revision"} {
			value, ok := c.Settings[key]
			if !ok {
				continue
			}
			if key == "repo" || key == "server" {
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
