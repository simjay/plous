package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClipboardBackendSelection(t *testing.T) {
	for _, tc := range []struct {
		name, platform string
		env            map[string]string
		available      []string
		lookups        []string
		read, write    bool
	}{
		{"macOS", "darwin", nil, []string{"pbcopy", "pbpaste"}, []string{"pbcopy", "pbpaste"}, true, true},
		{"Wayland", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, []string{"wl-copy", "wl-paste"}, []string{"wl-copy", "wl-paste"}, true, true},
		{"X11", "linux", map[string]string{"DISPLAY": ":0"}, []string{"xclip"}, []string{"xclip", "xclip"}, true, true},
		{"X11 xsel", "linux", map[string]string{"DISPLAY": ":0"}, []string{"xsel"}, []string{"xclip", "xclip", "xsel", "xsel"}, true, true},
		{"SSH", "darwin", map[string]string{"SSH_CONNECTION": "fixture"}, nil, nil, false, false},
		{"SSH tty", "linux", map[string]string{"SSH_TTY": "/dev/pts/1", "DISPLAY": ":0"}, nil, nil, false, false},
		{"Mosh", "linux", map[string]string{"MOSH_IP": "fixture", "WAYLAND_DISPLAY": "wayland-0"}, nil, nil, false, false},
		{"headless", "linux", nil, nil, nil, false, false},
		{"missing helpers", "linux", map[string]string{"DISPLAY": ":0"}, nil, []string{"xclip", "xclip", "xsel", "xsel"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var looked []string
			a := clipboardFor(tc.platform, func(key string) string { return tc.env[key] }, func(name string) (string, error) {
				looked = append(looked, name)
				for _, available := range tc.available {
					if name == available {
						return "/fixture/" + name, nil
					}
				}
				return "", errors.New("not installed")
			})
			if (a.read != nil) != tc.read || (a.write != nil) != tc.write || !reflect.DeepEqual(looked, tc.lookups) {
				t.Fatalf("read=%v write=%v lookups=%v", a.read != nil, a.write != nil, looked)
			}
		})
	}
}

func TestClipboardHelpersPreserveUnicodeAndBoundReads(t *testing.T) {
	dir := t.TempDir()
	storage := filepath.Join(dir, "clipboard")
	t.Setenv("PLOUS_TEST_CLIPBOARD", storage)
	for name, script := range map[string]string{
		"pbcopy":  "#!/bin/sh\ncat > \"$PLOUS_TEST_CLIPBOARD\"\n",
		"pbpaste": "#!/bin/sh\ncat \"$PLOUS_TEST_CLIPBOARD\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	a := clipboardFor("darwin", func(string) string { return "" }, func(name string) (string, error) { return filepath.Join(dir, name), nil })
	text := "literal $(touch forbidden) `command`\n界 cafe\u0301\n"
	if err := a.write(t.Context(), text); err != nil {
		t.Fatal(err)
	}
	if got, err := a.read(t.Context()); err != nil || got != text {
		t.Fatalf("clipboard = %q, %v", got, err)
	}
	if err := os.WriteFile(storage, []byte(strings.Repeat("x", clipboardLimit+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := a.read(t.Context()); err == nil || got != "" {
		t.Fatal("oversized clipboard was silently truncated")
	}
	if err := os.WriteFile(filepath.Join(dir, "pbpaste"), []byte("#!/bin/sh\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if got, err := a.read(ctx); err == nil || got != "" || time.Since(start) > time.Second {
		t.Fatal("clipboard helper ignored cancellation")
	}
}
