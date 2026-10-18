package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/simjay/plous/internal/ai/agent"
)

// TerminalOptions captures one native session and its immutable tool authority.
type TerminalOptions struct {
	Kind                              agent.Kind
	Path, Directory, Instructions, ID string
	StateDirectory                    string
	Resume                            bool
	Tools                             agent.Tools
	Width, Height                     int
	Input                             io.Reader
	Output                            io.Writer
	sessionID                         string
}

type Terminal struct {
	mu            sync.Mutex
	file          *os.File
	changed       chan struct{}
	width, height int
	ctx           context.Context
	cancel        context.CancelCauseFunc
	done          chan struct{}
	err           error
	once          sync.Once
}

// StartTerminal launches the installed interactive UI. It never parses replies.
func StartTerminal(ctx context.Context, o TerminalOptions) (*Terminal, error) {
	if o.Tools == nil || o.Input == nil || o.Output == nil {
		return nil, errors.New("native agent requires a terminal and Plous tools")
	}
	if o.StateDirectory == "" {
		return nil, errors.New("native agent requires a session storage directory")
	}
	if o.Kind == agent.ClaudeCode {
		var err error
		o.sessionID, err = claudeSessionID(o.StateDirectory, o.Resume)
		if err != nil {
			return nil, err
		}
	}
	path := o.Path
	if path == "" {
		var err error
		path, err = Executable(o.Kind)
		if err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithCancelCause(ctx)
	b, err := OpenBridge(ctx, o.Tools, o.ID)
	if err != nil {
		cancel(err)
		return nil, err
	}
	configDir, err := os.MkdirTemp("", "plous-native-")
	if err != nil {
		b.Close()
		cancel(err)
		return nil, err
	}
	cleanup := func() { b.Close(); _ = os.RemoveAll(configDir) }
	launch, err := prepareTerminal(ctx, path, configDir, o, b)
	if err != nil {
		cleanup()
		cancel(err)
		return nil, err
	}
	t := &Terminal{ctx: ctx, cancel: cancel, done: make(chan struct{}), changed: make(chan struct{}), width: o.Width, height: o.Height}
	start := func() (*exec.Cmd, *os.File, error) {
		args, err := terminalResumeArgs(o, launch.args)
		if err != nil {
			return nil, nil, err
		}
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Dir = o.Directory
		cmd.Env = launch.env
		cmd.WaitDelay = 2 * time.Second
		configureTerminalProcess(cmd)
		t.mu.Lock()
		defer t.mu.Unlock()
		file, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(max(1, t.width)), Rows: uint16(max(1, t.height))})
		if err != nil {
			return nil, nil, errors.New("could not start installed agent terminal: " + err.Error())
		}
		t.file = file
		close(t.changed)
		t.changed = make(chan struct{})
		return cmd, file, nil
	}
	cmd, file, err := start()
	if err != nil {
		cleanup()
		cancel(err)
		return nil, err
	}
	go func() { _, _ = io.Copy(terminalInput{t}, o.Input) }()
	go func() {
		for {
			diagnostic := resumeDiagnostic{enabled: o.Kind == agent.ClaudeCode && o.Resume, expected: noConversation + o.sessionID}
			outputDone := make(chan struct{})
			go func() { _, _ = io.Copy(o.Output, io.TeeReader(file, &diagnostic)); close(outputDone) }()
			err = cmd.Wait()
			killTerminalGroup(cmd)
			// Bound the drain in case a descendant keeps the terminal open.
			select {
			case <-outputDone:
			case <-time.After(time.Second):
			}
			_ = file.Close()
			<-outputDone
			if !diagnostic.missingConversation(err) || ctx.Err() != nil || b.ctx.Err() != nil {
				break
			}
			// Keep the same scope, bridge and input reader for one fresh launch.
			o.Resume = false
			_, _ = o.Output.Write([]byte("\x1b[2J\x1b[H"))
			cmd, file, err = start()
			if err != nil {
				break
			}
		}
		if cause := context.Cause(ctx); cause != nil {
			err = cause
		}
		if cause := context.Cause(b.ctx); cause != nil && !errors.Is(cause, context.Canceled) {
			err = cause
		}
		t.err = err
		cancel(context.Canceled)
		cleanup()
		close(t.done)
	}()
	return t, nil
}

func (t *Terminal) Resize(width, height int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.width, t.height = width, height
	return pty.Setsize(t.file, &pty.Winsize{Cols: uint16(max(1, width)), Rows: uint16(max(1, height))})
}
func (t *Terminal) Close()      { t.once.Do(func() { t.cancel(context.Canceled) }) }
func (t *Terminal) Wait() error { <-t.done; return t.err }

type terminalInput struct{ *Terminal }

func (w terminalInput) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		if err := w.ctx.Err(); err != nil {
			return written, context.Cause(w.ctx)
		}
		w.mu.Lock()
		file, changed := w.file, w.changed
		w.mu.Unlock()
		n, err := file.Write(p)
		written += n
		p = p[n:]
		if err != nil {
			select {
			case <-changed:
			case <-w.ctx.Done():
				return written, context.Cause(w.ctx)
			}
		}
	}
	return written, nil
}

// Retain only a bounded startup diagnostic, never a native conversation.
type resumeDiagnostic struct {
	enabled  bool
	data     []byte
	expected string
}

const noConversation = "No conversation found with session ID: "

var sessionUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Match the CLI's startup color and reset controls, including a split escape.
var resumeControls = regexp.MustCompile("\x1b(?:\\][^\x07\x1b]*(?:\x07|\x1b(?:\\\\|$)|$)|\\[[0-?]*[ -/]*(?:[@-~]|$)|[ -/]*(?:[0-~]|$))")

func (d *resumeDiagnostic) text() string {
	return compactDiagnostic(resumeControls.ReplaceAllString(string(d.data), ""))
}

func compactDiagnostic(text string) string {
	// Narrow terminals replace spaces with cursor moves and wrap the diagnostic.
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(" \t\r\n\x0e\x0f", r) {
			return -1
		}
		return r
	}, text)
}

func (d *resumeDiagnostic) Write(p []byte) (int, error) {
	if d.enabled {
		if len(d.data)+len(p) > 4096 {
			d.enabled, d.data = false, nil
		} else {
			d.data = append(d.data, p...)
			if !strings.HasPrefix(compactDiagnostic(d.expected), d.text()) {
				d.enabled, d.data = false, nil
			}
		}
	}
	return len(p), nil
}

func (d *resumeDiagnostic) missingConversation(err error) bool {
	var exit *exec.ExitError
	return d.enabled && errors.As(err, &exit) && exit.ExitCode() == 1 &&
		d.text() == compactDiagnostic(d.expected)
}
