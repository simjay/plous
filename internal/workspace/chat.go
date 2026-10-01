package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/simjay/plous/internal/ai"
)

type Conversation struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Title       string    `json:"title"`
	Created     time.Time `json:"created"`
}

type Request struct {
	ID               string    `json:"id"`
	Time             time.Time `json:"time"`
	SettingsRevision uint64    `json:"settings_revision"`
	Agent            ai.Config `json:"agent"`
	Role             string    `json:"role,omitempty"`
	Prompt           string    `json:"prompt"`
	Context          string    `json:"context"`
}

type Response struct {
	Status  string    `json:"status"`
	Text    string    `json:"text"`
	Error   string    `json:"error,omitempty"`
	Usage   ai.Usage  `json:"usage"`
	Finish  string    `json:"finish,omitempty"`
	Updated time.Time `json:"updated"`
}

type Turn struct {
	Request  Request  `json:"request"`
	Response Response `json:"response"`
}

// ConversationRecord is one immutable message version. Later assistant versions
// retain partial responses while a final status closes the turn.
type ConversationRecord struct {
	ID     string    `json:"id"`
	TurnID string    `json:"turn_id"`
	Role   string    `json:"role"`
	Time   time.Time `json:"time"`
	Delta  bool      `json:"delta,omitempty"`
	Response
}

// ReasoningEntry records observable execution, never private model thoughts.
type ReasoningEntry struct {
	ID            string          `json:"id"`
	MessageID     string          `json:"message_id,omitempty"`
	ParentID      string          `json:"parent_id,omitempty"`
	ParentSource  string          `json:"parent_source,omitempty"`
	Kind          string          `json:"kind"`
	Tool          string          `json:"tool,omitempty"`
	Status        string          `json:"status"`
	Input         json.RawMessage `json:"input,omitempty"`
	Content       string          `json:"content,omitempty"`
	Source        string          `json:"source,omitempty"`
	Model         ai.Config       `json:"model"`
	Time          time.Time       `json:"time"`
	CoveredTurnID string          `json:"covered_turn_id,omitempty"`
	Request       *Request        `json:"request,omitempty"`
}

func chronologicalID() string {
	return strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000Z"), ".", "") + "-" + newID()
}

func (s *Store) chatDir(workspaceID string) (string, error) {
	base := s.root
	if workspaceID != "" {
		if !idPattern.MatchString(workspaceID) {
			return "", errors.New("invalid workspace ID")
		}
		base = filepath.Join(s.root, "workspaces", workspaceID)
		info, err := os.Lstat(base)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", errors.New("workspace must be a directory")
		}
	}
	dir := filepath.Join(base, "conversations")
	return dir, privateDir(dir)
}

func (s *Store) Conversations(workspaceID string) ([]Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, ErrClosed
	}
	return s.conversations(workspaceID)
}

func (s *Store) conversations(workspaceID string) ([]Conversation, error) {
	dir, err := s.chatDir(workspaceID)
	if err != nil {
		return nil, err
	}
	entries, err := readDirBounded(dir, 10000)
	if err != nil {
		return nil, err
	}
	var result []Conversation
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".pending-") {
			continue
		}
		if !e.IsDir() || !idPattern.MatchString(e.Name()) {
			return nil, errors.New("unexpected conversation entry")
		}
		var c Conversation
		if err := readJSON(filepath.Join(dir, e.Name(), "metadata.json"), &c); err != nil {
			return nil, err
		}
		if c.ID != e.Name() || c.WorkspaceID != workspaceID || c.Created.IsZero() {
			return nil, errors.New("corrupt conversation identity")
		}
		result = append(result, c)
	}
	return result, nil
}

func (s *Store) NewConversation(workspaceID, title string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return Conversation{}, ErrClosed
	}
	dir, err := s.chatDir(workspaceID)
	if err != nil {
		return Conversation{}, err
	}
	c := Conversation{ID: chronologicalID(), WorkspaceID: workspaceID, Title: title, Created: time.Now().UTC()}
	stage, err := os.MkdirTemp(dir, ".pending-")
	if err != nil {
		return c, err
	}
	defer os.RemoveAll(stage)
	data, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	for name, value := range map[string][]byte{"metadata.json": data, "conversation.jsonl": {}, "reasoning.jsonl": {}} {
		if err := writeFile(filepath.Join(stage, name), value); err != nil {
			return c, err
		}
	}
	if err := syncDir(stage); err != nil {
		return c, err
	}
	if err := os.Rename(stage, filepath.Join(dir, c.ID)); err != nil {
		return c, err
	}
	return c, syncDir(dir)
}

