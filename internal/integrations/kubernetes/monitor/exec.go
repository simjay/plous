package monitor

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
	"k8s.io/streaming/pkg/httpstream"
)

type ExecOptions struct {
	Namespace, Pod, Container string
	UID                       types.UID
	Command                   []string
	Stdin                     io.Reader
	Stdout, Stderr            io.Writer
	TTY                       bool
	SizeQueue                 remotecommand.TerminalSizeQueue
}

func (c *Client) Exec(ctx context.Context, options ExecOptions) error {
	if !validPathPart(options.Namespace) || !validPathPart(options.Pod) || options.UID == "" || options.Container == "" || len(options.Command) == 0 || options.Command[0] == "" {
		return operationError("Choose a running Pod, container, and command before opening Exec.")
	}
	podResource := kubecommon.Resource{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true}
	pod, err := c.Get(ctx, podResource, options.Namespace, options.Pod)
	if err != nil {
		return err
	}
	if pod.GetUID() != options.UID {
		return operationError("This Pod was replaced. Reopen Exec to choose the current Pod.")
	}
	if pod.GetDeletionTimestamp() != nil || !runningContainer(*pod, options.Container) {
		return operationError("The selected container is not running or the Pod is terminating.")
	}
	request := c.rest.Post().AbsPath("/api/v1/namespaces", options.Namespace, "pods", options.Pod, "exec").
		Param("container", options.Container).Param("stdin", fmt.Sprint(options.Stdin != nil)).
		Param("stdout", fmt.Sprint(options.Stdout != nil)).Param("stderr", fmt.Sprint(options.Stderr != nil && !options.TTY)).
		Param("tty", fmt.Sprint(options.TTY))
	for _, argument := range options.Command {
		request.Param("command", argument)
	}
	endpoint := request.URL()
	websocket, err := remotecommand.NewWebSocketExecutor(c.config, http.MethodGet, endpoint.String())
	if err != nil {
		return err
	}
	spdy, err := remotecommand.NewSPDYExecutor(c.config, http.MethodPost, endpoint)
	if err != nil {
		return err
	}
	executor, err := remotecommand.NewFallbackExecutor(websocket, spdy, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})
	if err != nil {
		return err
	}
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin: options.Stdin, Stdout: options.Stdout, Stderr: options.Stderr,
		Tty: options.TTY, TerminalSizeQueue: options.SizeQueue,
	})
	var exit utilexec.ExitError
	if errors.As(err, &exit) {
		return operationError(fmt.Sprintf("The command exited with status %d.", exit.ExitStatus()))
	}
	return err
}

func runningContainer(pod unstructured.Unstructured, name string) bool {
	for _, field := range []string{"containerStatuses", "initContainerStatuses", "ephemeralContainerStatuses"} {
		statuses, _, _ := unstructured.NestedSlice(pod.Object, "status", field)
		for _, entry := range statuses {
			status, ok := entry.(map[string]any)
			if !ok || status["name"] != name {
				continue
			}
			_, running, _ := unstructured.NestedMap(status, "state", "running")
			return running
		}
	}
	return false
}

// Exec diagnostics may contain executable errors that a generic API message
// would hide. Remove transport credentials before showing the complete cause.
func (c *Client) ExecErrorText(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if c != nil && c.config != nil {
		cfg := c.config
		secrets := []string{cfg.BearerToken, cfg.Password, string(cfg.KeyData)}
		if cfg.Username != "" || cfg.Password != "" {
			secrets = append(secrets, base64.StdEncoding.EncodeToString([]byte(cfg.Username+":"+cfg.Password)))
		}
		for _, secret := range secrets {
			if secret != "" {
				text = strings.ReplaceAll(text, secret, "[redacted]")
			}
		}
	}
	text = execURLPattern.ReplaceAllStringFunc(text, func(raw string) string {
		u, parseErr := url.Parse(raw)
		if parseErr != nil {
			return "[connection URL]"
		}
		u.User = nil
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	})
	text = execAuthPattern.ReplaceAllString(text, "${1}[redacted]")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
}

var execURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)
var execAuthPattern = regexp.MustCompile(`(?i)(authorization[:=]\s*(?:bearer|basic)\s+)[^\s,;]+`)
