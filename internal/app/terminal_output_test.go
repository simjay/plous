package app

import (
	"bytes"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestTerminalOutputBelongsToRenderer(t *testing.T) {
	if mode := os.Getenv("PLOUS_TEST_OUTPUT_CHILD"); mode != "" {
		fmt.Fprintln(os.Stdout, "before")
		output, err := reserveTerminalOutput()
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(os.Stdout, "unowned stdout")
		fmt.Fprintln(os.Stderr, "unowned stderr")
		log.Print("unowned logger")
		if mode == "panic" {
			go func() { panic("background crash remains visible") }()
			select {}
		}
		fmt.Fprintln(output.renderer, "frame")
		fmt.Fprintln(output.diagnostics, "explicit error")
		if err := output.Close(); err != nil {
			panic(err)
		}
		fmt.Fprintln(os.Stdout, "after")
		fmt.Fprintln(os.Stderr, "restored stderr")
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestTerminalOutputBelongsToRenderer$")
	cmd.Env = append(os.Environ(), "PLOUS_TEST_OUTPUT_CHILD=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("output boundary failed: %v: %s", err, stderr.String())
	}
	if stdout.String() != "before\nframe\nafter\n" || stderr.String() != "explicit error\nrestored stderr\n" {
		t.Fatalf("terminal output escaped its owner: stdout %q, stderr %q", stdout.String(), stderr.String())
	}
	cmd = exec.Command(executable, "-test.run=^TestTerminalOutputBelongsToRenderer$")
	cmd.Env = append(os.Environ(), "PLOUS_TEST_OUTPUT_CHILD=panic")
	stdout.Reset()
	stderr.Reset()
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err == nil || !strings.Contains(stderr.String(), "panic: background crash remains visible") {
		t.Fatalf("output boundary hid a process failure: %v, %q", err, stderr.String())
	}
}

func TestCodeStartupFramesStayInsideTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m, _ := codeModel(t)
		m.width, m.height = size[0], size[1]
		m.selectScope(m.active, "manifests", m.connection)
		s := m.coder()
		inventory := s.Inventory
		for _, phase := range []string{"loading", "rendering", "ready", "failed"} {
			s.Inventory = inventory
			s.Busy, s.Running = false, ""
			s.Err = ""
			switch phase {
			case "loading":
				s.Inventory, s.Busy, s.Running = nil, true, "load"
			case "rendering":
				s.Busy, s.Running, s.RunningEntry = true, "preview", s.Preference.Selected
			case "failed":
				s.Err = strings.Repeat("source diagnostic ", 100)
			}
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) != m.height {
				t.Fatalf("%s at %v: %d rows, want %d", phase, size, len(lines), m.height)
			}
			for row, line := range lines {
				if width := ansi.StringWidth(line); width != m.width {
					t.Fatalf("%s at %v: row %d has width %d, want %d", phase, size, row, width, m.width)
				}
			}
		}
	}
}
