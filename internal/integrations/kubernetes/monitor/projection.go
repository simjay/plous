package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kjson "k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/rest"
)

const maxAgentResponseBytes = 1024 * 1024

var errProjectionRedirect = errors.New("Kubernetes read redirects are unavailable")

// Projector is the only Kubernetes capability exposed to assistant tools.
// It keeps the transport and captured scope private and validates every call.
type Projector struct {
	client      rest.Interface
	scope       CommandScope
	credentials []string
	projections []ResourceProjection
}

type ResourceProjection func(kubecommon.Resource, map[string]any, map[string]observation.Value, time.Time) (bool, error)

func NewProjector(client *Client, scope CommandScope, projections ...ResourceProjection) (*Projector, error) {
	if client == nil || client.rest == nil || client.config == nil {
		return nil, errors.New("Kubernetes read client is unavailable")
	}
	if scope.Namespace == "" {
		scope.Namespace = client.Namespace
	}
	if err := ValidateRead(Command{Operation: "list", Resource: "pods"}, scope); err != nil {
		return nil, err
	}
	config := rest.CopyConfig(client.config)
	config.WarningHandlerWithContext = rest.NoWarnings{}
	raw, err := guardedREST(config)
	if err != nil {
		return nil, err
	}
	return &Projector{
		client: raw, scope: scope, projections: projections,
		credentials: []string{config.BearerToken, config.Password, string(config.KeyData), string(config.CertData)},
	}, nil
}

func guardedREST(config *rest.Config) (rest.Interface, error) {
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, errors.New("cannot initialize scoped Kubernetes reads")
	}
	// Copy the HTTP client because client-go can return http.DefaultClient.
	// Redirects must not turn a validated endpoint into a different operation.
	guarded := *httpClient
	guarded.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errProjectionRedirect
	}
	raw, err := rest.UnversionedRESTClientForConfigAndClient(config, &guarded)
	if err != nil {
		return nil, errors.New("cannot initialize scoped Kubernetes reads")
	}
	return raw, nil
}

