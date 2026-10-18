package app

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"runtime/debug"
)

type terminalOutput struct {
	renderer, diagnostics *os.File
}

// Reserve terminal output for Bubble Tea before starting background work.
// Native dependencies can write to fd 1/2 directly, bypassing configured loggers.
func reserveTerminalOutput() (*terminalOutput, error) {
	duplicate := func(file *os.File) (*os.File, error) {
		fd, err := unix.FcntlInt(file.Fd(), unix.F_DUPFD_CLOEXEC, 3)
		if err != nil {
			return nil, err
		}
		return os.NewFile(uintptr(fd), file.Name()), nil
	}
	out, err := duplicate(os.Stdout)
	if err != nil {
		return nil, err
	}
	errout, err := duplicate(os.Stderr)
	if err != nil {
		out.Close()
		return nil, err
	}
	terminal := &terminalOutput{renderer: out, diagnostics: errout}
	sink, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		out.Close()
		errout.Close()
		return nil, err
	}
	defer sink.Close()
	if err := debug.SetCrashOutput(errout, debug.CrashOptions{}); err != nil {
		terminal.Close()
		return nil, err
	}
	if err := unix.Dup2(int(sink.Fd()), int(os.Stdout.Fd())); err != nil {
		terminal.Close()
		return nil, err
	}
	if err := unix.Dup2(int(sink.Fd()), int(os.Stderr.Fd())); err != nil {
		terminal.Close()
		return nil, err
	}
	return terminal, nil
}

// Close runs after the renderer exits, so shutdown errors and panic reports
// remain visible on the original terminal without racing Go stream variables.
func (t *terminalOutput) Close() error {
	return errors.Join(
		unix.Dup2(int(t.renderer.Fd()), int(os.Stdout.Fd())),
		unix.Dup2(int(t.diagnostics.Fd()), int(os.Stderr.Fd())),
		debug.SetCrashOutput(nil, debug.CrashOptions{}),
		t.renderer.Close(), t.diagnostics.Close(),
	)
}
