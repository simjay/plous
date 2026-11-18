package debugging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

const targetUsage = "plous mcp target CONTROL [KUBECONFIG [CONTEXT]]"
const targetControlTimeout = 30 * time.Second

// The private Unix socket gives the local operator a target selector when the MCP
// client cannot present a confirmation. It is opened only when requested.
func (s *standaloneTools) openTargetControl(ctx context.Context, workers *sync.WaitGroup) (string, error) {
	// Unix socket names have a small platform limit, even when TMPDIR is long.
	dir, err := os.MkdirTemp("/tmp", "plous-target-")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "control.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		_ = os.Remove(dir)
		return "", err
	}
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	workers.Go(func() {
		defer os.Remove(dir)
		defer listener.Close()
		defer stop()
		for {
			conn, err := listener.AcceptUnix()
			if err != nil {
				return
			}
			handleTargetControl(ctx, conn, s)
		}
	})
	return path, nil
}

func handleTargetControl(ctx context.Context, conn *net.UnixConn, s *standaloneTools) {
	defer conn.Close()
	ctx, cancel := context.WithTimeout(ctx, targetControlTimeout)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	selection, err := decodeTargetSelection(io.LimitReader(conn, 8192))
	if err != nil {
		_ = json.NewEncoder(conn).Encode(targetStatus{Error: err.Error()})
		return
	}
	_ = json.NewEncoder(conn).Encode(s.applyTarget(ctx, selection))
}

func runTargetControl(ctx context.Context, args []string, output io.Writer) error {
	if len(args) == 0 || len(args) > 3 {
		return errors.New("Usage: " + targetUsage)
	}
	var selection targetSelection
	if len(args) >= 2 {
		var err error
		selection.Kubeconfig, err = kubecommon.NormalizeConfigPaths(args[1])
		if err != nil {
			return err
		}
	}
	if len(args) == 3 {
		selection.Context = args[2]
	}
	dial := net.Dialer{Timeout: 3 * time.Second}
	stream, err := dial.DialContext(ctx, "unix", args[0])
	if err != nil {
		return errors.New("Cannot reach this Plous connection. Ask the agent for its current target control.")
	}
	conn := stream.(*net.UnixConn)
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(targetControlTimeout))
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if err = json.NewEncoder(conn).Encode(selection); err != nil {
		return err
	}
	if err = conn.CloseWrite(); err != nil {
		return err
	}
	var result targetStatus
	if err = json.NewDecoder(io.LimitReader(conn, 8192)).Decode(&result); err != nil {
		return fmt.Errorf("Could not confirm the target change. Ask the agent to run kubernetes_context_read: %w", err)
	}
	if result.Error != "" {
		return errors.New(result.Error)
	}
	_, err = fmt.Fprintf(output, "Context: %q\nDefault namespace: %q\nKubeconfig source: %s\n", result.Context, result.Namespace, result.Source)
	return err
}
