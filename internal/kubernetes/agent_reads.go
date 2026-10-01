package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/rest"
)

const maxAgentResponseBytes = 1024 * 1024

type ReadResult struct {
	Source    CommandScope `json:"source"`
	Command   Command      `json:"command"`
	FetchedAt time.Time    `json:"fetched_at"`
	Data      any          `json:"data,omitempty"`
	Truncated bool         `json:"truncated"`
	Continue  string       `json:"continue,omitempty"`
	Notice    string       `json:"notice,omitempty"`
}

// ReadExecutor is the only Kubernetes capability exposed to assistant tools.
// It keeps the transport and captured scope private and validates every call.
type ReadExecutor struct {
	client      rest.Interface
	scope       CommandScope
	credentials []string
}

func NewReadExecutor(client *Client, scope CommandScope) (*ReadExecutor, error) {
	if client == nil || client.rest == nil || client.config == nil {
		return nil, errors.New("Kubernetes read client is unavailable")
	}
	if scope.Namespace == "" {
		scope.Namespace = client.Namespace
	}
	if err := ValidateRead(Command{Operation: "get", Resource: "pods"}, scope); err != nil {
		return nil, err
	}
	config := rest.CopyConfig(client.config)
	config.WarningHandlerWithContext = rest.NoWarnings{}
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, errors.New("cannot initialize scoped Kubernetes reads")
	}
	// Copy the HTTP client because client-go can return http.DefaultClient.
	// Redirects must not turn a validated endpoint into a different operation.
	guarded := *httpClient
	guarded.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("Kubernetes read redirects are unavailable")
	}
	raw, err := rest.UnversionedRESTClientForConfigAndClient(config, &guarded)
	if err != nil {
		return nil, errors.New("cannot initialize scoped Kubernetes reads")
	}
	return &ReadExecutor{
		client: raw, scope: scope,
		credentials: []string{config.BearerToken, config.Password, string(config.KeyData), string(config.CertData)},
	}, nil
}

func (e *ReadExecutor) ExecuteJSON(ctx context.Context, input string) (ReadResult, error) {
	command, err := DecodeCommand(input)
	if err != nil {
		return ReadResult{}, err
	}
	return e.Execute(ctx, command)
}

