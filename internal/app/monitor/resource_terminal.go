package monitor

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/vt"
	"github.com/simjay/plous/internal/ui/terminal"
	"io"
	"k8s.io/client-go/tools/remotecommand"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

type TerminalSizes struct {
	Ctx   context.Context
	Sizes chan remotecommand.TerminalSize
}

func (q *TerminalSizes) Next() *remotecommand.TerminalSize {
	select {
	case size := <-q.Sizes:
		return &size
	case <-q.Ctx.Done():
		return nil
	}
}

func (q *TerminalSizes) Resize(width, height int) {
	value := remotecommand.TerminalSize{Width: uint16(width), Height: uint16(height)}
	select {
	case q.Sizes <- value:
	default:
		select {
		case <-q.Sizes:
		default:
		}
		select {
		case q.Sizes <- value:
		default:
		}
	}
}

type TerminalInput = terminal.Input

type ResourceTerminal struct {
	Emulator                           *vt.SafeEmulator
	Input                              *TerminalInput
	Sizes                              *TerminalSizes
	Cancel                             context.CancelFunc
	Once                               sync.Once
	Context, Namespace, Pod, Container string
	Captured, Done                     bool
	Err                                string
	CursorVisible                      atomic.Bool
	Command                            string
	ScreenMu                           sync.Mutex
	OutputVersion                      atomic.Uint64
	Review                             *TerminalReview
	Launch                             *ResourceActionState
}

type ResourceTerminalResult struct {
	Terminal *ResourceTerminal
	Err      error
}

type ResourceTerminalTick struct{ Terminal *ResourceTerminal }

func (t *ResourceTerminal) Close() {
	t.Once.Do(func() {
		t.Cancel()
		// SafeEmulator.Close writes an unguarded flag. Closing its input pipe
		// unblocks reads and replies without racing an in-flight screen write.
		if pipe, ok := t.Emulator.InputPipe().(io.Closer); ok {
			_ = pipe.Close()
		}
	})
}

func TerminalTick(t *ResourceTerminal) tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return ResourceTerminalTick{t} })
}

// Parse arguments without a local shell. Metacharacters remain literal unless
// the user explicitly runs a shell with -c in the container.
func SplitExecCommand(text string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, r := range text {
		if escaped {
			if quote == '"' && !strings.ContainsRune("$`\"\\\n", r) {
				word.WriteRune('\\')
			}
			if r == '\n' {
				escaped = false
				continue
			}
			word.WriteRune(r)
			escaped = false
			started = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(r)
		started = true
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("Close the quote or complete the escaped character.")
	}
	if started {
		args = append(args, word.String())
	}
	if len(args) == 0 || args[0] == "" {
		return nil, fmt.Errorf("Enter a command to run in the container.")
	}
	return args, nil
}
