package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	argo "github.com/simjay/plous/internal/integrations/argo/monitor"
	flux "github.com/simjay/plous/internal/integrations/flux/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/source"
)

// Connection is a send-time snapshot. Local configuration stays out of prompts.
type Connection struct {
	SourceView     *source.ReadView   `json:"-"`
	TargetCaptured bool               `json:"-"`
	Target         *kubecommon.Target `json:"-"`
	ConnectionID   string             `json:"connection_id"`
	Name           string             `json:"name"`
	Tool           string             `json:"tool"`
	Context        string             `json:"context,omitempty"`
	Namespace      string             `json:"default_namespace,omitempty"`
	Path           string             `json:"-"`
	Driver         string             `json:"-"`
}

// ClusterConnections exposes both read capabilities of one captured cluster.
// The base identity is retained for existing saved connections and histories.
func ClusterConnections(c Connection) []Connection {
	other := c
	if c.Tool == "kubernetes" {
		other.Tool = "helm"
	} else if c.Tool == "helm" {
		other.Tool = "kubernetes"
	} else {
		return []Connection{c}
	}
	other.ConnectionID = c.ConnectionID + "/" + other.Tool
	return []Connection{c, other}
}

func (op *operation) hasTool(name string) bool {
	if op.o.WorkspaceID == "" {
		return false
	}
	for _, c := range op.o.Connections {
		if c.Tool == name {
			return true
		}
	}
	return false
}

// Preserve duplicate detection before removing the routing argument. The tool's
// own decoder still validates every remaining field and operation.
func connectionCommand(input string) (string, string, error) {
	d := json.NewDecoder(strings.NewReader(input))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return "", "", errors.New("read arguments must be one JSON object")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok || fields[key] != nil {
			return "", "", errors.New("invalid or duplicate read argument")
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || string(value) == "null" {
			return "", "", errors.New("invalid read argument")
		}
		fields[key] = value
	}
	if _, err = d.Token(); err != nil {
		return "", "", errors.New("invalid read arguments")
	}
	if _, err = d.Token(); err != io.EOF {
		return "", "", errors.New("read arguments must be one JSON object")
	}
	var id string
	if json.Unmarshal(fields["connection_id"], &id) != nil || id == "" {
		return "", "", errors.New("connection_id must identify a connection in the captured workspace")
	}
	delete(fields, "connection_id")
	return id, string(marshal(fields)), nil
}

func (op *operation) connection(id, tool string) (Connection, error) {
	if op.o.WorkspaceID != "" {
		for _, c := range op.o.Connections {
			if c.ConnectionID == id && (tool == "" || c.Tool == tool) && (c.Tool == "kubernetes" || c.Tool == "helm" || c.Tool == "source") {
				return c, nil
			}
		}
	}
	return Connection{}, errors.New("connection is unavailable for this read in the captured workspace")
}

func (op *operation) reading(c Connection, namespace string) {
	if op.o.Progress != nil {
		name := c.Name
		if name == "" {
			name = c.ConnectionID
		}
		op.o.Progress("Reading " + c.Tool + " / " + name + " / " + namespace)
	}
}

func (op *operation) readKubernetes(ctx context.Context, input string) (any, error) {
	id, args, err := connectionCommand(input)
	if err != nil {
		return nil, denyRead(err)
	}
	captured, err := op.connection(id, "kubernetes")
	if err != nil {
		return nil, denyRead(err)
	}
	command, err := kubernetes.DecodeCommand(args)
	if err != nil {
		return nil, denyRead(err)
	}
	namespace := captured.Namespace
	if command.Namespace != "" {
		namespace = command.Namespace
	}
	scope := kubernetes.CommandScope{ConnectionID: id, ConnectionName: captured.Name, Context: captured.Context, Namespace: namespace}
	preliminary := scope
	if preliminary.Namespace == "" {
		preliminary.Namespace = "default"
	}
	if err = kubernetes.ValidateRead(command, preliminary); err != nil {
		return nil, denyRead(err)
	}
	key := id + "\x00" + namespace
	op.mu.Lock()
	executor := op.kube[key]
	if executor == nil {
		if captured.TargetCaptured && captured.Target == nil {
			op.mu.Unlock()
			return nil, errors.New("the captured connection is unavailable; reselect it before retrying")
		}
		target := captured.Target
		var e error
		if target == nil {
			target, e = kubecommon.ResolveTarget(captured.Path, captured.Context, namespace)
		}
		if e != nil {
			op.mu.Unlock()
			return nil, errors.New("could not resolve the captured Kubernetes connection")
		}
		client, e := kubernetes.NewTarget(target, namespace)
		if e != nil {
			op.mu.Unlock()
			return nil, errors.New("could not open the captured Kubernetes connection")
		}
		if scope.Namespace == "" {
			scope.Namespace = client.Namespace
		}
		executor, e = kubernetes.NewProjector(client, scope, argo.ProjectFacts, flux.ProjectFacts)
		if e != nil {
			op.mu.Unlock()
			return nil, e
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

func (op *operation) readHelm(ctx context.Context, input string) (any, error) {
	id, args, err := connectionCommand(input)
	if err != nil {
		return nil, denyRead(err)
	}
	captured, err := op.connection(id, "helm")
	if err != nil {
		return nil, denyRead(err)
	}
	command, err := helmtool.DecodeCommand(args)
	if err != nil {
		return nil, denyRead(err)
	}
	namespace := captured.Namespace
	if command.Namespace != "" {
		namespace = command.Namespace
	}
	if captured.TargetCaptured && captured.Target == nil {
		return nil, errors.New("the captured connection is unavailable; reselect it before retrying")
	}
	config := helmtool.Config{Target: captured.Target, ConnectionID: id, Context: captured.Context, Namespace: namespace, Kubeconfig: captured.Path, Driver: captured.Driver}
	preliminary := config
	if preliminary.Namespace == "" {
		preliminary.Namespace = "default"
	}
	if err = helmtool.ValidateRead(command, preliminary); err != nil {
		return nil, denyRead(err)
	}
	key := id + "\x00" + namespace
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
			return nil, e
		}
		executor, e = helmtool.NewProjector(client, namespace)
		if e != nil {
			op.mu.Unlock()
			return nil, e
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
