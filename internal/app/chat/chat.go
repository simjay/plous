package chat

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	agent "github.com/simjay/plous/internal/ai"
	backend "github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/history"
	aimodel "github.com/simjay/plous/internal/ai/model"
)

type ChatState struct {
	Conversation   history.Conversation
	Messages       []aimodel.RequestMessage
	Progress       string
	Turns          []history.Turn
	ActiveTurn     *history.Turn
	Older, Newer   bool
	Oversized      bool
	Evicted        bool
	ResumeSequence uint64
	Restore        *ChatRestore
	SaveVersion    uint64
	Dirty          bool
	SaveFailed     bool
	History        bool
	Models         bool
	ModelCursor    int
	Choices        []history.Conversation
	Cursor, Scroll int
	Reading        bool
	ViewRows       int
	ViewHeight     int
	ViewWidth      int
	Anchor         ChatRow
	Reply          int
	ReplySelected  bool
	ReplyJump      bool
	ReplyLines     []int
	Rendered       []ChatRenderedReply
	Questions      []ChatQuestion
	ContentWidth   int
	Evidence       *ChatEvidence
	Err            string
}

type ChatRun struct {
	Agent                     backend.Kind
	ContextUsed, ContextLimit int
	Chat                      *ChatState
	Cancel                    context.CancelFunc
	Events                    chan ChatEvent
	Terminal                  ChatEvent
	Stopping                  bool
	Starting, Finished        bool
	Response                  history.Response
}

type ChatEvent struct {
	ContextUsed, ContextLimit int
	Run                       *ChatRun
	Text                      string
	Status                    string
	Progress                  bool
	Done                      bool
	HasText                   bool
	Result                    aimodel.Result
	Err                       error
}

func (run *ChatRun) Produce(ctx context.Context, options agent.Options) {
	defer run.Cancel()
	defer close(run.Events)
	latestText, emitted := "", false
	options.ContextUsage = func(used, limit int) {
		select {
		case run.Events <- ChatEvent{Run: run, ContextUsed: used, ContextLimit: limit}:
		case <-ctx.Done():
		}
	}
	options.Progress = func(status string) {
		select {
		case run.Events <- ChatEvent{Run: run, Progress: true, Status: status}:
		case <-ctx.Done():
		}
	}
	options.Emit = func(text string) error {
		latestText, emitted = text, true
		select {
		case run.Events <- ChatEvent{Run: run, Text: text}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	result, err := agent.Run(ctx, options)
	run.Terminal = ChatEvent{Run: run, Done: true, Result: result, Err: err, Text: latestText, HasText: emitted}
}

func WaitChat(run *ChatRun) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-run.Events
		if !ok {
			if run.Terminal.Done {
				return run.Terminal
			}
			return ChatEvent{Run: run, Done: true, Err: fmt.Errorf("AI stream ended unexpectedly")}
		}
		return event
	}
}