func (e *Projector) Execute(ctx context.Context, command Command) (result observation.Result, err error) {
	operation := command.Operation
	if operation == "list" {
		operation = "get" // Retain compatibility with saved collection observations.
	}
	result = observation.New("kubernetes/"+command.Resource, operation, time.Now().UTC())
	result.Connection, result.Namespace = e.scope.ConnectionID, e.scope.Namespace
	result.Selection = map[string]string{"resource": command.Resource, "name": command.Name, "label_selector": command.LabelSelector, "field_selector": command.FieldSelector}
	defer func() {
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		err = result.FinishRead(err)
		if command.Operation != "logs" && command.Operation != "usage" && command.Operation != "discover" {
			collectionFacts(&result, command.Resource)
		}
	}()
	if err := ValidateRead(command, e.scope); err != nil {
		return result, err
	}
	if len(command.Paths) > 0 {
		paths := slices.Clone(command.Paths)
		slices.Sort(paths)
		selection, _ := json.Marshal(paths)
		result.Selection["paths"] = string(selection)
	}
	command.Namespace = e.scope.Namespace
	if command.MaxBytes == 0 {
		command.MaxBytes = MaxAgentBytes
	}
	if command.Operation == "usage" {
		return e.usage(ctx, command, result)
	}
	if command.Operation == "logs" {
		if command.Resource != "pods" {
			return e.workloadLogSnapshot(ctx, command, result)
		}
		return e.logs(ctx, command, result)
	}
	if command.Operation == "discover" {
		return e.projectCatalog(ctx, command.Query, result)
	}
	resource, err := e.resolveResource(ctx, command.Resource, command.APIVersion, command.Operation)
	if err != nil {
		return result, err
	}
	if !resource.Namespaced || command.Namespace == "*" {
		command.Namespace = ""
	}
	if !resource.Namespaced {
		result.Namespace = ""
	}
	result.Selection["api_version"] = resource.Group + "/" + resource.Version
	path := resourcePath(resource, command.Namespace)
	if command.Name != "" {
		path += "/" + command.Name
	}
	result.Facts["label_selector"] = observation.Text(command.LabelSelector)
	result.Facts["field_selector"] = observation.Text(command.FieldSelector)
	continuation := ""
	seen := map[string]observation.Row{}
	cursors := map[string]bool{}
	snapshot := ""
	for {
		request := e.client.Get().AbsPath(path)
		if command.Name == "" {
			request.Param("limit", "50").Param("labelSelector", command.LabelSelector).Param("fieldSelector", command.FieldSelector).Param("continue", continuation)
		}
		var object map[string]any
		if err = readAgentJSON(ctx, request, maxAgentResponseBytes, &object); err != nil {
			return result, err
		}
		var objects []map[string]any
		next := ""
		if command.Name != "" {
			objects = []map[string]any{object}
		} else {
			items, ok := object["items"].([]any)
			if !ok {
				return result, errors.New("Kubernetes returned an invalid resource list")
			}
			metadata, _ := object["metadata"].(map[string]any)
			current, _ := metadata["resourceVersion"].(string)
			next, _ = metadata["continue"].(string)
			if snapshot != "" && current != snapshot {
				result.Partial("collection snapshot changed")
				break
			}
			snapshot = current
			if len(next) > 4096 || next != "" && cursors[next] {
				result.Partial("invalid continuation chain")
				break
			}
			cursors[next] = true
			for _, item := range items {
				o, ok := item.(map[string]any)
				if !ok {
					return result, errors.New("Kubernetes returned an invalid resource")
				}
				objects = append(objects, o)
			}
		}
		for _, o := range objects {
			safe := e.sanitize(o).(map[string]any)
			row, projectErr := projectResource(resource, safe, result.ObservedAt)
			if projectErr != nil {
				return result, projectErr
			}
			for _, project := range e.projections {
				handled, err := project(resource, safe, row.Facts, result.ObservedAt)
				if err != nil {
					return result, err
				}
				if handled {
					delete(row.Facts, "projection_coverage")
					break
				}
			}
			e.projectPaths(o, row.Facts, command.Paths)
			if command.Name != "" && row.Identity.Name != command.Name || resource.Namespaced && e.scope.Namespace != "*" && row.Identity.Namespace != command.Namespace {
				return result, errors.New("Kubernetes returned an object outside the requested identity")
			}
			key := row.Identity.Key()
			if previous, exists := seen[key]; exists {
				if !reflect.DeepEqual(previous, row) {
					return result, errors.New("Kubernetes returned conflicting observations for one resource identity")
				}
				continue
			}
			seen[key] = row
			result.Rows = append(result.Rows, row)
			result.Coverage.Observed++
		}
		if len(result.Rows) > 10000 || encodedSize(result) > 4<<20 {
			return result, errors.New("projected collection exceeds storage allowance; narrow the query")
		}
		result.Coverage.Snapshot = snapshot
		if next == "" {
			result.Complete()
			break
		}
		if len(result.Rows) >= 10000 || encodedSize(result) > 4<<20 {
			result.Partial("projected collection reached its storage bound")
			break
		}
		continuation = next
	}
	return result, nil
}

func collectionFacts(result *observation.Result, resource string) {
	result.Facts["observed_total"] = observation.Number(int64(result.Coverage.Observed))
	result.Facts["total"] = observation.Unknown()
	if result.Coverage.Complete {
		result.Facts["total"] = observation.Number(int64(result.Coverage.Observed))
	}
	if resource == "pods" {
		ready, unready, unknown := int64(0), int64(0), int64(0)
		for _, row := range result.Rows {
			v := row.Facts["ready"]
			if v.Boolean == nil {
				unknown++
			} else if *v.Boolean {
				ready++
			} else {
				unready++
			}
		}
		for name, count := range map[string]int64{"ready": ready, "unready": unready, "readiness_unknown": unknown} {
			result.Facts["observed_"+name] = observation.Number(count)
			result.Facts[name] = observation.Unknown()
			if result.Coverage.Complete && (unknown == 0 || name == "readiness_unknown") {
				result.Facts[name] = observation.Number(count)
			}
		}
	}
}

func (e *Projector) readDiscovery(ctx context.Context, path, accept string, target any) error {
	request := e.client.Get().AbsPath(path)
	if accept != "" {
		request.SetHeader("Accept", accept)
	}
	return readAgentJSON(ctx, request, maxAgentResponseBytes, target)
}

