package ai

import (
	"context"
	"encoding/json"
	"errors"

	argo "github.com/simjay/plous/internal/integrations/argo/monitor"
	flux "github.com/simjay/plous/internal/integrations/flux/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/integrations/observation"
)

func (op *operation) reading(c Connection, namespace string) {
	if op.o.Progress != nil {
		name := c.Name
		if name == "" {
			name = c.ConnectionID
		}
		op.o.Progress("Reading " + c.Tool + " / " + name + " / " + namespace)
	}
}

func (op *operation) readKubernetes(ctx context.Context, captured Connection, args map[string]json.RawMessage) (observation.Result, error) {
	return op.readKubernetesCommand(ctx, captured, args, nil)
}

func (op *operation) kubernetesReader(validate func(kubernetes.Command) error) func(context.Context, Connection, map[string]json.RawMessage) (observation.Result, error) {
	return func(ctx context.Context, captured Connection, args map[string]json.RawMessage) (observation.Result, error) {
		return op.readKubernetesCommand(ctx, captured, args, validate)
	}
}

func (op *operation) readKubernetesCommand(ctx context.Context, captured Connection, args map[string]json.RawMessage, validate func(kubernetes.Command) error) (observation.Result, error) {
	command, err := kubernetes.DecodeCommand(args)
	if err != nil {
		return observation.Result{}, denyRead(err)
	}
	if validate != nil {
		if err := validate(command); err != nil {
			return observation.Result{}, denyRead(err)
		}
	}
	namespace := captured.Namespace
	if command.Namespace != "" {
		namespace = command.Namespace
	}
	scope := kubernetes.CommandScope{ConnectionID: captured.ConnectionID, ConnectionName: captured.Name, Context: captured.Context, Namespace: namespace}
	preliminary := scope
	if preliminary.Namespace == "" {
		preliminary.Namespace = "default"
	}
	if err = kubernetes.ValidateRead(command, preliminary); err != nil {
		return observation.Result{}, denyRead(err)
	}
	key := captured.ConnectionID + "\x00" + namespace
	op.mu.Lock()
	executor := op.kube[key]
	if executor == nil {
		if captured.TargetCaptured && captured.Target == nil {
			op.mu.Unlock()
			return observation.Result{}, errors.New("the captured connection is unavailable; reselect it before retrying")
		}
		target := captured.Target
		var e error
		if target == nil {
			target, e = kubecommon.ResolveTarget(captured.Path, captured.Context, namespace)
		}
		if e != nil {
			op.mu.Unlock()
			return observation.Result{}, errors.New("could not resolve the captured Kubernetes connection")
		}
		client, e := kubernetes.NewTarget(target, namespace)
		if e != nil {
			op.mu.Unlock()
			return observation.Result{}, errors.New("could not open the captured Kubernetes connection")
		}
		if scope.Namespace == "" {
			scope.Namespace = client.Namespace
		}
		executor, e = kubernetes.NewProjector(client, scope, argo.ProjectFacts, flux.ProjectFacts)
		if e != nil {
			op.mu.Unlock()
			return observation.Result{}, e
		}
		if op.kube == nil {
			op.kube = map[string]*kubernetes.Projector{}
		}
		op.kube[key] = executor
	}
	op.mu.Unlock()
	op.reading(captured, executor.Namespace())
	result, err := executor.Execute(ctx, command)
	return result, err
}

func (op *operation) readHelm(ctx context.Context, captured Connection, args map[string]json.RawMessage) (observation.Result, error) {
	command, err := helmtool.DecodeCommand(args)
	if err != nil {
		return observation.Result{}, denyRead(err)
	}
	namespace := captured.Namespace
	if command.Namespace != "" {
		namespace = command.Namespace
	}
	if captured.TargetCaptured && captured.Target == nil {
		return observation.Result{}, errors.New("the captured connection is unavailable; reselect it before retrying")
	}
	config := helmtool.Config{Target: captured.Target, ConnectionID: captured.ConnectionID, Context: captured.Context, Namespace: namespace, Kubeconfig: captured.Path, Driver: captured.Driver}
	preliminary := config
	if preliminary.Namespace == "" {
		preliminary.Namespace = "default"
	}
	if err = helmtool.ValidateRead(command, preliminary); err != nil {
		return observation.Result{}, denyRead(err)
	}
	key := captured.ConnectionID + "\x00" + namespace
	op.mu.Lock()
	executor := op.helm[key]
	if executor == nil {
		// The Helm client uses a concrete default; the read executor can bind a
		// list to all namespaces without changing that client's write defaults.
		if config.Namespace == "*" {
			config.Namespace = ""
		}
		client, e := helmtool.New(config)
		if e != nil {
			op.mu.Unlock()
			return observation.Result{}, e
		}
		executor, e = helmtool.NewProjector(client, namespace)
		if e != nil {
			op.mu.Unlock()
			return observation.Result{}, e
		}
		if op.helm == nil {
			op.helm = map[string]*helmtool.Projector{}
		}
		op.helm[key] = executor
	}
	op.mu.Unlock()
	op.reading(captured, executor.Namespace())
	result, err := executor.Execute(ctx, command)
	return result, err
}

func (op *operation) runSchema() map[string]any {
	var variants []observation.OperationVariant
	for _, operation := range op.o.Debug.Operations() {
		variants = append(variants, operationVariant(operation, kubernetes.DebugSchema(operation)))
	}
	return observation.OperationSchema(nil, variants)
}

func (op *operation) debugTool(ctx context.Context, captured Connection, args map[string]json.RawMessage) (observation.Result, error) {
	var empty observation.Result
	namespace := captured.Namespace
	if namespace == "" {
		namespace = "default"
	}
	operation := stringArgument(args, "operation")
	delete(args, "operation")
	command, err := kubernetes.DecodeDebug(operation, args, op.o.Debug, namespace)
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
	return client.Debug(ctx, operation, command, op.o.Debug, captured.ConnectionID)
}
