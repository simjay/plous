package debugging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/cli"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

const selectContextTool = "kubernetes_context_select"

type targetSelection struct {
	Kubeconfig string `json:"kubeconfig"`
	Context    string `json:"context"`
}

type targetStatus struct {
	Context     string          `json:"context,omitempty"`
	Namespace   string          `json:"default_namespace,omitempty"`
	Source      string          `json:"kubeconfig_source,omitempty"`
	Connections []ai.Connection `json:"connections,omitempty"`
	Error       string          `json:"error,omitempty"`
	Control     string          `json:"local_target_control,omitempty"`
}

func decodeTargetSelection(input io.Reader) (targetSelection, error) {
	var selection *targetSelection
	decoder := json.NewDecoder(input)
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&selection) != nil || selection == nil || decoder.Decode(&extra) != io.EOF {
		return targetSelection{}, errors.New("Expected an object with optional kubeconfig and context strings.")
	}
	if len(selection.Kubeconfig) > 4096 || len(selection.Context) > 256 {
		return targetSelection{}, errors.New("Kubeconfig path or context name is too long.")
	}
	return *selection, nil
}

func (s *standaloneTools) requestTarget(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	selection, err := decodeTargetSelection(bytes.NewReader(args))
	if err != nil {
		return json.Marshal(targetStatus{Error: err.Error()})
	}
	path, err := kubecommon.NormalizeConfigPaths(selection.Kubeconfig)
	if err != nil {
		return json.Marshal(targetStatus{Error: err.Error()})
	}
	selection.Kubeconfig = path
	source, name := "the default kubeconfig", "its current context"
	if path != "" {
		source = fmt.Sprintf("kubeconfig %q", path)
	}
	if selection.Context != "" {
		name = fmt.Sprintf("context %q", selection.Context)
	}
	message := "Use " + name + " from " + source + " for this Plous connection?"
	if s.config.policy.Exec || s.config.policy.Image != "" {
		message += " Enabled diagnostics will use the selected context and its default namespace."
	}
	approved, err := cli.Confirm(ctx, message)
	if ctx.Err() != nil {
		return nil, agent.ErrRequestCancelled
	}
	if err != nil {
		control, controlErr := s.control()
		if controlErr != nil {
			return json.Marshal(targetStatus{Error: "Could not open local target control. Check the Plous MCP host."})
		}
		return json.Marshal(targetStatus{Error: err.Error() + " Run " + targetUsage + " in a terminal on the MCP host. Use an empty kubeconfig argument to choose a named context in the default file.", Control: control})
	}
	if !approved {
		return json.Marshal(targetStatus{Error: "Target change was declined or cancelled."})
	}
	return json.Marshal(s.applyTarget(ctx, selection))
}

func (s *standaloneTools) applyTarget(ctx context.Context, selection targetSelection) targetStatus {
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return targetStatus{Error: "Target change was cancelled."}
	}
	if s.failure != nil || s.ctx.Err() != nil {
		return targetStatus{Error: "The Plous tool session is closed. Reconnect the client."}
	}
	source := kubeconfigSource(selection.Kubeconfig, "runtime selection")
	target, err := kubecommon.ResolveTarget(selection.Kubeconfig, selection.Context, "")
	if ctx.Err() != nil {
		return targetStatus{Error: "Target change was cancelled."}
	}
	if err == nil || selection == (targetSelection{}) {
		s.selection, s.pinned = &selection, nil
		if selection.Context != "" {
			s.pinned = target
		}
	}
	return statusForTarget(target, source, err)
}

func (s *standaloneTools) resolveTarget() (*kubecommon.Target, string, error) {
	paths, name, namespace, source := s.config.paths, s.config.context, s.config.namespace, "--kubeconfig"
	if s.selection != nil {
		paths, name, namespace, source = s.selection.Kubeconfig, s.selection.Context, "", "runtime selection"
	}
	source = kubeconfigSource(paths, source)
	if s.pinned != nil {
		return s.pinned, source, nil
	}
	target, err := kubecommon.ResolveTarget(paths, name, namespace)
	if err == nil && name != "" {
		s.pinned = target
	}
	return target, source, err
}

func kubeconfigSource(paths, override string) string {
	if paths != "" {
		return override
	}
	if os.Getenv("KUBECONFIG") != "" {
		return "KUBECONFIG"
	}
	return "default (~/.kube/config)"
}

func clusterConnections(target *kubecommon.Target) []ai.Connection {
	return ai.ClusterConnections(ai.Connection{
		ConnectionID: "kube-" + target.Fingerprint, Name: target.Context, Tool: "kubernetes",
		Context: target.Context, Namespace: target.Namespace, Target: target, TargetCaptured: true,
	})
}

func statusForTarget(target *kubecommon.Target, source string, err error) targetStatus {
	if err != nil {
		return targetStatus{Error: err.Error(), Source: source}
	}
	return targetStatus{Context: target.Context, Namespace: target.Namespace, Source: source, Connections: clusterConnections(target)}
}
