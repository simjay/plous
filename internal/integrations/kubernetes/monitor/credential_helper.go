package monitor

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	osexec "os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	kubeexec "k8s.io/client-go/plugin/pkg/client/auth/exec"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	CredentialHelperCommand = "__credential-helper"
	credentialHelperTimeout = 20 * time.Second
)

var credentialHelpers atomic.Pointer[credentialHelperScope]

type credentialInvocation struct {
	Path      string
	Command   string
	Arguments []string
	Policy    clientcmdapi.PolicyType
	Allowlist []string
}

type credentialHelperScope struct {
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	dir      string
	listener *net.UnixListener
	helpers  map[string][]byte
}

// StartCredentialHelpers binds exec plugins to the lifetime of the application host.
func StartCredentialHelpers() func() {
	ctx, cancel := context.WithCancel(context.Background())
	scope := &credentialHelperScope{ctx: ctx, cancel: cancel, helpers: make(map[string][]byte)}
	credentialHelpers.Store(scope)
	return scope.close
}

func (s *credentialHelperScope) socket() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return "", err
	}
	if s.listener != nil {
		return s.listener.Addr().String(), nil
	}
	// Keep the path within sockaddr_un's limit, including on macOS.
	dir, err := os.MkdirTemp("/tmp", "plous-credentials-")
	if err != nil {
		return "", err
	}
	address := filepath.Join(dir, "lifetime.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: address, Net: "unix"})
	if err != nil {
		_ = os.Remove(dir)
		return "", err
	}
	s.dir, s.listener = dir, listener
	go func() {
		for {
			connection, err := listener.AcceptUnix()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				stop := context.AfterFunc(s.ctx, func() { _ = connection.Close() })
				defer stop()
				_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
				var id string
				if json.NewDecoder(io.LimitReader(connection, 128)).Decode(&id) != nil {
					return
				}
				s.mu.Lock()
				invocation := s.helpers[id]
				s.mu.Unlock()
				if invocation == nil {
					return
				}
				if _, err := connection.Write(invocation); err != nil {
					return
				}
				_ = connection.SetDeadline(time.Time{})
				// Keep the connection open so the process can observe shutdown.
				var unused [1]byte
				_, _ = connection.Read(unused[:])
			}()
		}
	}()
	return address, nil
}

func (s *credentialHelperScope) register(invocation credentialInvocation) (string, string, error) {
	encoded, err := json.Marshal(invocation)
	if err != nil {
		return "", "", err
	}
	address, err := s.socket()
	if err != nil {
		return "", "", err
	}
	id := fmt.Sprintf("%x", sha256.Sum256(encoded))
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return "", "", err
	}
	s.helpers[id] = append(encoded, '\n')
	return address, id, nil
}

func (s *credentialHelperScope) close() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		_ = s.listener.Close()
		_ = os.Remove(s.dir)
		s.listener = nil
	}
	clear(s.helpers)
}

// PrepareExecCredentials keeps client-go's cache and ExecCredential handling,
// while giving the application ownership of the helper's subprocess lifetime.
func PrepareExecCredentials(config *rest.Config) error {
	if config.ExecProvider == nil {
		return nil
	}
	provider := config.ExecProvider.DeepCopy()
	provider.StdinUnavailable = true
	provider.StdinUnavailableMessage = "Authenticate outside Plous, then refresh."
	scope := credentialHelpers.Load()
	if scope != nil {
		if err := kubeexec.ValidatePluginPolicy(provider.PluginPolicy); err != nil {
			return errors.New("invalid Kubernetes credential helper policy")
		}
		executable, err := os.Executable()
		if err != nil {
			return errors.New("cannot locate the Plous credential helper runner")
		}
		invocation := credentialInvocation{
			Path: os.Getenv("PATH"), Command: filepath.Clean(provider.Command),
			Arguments: provider.Args, Policy: provider.PluginPolicy.PolicyType,
		}
		for _, entry := range provider.PluginPolicy.Allowlist {
			invocation.Allowlist = append(invocation.Allowlist, entry.Command)
		}
		address, id, err := scope.register(invocation)
		if err != nil {
			return fmt.Errorf("start credential helper supervision: %w", err)
		}
		provider.Command = executable
		provider.Args = []string{CredentialHelperCommand, address, id}
		// The runner enforces the original executable's policy at each invocation.
		provider.PluginPolicy = clientcmdapi.PluginPolicy{
			PolicyType: clientcmdapi.PluginPolicyAllowlist,
			Allowlist:  []clientcmdapi.AllowlistEntry{{Command: executable}},
		}
	}
	config.ExecProvider = provider
	return nil
}

