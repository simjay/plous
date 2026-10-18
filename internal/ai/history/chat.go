package history

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/simjay/plous/internal/ai/agent"
	ai "github.com/simjay/plous/internal/ai/model"
	"github.com/simjay/plous/internal/integrations/observation"
)

type Conversation struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Title       string    `json:"title"`
	Created     time.Time `json:"created"`
}

type Request struct {
	Backend          agent.Kind `json:"backend,omitempty"`
	ID               string     `json:"id"`
	Time             time.Time  `json:"time"`
	SettingsRevision uint64     `json:"settings_revision"`
	Agent            ai.Config  `json:"agent"`
	Role             string     `json:"role,omitempty"`
	Prompt           string     `json:"prompt"`
	Context          string     `json:"context"`
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
	Sequence    uint64   `json:"sequence,string"`
	EndSequence uint64   `json:"end_sequence,string,omitempty"`
	UserRef     string   `json:"user_ref,omitempty"`
	AnswerRef   string   `json:"answer_ref,omitempty"`
	Request     Request  `json:"request"`
	Response    Response `json:"response"`
}

// ConversationRecord is one immutable message version. Later assistant versions
// retain partial responses while a final status closes the turn.
type ConversationRecord struct {
	Kind       string         `json:"kind,omitempty"`
	AnswerID   string         `json:"answer_id,omitempty"`
	Sequence   uint64         `json:"sequence,string,omitempty"`
	Boundaries historyOffsets `json:"boundaries,omitempty"`
	ID         string         `json:"id"`
	TurnID     string         `json:"turn_id"`
	Role       string         `json:"role"`
	Time       time.Time      `json:"time"`
	Delta      bool           `json:"delta,omitempty"`
	Response
}

// ReasoningEntry records observable execution, never private model thoughts.
type ReasoningEntry struct {
	Projection *observation.Result `json:"projection,omitempty"`
	Messages   []ai.RequestMessage `json:"messages,omitempty"`
	Sequence   uint64              `json:"sequence,string,omitempty"`
	TurnID     string              `json:"turn_id,omitempty"`
	Ref        string              `json:"-"`
	ID         string              `json:"id"`
	MessageID  string              `json:"message_id,omitempty"`
	Kind       string              `json:"kind"`
	Tool       string              `json:"tool,omitempty"`
	Status     string              `json:"status"`
	Input      json.RawMessage     `json:"input,omitempty"`
	Content    string              `json:"content,omitempty"`
	Model      ai.Config           `json:"model"`
	Time       time.Time           `json:"time"`
	Request    *Request            `json:"request,omitempty"`
}

func chronologicalID() string {
	return strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000Z"), ".", "") + "-" + newID()
}

func (s *Store) chatDir(workspaceID string) (string, error) {
	base := s.root
	if s.scopeDir != nil {
		var err error
		base, err = s.scopeDir(workspaceID)
		if err != nil {
			return "", err
		}
	} else if workspaceID != "" {
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
	if s.closed {
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
	if s.closing {
		return Conversation{}, ErrClosed
	}
	if s.closed {
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
	for name, value := range map[string][]byte{"metadata.json": data} {
		if err := writeFile(filepath.Join(stage, name), value); err != nil {
			return c, err
		}
	}
	if err := createHistoryFiles(stage, c.ID); err != nil {
		return c, err
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

func validResponseStatus(status string) bool {
	switch status {
	case "streaming", "completed", "cancelled", "error", "interrupted":
		return true
	}
	return false
}