func (e *Projector) discoverResource(ctx context.Context, identity string) (kubecommon.Resource, error) {
	return e.resolveResource(ctx, identity, "", "")
}

func (e *Projector) resolveResource(ctx context.Context, identity, version, verb string) (kubecommon.Resource, error) {
	catalog, err := discoverCatalog(ctx, e.readDiscovery, identity)
	if err != nil {
		return kubecommon.Resource{}, err
	}
	if resource, ok := catalog.Resolve(identity, version, verb); ok {
		return resource, nil
	}
	message := "Kubernetes discovery did not find the requested resource, version and read verb"
	if len(catalog.Missing) > 0 {
		message += "; discovery was incomplete"
	}
	return kubecommon.Resource{}, &observation.ReadFailure{Message: message}
}

func projectedReadError(ctx context.Context, err error) error {
	if errors.Is(err, errProjectionRedirect) {
		return errProjectionRedirect
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return &observation.ReadFailure{Message: ErrorText(err), Denied: apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err)}
}

func readAgentJSON(ctx context.Context, request *rest.Request, limit int64, target any) error {
	body, err := readAgentBytes(ctx, request, limit, 20*time.Second)
	if err != nil {
		return err
	}
	if int64(len(body)) > limit {
		return errors.New("Kubernetes response exceeded the read limit; request fewer resources")
	}
	if err := kjson.Unmarshal(body, target); err != nil {
		return errors.New("Kubernetes returned an invalid JSON response")
	}
	return nil
}

func readAgentBytes(ctx context.Context, request *rest.Request, limit int64, timeout time.Duration) ([]byte, error) {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stream, err := request.MaxRetries(0).Stream(requestCtx)
	if err != nil {
		return nil, projectedReadError(ctx, err)
	}
	defer stream.Close()
	body, err := io.ReadAll(io.LimitReader(stream, limit+1))
	if err != nil {
		return nil, projectedReadError(ctx, err)
	}
	return body, nil
}

func (e *Projector) logs(ctx context.Context, command Command, result observation.Result) (observation.Result, error) {
	if command.TailLines == 0 {
		command.TailLines = DefaultAgentLines
	}
	result.Facts["container"] = observation.Text(command.Container)
	result.Selection["container"] = command.Container
	result.Selection["previous"] = fmt.Sprint(command.Previous)
	request := e.client.Get().AbsPath("/api/v1/namespaces/"+command.Namespace+"/pods/"+command.Name+"/log").
		Param("container", command.Container).Param("tailLines", fmt.Sprint(command.TailLines)).
		Param("limitBytes", fmt.Sprint(command.MaxBytes)).Param("timestamps", "true").
		Param("previous", fmt.Sprint(command.Previous)).Param("follow", "false")
	if command.SinceSeconds != 0 {
		request.Param("sinceSeconds", fmt.Sprint(command.SinceSeconds))
	}
	body, err := readAgentBytes(ctx, request, int64(command.MaxBytes), 30*time.Second)
	if err != nil {
		return result, err
	}
	truncated := len(body) >= command.MaxBytes
	if len(body) > command.MaxBytes {
		body = body[:command.MaxBytes]
	}
	lines := strings.SplitAfter(string(body), "\n")
	if len(lines) > command.TailLines {
		body = []byte(strings.Join(lines[:command.TailLines], ""))
		truncated = true
	}
	text := e.sanitizeText(strings.ToValidUTF8(string(body), ""))
	if len(text) > command.MaxBytes {
		text = text[:command.MaxBytes]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
		truncated = true
	}
	result.Facts["excerpt"] = observation.Text(text)
	result.Facts["tail_lines"] = observation.Number(int64(command.TailLines))
	result.Facts["previous"] = observation.Boolean(command.Previous)
	result.Coverage.Observed = len(strings.Split(strings.TrimSuffix(text, "\n"), "\n"))
	result.Partial("selected log excerpt, not complete log history")
	if truncated {
		result.Coverage.Limitations = append(result.Coverage.Limitations, "log excerpt reached its line or byte limit")
	}
	return result, nil
}

func encodedSize(value any) int {
	body, _ := json.Marshal(value)
	return len(body)
}

func (e *Projector) Namespace() string { return e.scope.Namespace }