func authorizedCredentialHelper(invocation credentialInvocation) (string, error) {
	switch invocation.Policy {
	case "", clientcmdapi.PluginPolicyAllowAll, clientcmdapi.PluginPolicyAllowlist:
	case clientcmdapi.PluginPolicyDenyAll:
		return "", errors.New("Kubernetes credential helpers are disabled by policy")
	default:
		return "", errors.New("invalid Kubernetes credential helper policy")
	}
	command := invocation.Command
	path, err := osexec.LookPath(command)
	if err != nil {
		return "", errors.New("Kubernetes credential helper is unavailable; check the kubeconfig and PATH")
	}
	if invocation.Policy != clientcmdapi.PluginPolicyAllowlist {
		return path, nil
	}
	for _, entry := range invocation.Allowlist {
		if entry == command || entry == path {
			return path, nil
		}
		if filepath.Base(entry) == filepath.Base(path) {
			if resolved, err := osexec.LookPath(entry); err == nil && resolved == path {
				return path, nil
			}
		}
	}
	return "", errors.New("Kubernetes credential helper is not allowed by policy")
}

// RunCredentialHelper is the process entry point used by client-go.
func RunCredentialHelper(args []string, output, diagnostics io.Writer) int {
	if len(args) != 2 {
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connection, invocation, err := registeredCredentialHelper(ctx, args[0], args[1])
	if err != nil {
		fmt.Fprintln(diagnostics, "Plous: Kubernetes credential helper is unavailable.")
		return 1
	}
	defer connection.Close()
	// client-go resolves the executable before applying the plugin environment.
	// Use Plous's PATH for lookup and restore the plugin's PATH for execution.
	pluginPath, hasPluginPath := os.LookupEnv("PATH")
	_ = os.Setenv("PATH", invocation.Path)
	path, err := authorizedCredentialHelper(invocation)
	if hasPluginPath {
		_ = os.Setenv("PATH", pluginPath)
	} else {
		_ = os.Unsetenv("PATH")
	}
	if err != nil {
		fmt.Fprintln(diagnostics, "Plous:", err)
		return 1
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		var unused [1]byte
		_, _ = connection.Read(unused[:])
		cancel()
	}()
	err = runCredentialHelper(ctx, path, append([]string{invocation.Command}, invocation.Arguments...), output, credentialHelperTimeout)
	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintln(diagnostics, "Plous: Kubernetes credential helper timed out.")
		return 124
	}
	if errors.Is(err, context.Canceled) {
		return 1
	}
	if err != nil {
		fmt.Fprintln(diagnostics, "Plous: Kubernetes credential helper did not complete. Check your cluster login.")
		return 1
	}
	return 0
}

func registeredCredentialHelper(ctx context.Context, address, id string) (*net.UnixConn, credentialInvocation, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	raw, err := (&net.Dialer{}).DialContext(ctx, "unix", address)
	if err != nil {
		return nil, credentialInvocation{}, err
	}
	connection := raw.(*net.UnixConn)
	if pid, err := credentialSupervisorPID(connection); err != nil || pid != os.Getppid() {
		connection.Close()
		return nil, credentialInvocation{}, errors.New("credential supervisor does not belong to the Plous parent")
	}
	deadline, _ := ctx.Deadline()
	_ = connection.SetDeadline(deadline)
	var invocation credentialInvocation
	err = json.NewEncoder(connection).Encode(id)
	if err == nil {
		err = json.NewDecoder(io.LimitReader(connection, 1<<20)).Decode(&invocation)
	}
	if err != nil || invocation.Command == "" {
		connection.Close()
		return nil, credentialInvocation{}, errors.New("credential helper was not registered")
	}
	_ = connection.SetDeadline(time.Time{})
	return connection, invocation, nil
}

func runCredentialHelper(ctx context.Context, path string, args []string, output io.Writer, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := osexec.CommandContext(ctx, path, args[1:]...)
	cmd.Args[0] = args[0]
	// Own a pipe so descendants cannot retain client-go's stdout descriptor.
	cmd.Stdout = struct{ io.Writer }{output}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