func (s *Store) conversationDir(w, id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", errors.New("invalid conversation ID")
	}
	dir, err := s.chatDir(w)
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, id)
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("conversation must be a directory")
	}
	var c Conversation
	if err := readJSON(filepath.Join(dir, "metadata.json"), &c); err != nil {
		return "", err
	}
	if c.ID != id || c.WorkspaceID != w {
		return "", errors.New("corrupt conversation identity")
	}
	return dir, nil
}

func (s *Store) Turns(w, id string) ([]Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, ErrClosed
	}
	dir, err := s.conversationDir(w, id)
	if err != nil {
		return nil, err
	}
	return readTurns(dir)
}

func readTurns(dir string) ([]Turn, error) {
	requests := make(map[string]Request)
	if err := scanJSONL(filepath.Join(dir, "reasoning.jsonl"), func(data []byte) error {
		var entry ReasoningEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return err
		}
		if entry.Kind == "request" && entry.Request != nil {
			if entry.MessageID != entry.Request.ID || !idPattern.MatchString(entry.MessageID) {
				return errors.New("corrupt request identity")
			}
			if _, exists := requests[entry.MessageID]; exists {
				return errors.New("duplicate request identity")
			}
			requests[entry.MessageID] = *entry.Request
		}
		return nil
	}); err != nil {
		return nil, err
	}
	var turns []Turn
	positions := make(map[string]int)
	err := scanJSONL(filepath.Join(dir, "conversation.jsonl"), func(data []byte) error {
		var record ConversationRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return err
		}
		if !idPattern.MatchString(record.ID) || !idPattern.MatchString(record.TurnID) || record.Time.IsZero() {
			return errors.New("corrupt conversation record")
		}
		switch record.Role {
		case "user":
			request, ok := requests[record.TurnID]
			if !ok || request.Prompt != record.Text {
				return errors.New("conversation request is missing or mismatched")
			}
			if _, exists := positions[record.TurnID]; exists {
				return errors.New("duplicate conversation turn")
			}
			positions[record.TurnID] = len(turns)
			turns = append(turns, Turn{Request: request, Response: Response{Status: "streaming", Updated: record.Time}})
		case "assistant":
			i, ok := positions[record.TurnID]
			if !ok || turns[i].Response.Status != "streaming" || !validResponseStatus(record.Status) {
				return errors.New("corrupt response sequence")
			}
			if record.Delta {
				if record.Status != "streaming" {
					return errors.New("only streaming responses can append text")
				}
				record.Response.Text = turns[i].Response.Text + record.Response.Text
			}
			turns[i].Response = record.Response
		default:
			return errors.New("invalid conversation role")
		}
		return nil
	})
	return turns, err
}

func (s *Store) StartTurn(w, id string, r Request) (Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return Turn{}, ErrClosed
	}
	dir, err := s.conversationDir(w, id)
	if err != nil {
		return Turn{}, err
	}
	r.ID, r.Time = chronologicalID(), time.Now().UTC()
	t := Turn{Request: r, Response: Response{Status: "streaming", Updated: r.Time}}
	entry := ReasoningEntry{ID: chronologicalID(), MessageID: r.ID, Kind: "request", Status: "completed", Model: r.Agent, Time: r.Time, Request: &r}
	if err := appendJSONL(filepath.Join(dir, "reasoning.jsonl"), entry); err != nil {
		return Turn{}, err
	}
	message := ConversationRecord{ID: r.ID, TurnID: r.ID, Role: "user", Time: r.Time, Response: Response{Status: "completed", Text: r.Prompt, Updated: r.Time}}
	return t, appendJSONL(filepath.Join(dir, "conversation.jsonl"), message)
}

