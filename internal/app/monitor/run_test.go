package monitor

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMonitorStopWaitsForLockAndHonorsCancellation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "payments")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	lock, err := lockMonitor(dir, "payments")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	controlDir, err := os.MkdirTemp("/tmp", "plous-stop-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(controlDir)
	address := filepath.Join(controlDir, "stop")
	listener, err := net.Listen("unix", address)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := save(dir, storedState{State: State{Running: true}, Control: address}); err != nil {
		t.Fatal(err)
	}

	requested := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_, err = io.Copy(io.Discard, conn)
			conn.Close()
		}
		requested <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- stop(ctx, root, "payments") }()
	select {
	case err := <-requested:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("stop request did not reach the worker")
	}

	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("stop returned before the lock was released: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceling the command did not end the shutdown wait")
	}
}