func (e *ReadExecutor) Execute(ctx context.Context, command Command) (ReadResult, error) {
	if err := ValidateRead(command, e.scope); err != nil {
		return ReadResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	command.Namespace = e.scope.Namespace
	if command.MaxBytes == 0 {
		command.MaxBytes = MaxAgentBytes
	}
	result := ReadResult{Source: e.scope, Command: command, FetchedAt: time.Now().UTC()}
	if command.Operation == "logs" {
		return e.logs(ctx, command, result)
	}
	resource, err := e.discoverResource(ctx, command.Resource)
	if err != nil {
		return result, err
	}
	verb := "get"
	if command.Name == "" {
		verb = "list"
	}
	if !resource.Supports(verb, "") {
		return result, errors.New("Kubernetes resource does not support the requested read")
	}
	if !resource.Namespaced {
		command.Namespace = ""
		result.Command = command
	}
	path := resourcePath(resource, command.Namespace)
	if command.Name != "" {
		path += "/" + command.Name
	}
	request := e.client.Get().AbsPath(path)
	if command.Name == "" {
		if command.Limit == 0 {
			command.Limit = 50
		}
		result.Command = command
		request.Param("limit", fmt.Sprint(command.Limit))
		request.Param("labelSelector", command.LabelSelector)
		request.Param("fieldSelector", command.FieldSelector)
		request.Param("continue", command.Continue)
	}
	var object map[string]any
	if err := readAgentJSON(ctx, request, maxAgentResponseBytes, &object); err != nil {
		return result, err
	}
	object = e.sanitize(object).(map[string]any)
	if command.Name == "" {
		items, ok := object["items"].([]any)
		if !ok {
			return result, errors.New("Kubernetes returned an invalid resource list")
		}
		if metadata, ok := object["metadata"].(map[string]any); ok {
			result.Continue, _ = metadata["continue"].(string)
		}
		if len(result.Continue) > 4096 {
			return result, errors.New("Kubernetes returned an oversized continuation token")
		}
		result.Truncated = result.Continue != ""
		if len(items) > command.Limit {
			items = items[:command.Limit]
			object["items"] = items
			result.Truncated = true
			result.Notice = "The server exceeded the requested item limit. Repeat with a narrower selector."
			result.Continue = ""
		}
		for len(items) > 0 && encodedSize(object) > command.MaxBytes {
			items = items[:len(items)-1]
			object["items"] = items
			result.Truncated = true
			result.Continue = ""
			result.Notice = "The page exceeded the output limit. Repeat with a smaller item limit or narrower selector."
		}
		if metadata, ok := object["metadata"].(map[string]any); ok && result.Continue == "" {
			delete(metadata, "continue")
		}
	}
	if encodedSize(object) > command.MaxBytes {
		result.Truncated = true
		result.Notice = "The resource exceeded the output limit. Its body was omitted."
	} else {
		result.Data = object
	}
	return result, nil
}

// Discovery is limited to the requested API group rather than the full cluster.
func (e *ReadExecutor) discoverResource(ctx context.Context, identity string) (Resource, error) {
	name, group, _ := strings.Cut(identity, ".")
	version := "v1"
	path := "/api/v1"
	if group != "" {
		var apiGroup metav1.APIGroup
		if err := readAgentJSON(ctx, e.client.Get().AbsPath("/apis/"+group), 128*1024, &apiGroup); err != nil {
			return Resource{}, err
		}
		gv, err := schema.ParseGroupVersion(apiGroup.PreferredVersion.GroupVersion)
		if err != nil || gv.Group != group || len(validation.IsDNS1123Label(gv.Version)) != 0 {
			return Resource{}, errors.New("Kubernetes returned invalid API discovery")
		}
		version = gv.Version
		path = "/apis/" + group + "/" + version
	}
	var list metav1.APIResourceList
	if err := readAgentJSON(ctx, e.client.Get().AbsPath(path), 256*1024, &list); err != nil {
		return Resource{}, err
	}
	for _, item := range list.APIResources {
		if item.Name == name {
			return Resource{Group: group, Version: version, Name: name, Kind: item.Kind, Namespaced: item.Namespaced, Verbs: append([]string(nil), item.Verbs...)}, nil
		}
	}
	return Resource{}, errors.New("resource identity was not found in Kubernetes API discovery")
}

func readAgentJSON(ctx context.Context, request *rest.Request, limit int64, target any) error {
	stream, err := request.MaxRetries(0).Stream(ctx)
	if err != nil {
		return errors.New(ErrorText(err))
	}
	defer stream.Close()
	body, err := io.ReadAll(io.LimitReader(stream, limit+1))
	if err != nil {
		return errors.New(ErrorText(err))
	}
	if int64(len(body)) > limit {
		return errors.New("Kubernetes response exceeded the read limit; request fewer resources")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return errors.New("Kubernetes returned an invalid JSON response")
	}
	return nil
}

func (e *ReadExecutor) logs(ctx context.Context, command Command, result ReadResult) (ReadResult, error) {
	if command.TailLines == 0 {
		command.TailLines = 200
	}
	result.Command = command
	request := e.client.Get().AbsPath("/api/v1/namespaces/"+command.Namespace+"/pods/"+command.Name+"/log").
		Param("container", command.Container).Param("tailLines", fmt.Sprint(command.TailLines)).
		Param("limitBytes", fmt.Sprint(command.MaxBytes)).Param("timestamps", "true").
		Param("previous", fmt.Sprint(command.Previous)).Param("follow", "false")
	if command.SinceSeconds != 0 {
		request.Param("sinceSeconds", fmt.Sprint(command.SinceSeconds))
	}
	stream, err := request.MaxRetries(0).Stream(ctx)
	if err != nil {
		return result, errors.New(ErrorText(err))
	}
	defer stream.Close()
	body, err := io.ReadAll(io.LimitReader(stream, int64(command.MaxBytes)+1))
	if err != nil {
		return result, errors.New(ErrorText(err))
	}
	result.Truncated = len(body) >= command.MaxBytes
	if len(body) > command.MaxBytes {
		body = body[:command.MaxBytes]
	}
	lines := strings.SplitAfter(string(body), "\n")
	if len(lines) > command.TailLines {
		body = []byte(strings.Join(lines[:command.TailLines], ""))
		result.Truncated = true
	}
	text := e.sanitizeText(strings.ToValidUTF8(string(body), ""))
	if len(text) > command.MaxBytes {
		text = text[:command.MaxBytes]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
		result.Truncated = true
	}
	result.Data = text
	if result.Truncated {
		result.Notice = "Log excerpt reached its line or byte limit."
	}
	return result, nil
}

func encodedSize(value any) int {
	body, _ := json.Marshal(value)
	return len(body)
}

var credentialAssignment = regexp.MustCompile(`(?i)((?:password|passwd|token|secret|api[_-]?key|authorization|credential|client[_-]?key|access[_-]?key)["']?\s*[=:]\s*)(?:"[^"\n]*"|'[^'\n]*'|[^\s,;]+)`)
var authorizationCredential = regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[^\s"',;]+`)
var jwtCredential = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+\b`)
var privateKeyMaterial = regexp.MustCompile(`(?s)-----BEGIN [^-]*PRIVATE KEY-----.*?(?:-----END [^-]*PRIVATE KEY-----|$)`)
var credentialURL = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^\s/@]+:[^\s/@]+@`)

func (e *ReadExecutor) sanitizeText(value string) string {
	for _, credential := range e.credentials {
		if credential != "" {
			value = strings.ReplaceAll(value, credential, "[redacted]")
		}
	}
	value = privateKeyMaterial.ReplaceAllString(value, "[redacted private key]")
	value = authorizationCredential.ReplaceAllString(value, "${1} [redacted]")
	value = jwtCredential.ReplaceAllString(value, "[redacted token]")
	value = credentialAssignment.ReplaceAllString(value, `${1}"[redacted]"`)
	return credentialURL.ReplaceAllString(value, "${1}[redacted]@")
}

func (e *ReadExecutor) sanitize(value any) any {
	switch item := value.(type) {
	case map[string]any:
		if kind, _ := item["kind"].(string); strings.EqualFold(kind, "Secret") {
			return map[string]any{"kind": "Secret", "redacted": true}
		}
		result := make(map[string]any, len(item))
		for key, child := range item {
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
			switch normalized {
			case "data", "binarydata", "stringdata", "annotations", "managedfields", "env", "command", "args", "userdata":
				result[key] = "[redacted]"
				continue
			}
			if strings.Contains(normalized, "password") || strings.Contains(normalized, "credential") || strings.Contains(normalized, "privatekey") || normalized == "token" || normalized == "accesstoken" || normalized == "refreshtoken" || normalized == "apikey" || normalized == "accesskey" || normalized == "clientkey" || normalized == "clientcertificate" || normalized == "authorization" || normalized == "secret" {
				result[key] = "[redacted]"
				continue
			}
			result[key] = e.sanitize(child)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for i, child := range item {
			result[i] = e.sanitize(child)
		}
		return result
	case string:
		return e.sanitizeText(item)
	default:
		return value
	}
}