func (s *Store) SaveResponse(w, id, turn string, r Response) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return ErrClosed
	}
	if !idPattern.MatchString(turn) || !validResponseStatus(r.Status) {
		return errors.New("invalid response identity or status")
	}
	dir, err := s.conversationDir(w, id)
	if err != nil {
		return err
	}
	turns, err := readTurns(dir)
	if err != nil {
		return err
	}
	for _, t := range turns {
		if t.Request.ID != turn {
			continue
		}
		comparison := r
		comparison.Updated = t.Response.Updated
		if reflect.DeepEqual(t.Response, comparison) {
			f, err := openRecordFile(filepath.Join(dir, "conversation.jsonl"), true)
			if err != nil {
				return err
			}
			defer f.Close()
			return f.Sync()
		}
		if t.Response.Status != "streaming" {
			return errors.New("a finished response cannot be overwritten")
		}
		r.Updated = time.Now().UTC()
		record := ConversationRecord{ID: chronologicalID(), TurnID: turn, Role: "assistant", Time: r.Updated, Response: r}
		if r.Status == "streaming" && strings.HasPrefix(r.Text, t.Response.Text) {
			record.Delta = true
			record.Text = strings.TrimPrefix(r.Text, t.Response.Text)
		}
		return appendJSONL(filepath.Join(dir, "conversation.jsonl"), record)
	}
	return errors.New("conversation turn does not exist")
}

func validResponseStatus(status string) bool {
	switch status {
	case "streaming", "completed", "cancelled", "error", "interrupted":
		return true
	}
	return false
}

func (s *Store) AppendReasoning(w, id string, entry ReasoningEntry) (ReasoningEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return entry, ErrClosed
	}
	if strings.TrimSpace(entry.Kind) == "" || strings.TrimSpace(entry.Status) == "" || entry.Kind == "request" || entry.Request != nil {
		return entry, errors.New("invalid execution entry")
	}
	for _, id := range []string{entry.MessageID, entry.ParentID, entry.CoveredTurnID} {
		if id != "" && !idPattern.MatchString(id) {
			return entry, errors.New("invalid execution reference")
		}
	}
	dir, err := s.conversationDir(w, id)
	if err != nil {
		return entry, err
	}
	if entry.Kind == "compaction" {
		if strings.TrimSpace(entry.Content) == "" || entry.Status != "completed" || entry.CoveredTurnID == "" {
			return entry, errors.New("compaction requires a summary and covered turn")
		}
		turns, err := readTurns(dir)
		if err != nil {
			return entry, err
		}
		found := false
		for _, turn := range turns {
			found = found || turn.Request.ID == entry.CoveredTurnID && turn.Response.Status == "completed"
		}
		if !found {
			return entry, errors.New("compaction must cover a completed conversation turn")
		}
	}
	entry.ID, entry.Time = chronologicalID(), time.Now().UTC()
	return entry, appendJSONL(filepath.Join(dir, "reasoning.jsonl"), entry)
}

func (s *Store) LatestCompaction(w, id string) (*ReasoningEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, ErrClosed
	}
	dir, err := s.conversationDir(w, id)
	if err != nil {
		return nil, err
	}
	completed := make(map[string]bool)
	if err := scanJSONL(filepath.Join(dir, "conversation.jsonl"), func(data []byte) error {
		var record ConversationRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return err
		}
		if record.Role == "assistant" && record.Status == "completed" && !record.Delta && idPattern.MatchString(record.TurnID) {
			completed[record.TurnID] = true
		}
		return nil
	}); err != nil {
		return nil, err
	}
	var latest *ReasoningEntry
	err = scanJSONL(filepath.Join(dir, "reasoning.jsonl"), func(data []byte) error {
		var entry ReasoningEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return err
		}
		if entry.Kind == "compaction" && entry.Status == "completed" && completed[entry.CoveredTurnID] && strings.TrimSpace(entry.Content) != "" && (latest == nil || entry.CoveredTurnID >= latest.CoveredTurnID) {
			latest = &entry
		}
		return nil
	})
	return latest, err
}

func scanJSONL(path string, visit func([]byte) error) error {
	f, err := openRecordFile(path, false)
	if err != nil {
		return err
	}
	defer f.Close()
	boundary, err := completedBoundary(f)
	if err != nil {
		return err
	}
	var offset int64
	for offset < boundary {
		lines, next, err := readRecordLines(context.Background(), f, offset, boundary, 64, 2*MaxRecordBytes)
		if err != nil {
			return fmt.Errorf("corrupt %s; preserved for recovery: %w", path, err)
		}
		for _, line := range lines {
			if err := visit(line.Data); err != nil {
				return fmt.Errorf("corrupt %s; preserved for recovery: %w", path, err)
			}
		}
		offset = next
	}
	return nil
}
