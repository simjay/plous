package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

func environment() []string {
	// Only runtime, CLI authentication, proxy and certificate settings cross this boundary.
	names := []string{"PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "TMPDIR", "TMP", "TEMP", "LANG", "LC_ALL", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "ANTHROPIC_MODEL", "CLAUDE_CODE_OAUTH_TOKEN", "OPENAI_API_KEY", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS"}
	var env []string
	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return append(env, "DISABLE_AUTOUPDATER=1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1")
}

type process struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	cancel  context.CancelFunc
	stopIO  func() bool
}

func startProcess(ctx context.Context, path, dir string, args, extraEnv []string) (*process, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir, cmd.Env = dir, append(environment(), extraEnv...)
	cmd.WaitDelay = 2 * time.Second
	configureProcess(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		cancel()
		return nil, err
	}
	// CLI diagnostics can contain credentials and raw tool payloads. Report protocol errors instead.
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		in.Close()
		out.Close()
		cancel()
		return nil, fmt.Errorf("could not start installed agent: %w", err)
	}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	// A descendant can retain pipes after the agent exits. Cancellation must
	// unblock reads and writes before Wait can reap the process.
	stopIO := context.AfterFunc(ctx, func() { _ = in.Close(); _ = out.Close() })
	return &process{cmd: cmd, stdin: in, scanner: scanner, cancel: cancel, stopIO: stopIO}, nil
}
func (p *process) Close()           { p.cancel(); p.stdin.Close(); _ = p.cmd.Wait(); p.stopIO() }
func (p *process) send(v any) error { return json.NewEncoder(p.stdin).Encode(v) }
func (p *process) next(v any) error {
	if !p.scanner.Scan() {
		if err := p.scanner.Err(); err != nil {
			return fmt.Errorf("agent output: %w", err)
		}
		return errors.New("agent exited before a successful completion; check its installation and sign-in")
	}
	if err := json.Unmarshal(p.scanner.Bytes(), v); err != nil {
		return errors.New("installed agent returned an unsupported stream message")
	}
	return nil
}
