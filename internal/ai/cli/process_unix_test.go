//go:build unix

package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestCancellationUnblocksInheritedStdout(t *testing.T) {
	// A detached descendant retains stdout after the installed runtime is killed.
	// It must not prevent Stop from returning; clean it up explicitly afterward.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p, err := startProcess(ctx, path, t.TempDir(), []string{"-test.run=^TestDetachedStdoutProcess$", "--", "inherited-stdout"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	var pid int
	if err := p.next(&pid); err != nil {
		t.Fatal(err)
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	ended := make(chan error, 1)
	go func() { var v any; ended <- p.next(&v) }()
	cancel()
	select {
	case err := <-ended:
		if err == nil {
			t.Fatal("canceled stream continued")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation remained blocked by inherited stdout")
	}
}

func TestDetachedStdoutProcess(t *testing.T) {
	if os.Args[len(os.Args)-1] != "inherited-stdout" {
		return
	}
	cmd := exec.Command("/bin/sleep", "30")
	cmd.Stdout = os.Stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if cmd.Start() != nil {
		os.Exit(8)
	}
	fmt.Fprintln(os.Stdout, cmd.Process.Pid)
	time.Sleep(30 * time.Second)
	os.Exit(0)
}
