package terminal

import (
	"bytes"
	"fmt"

	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestTerminalOutputBelongsToRenderer(t *testing.T) {
	if mode := os.Getenv("PLOUS_TEST_OUTPUT_CHILD"); mode != "" {
		fmt.Fprintln(os.Stdout, "before")
		output, err := ReserveOutput()
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
		fmt.Fprintln(output.Renderer, "frame")
		fmt.Fprintln(output.Diagnostics, "explicit error")
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
