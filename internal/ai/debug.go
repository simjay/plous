package ai

import (
	"context"
	"errors"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/integrations/observation"
)

func (op *operation) debugEnabled(name string) bool {
	return name == "kubernetes_exec_run" && op.o.Debug.Exec || name == "kubernetes_diagnostic_run" && op.o.Debug.Image != ""
}

func (op *operation) debugTool(ctx context.Context, name, input string) (observation.Result, error) {
	var empty observation.Result
	id, args, err := connectionCommand(input)
	if err != nil {
		return empty, denyRead(err)
	}
	captured, err := op.connection(id, "kubernetes")
	if err != nil {
		return empty, denyRead(err)
	}
	namespace := captured.Namespace
	if namespace == "" {
		namespace = "default"
	}
	command, err := kubernetes.DecodeDebug(name, args, op.o.Debug, namespace)
	if err != nil {
		return empty, denyRead(err)
	}
	if captured.TargetCaptured && captured.Target == nil {
		return empty, errors.New("captured Kubernetes target is unavailable")
	}
	target := captured.Target
	if target == nil {
		target, err = kubecommon.ResolveTarget(captured.Path, captured.Context, namespace)
		if err != nil {
			return empty, errors.New("could not resolve captured Kubernetes target")
		}
	}
	client, err := kubernetes.NewTarget(target, namespace)
	if err != nil {
		return empty, errors.New("could not initialize captured Kubernetes target")
	}
	return client.Debug(ctx, name, command, op.o.Debug, id)
}
