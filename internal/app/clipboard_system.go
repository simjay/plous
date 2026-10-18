package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const clipboardLimit = 2 * 1024 * 1024

func systemClipboard() clipboardAccess {
	return clipboardFor(runtime.GOOS, os.Getenv, exec.LookPath)
}

func clipboardFor(platform string, getenv func(string) string, lookPath func(string) (string, error)) clipboardAccess {
	// Remote sessions must address the user's terminal, not the server's desktop.
	if getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "" || getenv("MOSH_IP") != "" {
		return clipboardAccess{}
	}
	var writer, reader []string
	find := func(name string, args ...string) []string {
		path, err := lookPath(name)
		if err != nil {
			return nil
		}
		return append([]string{path}, args...)
	}
	switch platform {
	case "darwin":
		writer, reader = find("pbcopy"), find("pbpaste", "-Prefer", "txt")
	case "linux":
		if getenv("WAYLAND_DISPLAY") != "" {
			writer, reader = find("wl-copy"), find("wl-paste", "--no-newline")
		}
		if getenv("DISPLAY") != "" {
			if writer == nil {
				writer = find("xclip", "-selection", "clipboard", "-in")
			}
			if reader == nil {
				reader = find("xclip", "-selection", "clipboard", "-out")
			}
			if writer == nil {
				writer = find("xsel", "--clipboard", "--input")
			}
			if reader == nil {
				reader = find("xsel", "--clipboard", "--output")
			}
		}
	}
	var access clipboardAccess
	if len(writer) > 0 {
		access.write = func(ctx context.Context, text string) error {
			cmd := exec.CommandContext(ctx, writer[0], writer[1:]...)
			cmd.WaitDelay = 100 * time.Millisecond
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
	}
	if len(reader) > 0 {
		access.read = func(ctx context.Context) (string, error) {
			cmd := exec.CommandContext(ctx, reader[0], reader[1:]...)
			cmd.WaitDelay = 100 * time.Millisecond
			var buffer clipboardBuffer
			cmd.Stdout = &buffer
			if err := cmd.Run(); err != nil {
				return "", err
			}
			return buffer.data.String(), nil
		}
	}
	return access
}

type clipboardBuffer struct{ data bytes.Buffer }

func (b *clipboardBuffer) Write(p []byte) (int, error) {
	if len(p) > clipboardLimit-b.data.Len() {
		return 0, errors.New("clipboard exceeds document limit")
	}
	return b.data.Write(p)
}
