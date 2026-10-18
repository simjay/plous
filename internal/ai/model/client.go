package model

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	openaisdk "github.com/meguminnnnnnnnn/go-openai"
)

const MaxResponseBytes = 1 << 20
const MaxOutputTokens = 4096

// NewChatModel captures one explicit connection for the entire operation.
func NewChatModel(ctx context.Context, c Config, key string) (model.ToolCallingChatModel, error) {
	if err := c.Ready(key); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 3 * time.Minute, Transport: credentialTransport{base: http.DefaultTransport, key: key, provider: c.Provider}, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("redirects are disabled; configure the final API endpoint")
	}}
	var m model.ToolCallingChatModel
	var err error
	switch c.Provider {
	case "openai":
		limit := MaxOutputTokens
		m, err = openai.NewChatModel(ctx, &openai.ChatModelConfig{APIKey: key, BaseURL: c.BaseURL(), Model: c.Model, HTTPClient: client, MaxCompletionTokens: &limit})
	case "anthropic":
		base := c.BaseURL()
		// The SDK checks authentication before transport, even for an unauthenticated
		// local proxy. Transport removes this placeholder and sets only the captured key.
		sdkKey := key
		if sdkKey == "" {
			sdkKey = "plous-unauthenticated-proxy"
		}
		m, err = claude.NewChatModel(ctx, &claude.Config{APIKey: sdkKey, BaseURL: &base, Model: c.Model, MaxTokens: MaxOutputTokens, HTTPClient: client})
	default:
		return nil, errors.New("unsupported AI provider")
	}
	if err != nil {
		return nil, SafeError(ctx, err)
	}
	return &safeModel{base: m, provider: c.Provider, gateStream: c.Provider == "anthropic"}, nil
}

type safeModel struct {
	base       model.ToolCallingChatModel
	provider   string
	gateStream bool
}

func (m *safeModel) WithTools(t []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	bound, err := m.base.WithTools(t)
	if err != nil {
		return nil, SafeError(context.Background(), err)
	}
	return &safeModel{base: bound, provider: m.provider, gateStream: m.gateStream}, nil
}
func (m *safeModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	out, err := m.base.Generate(ctx, input, m.callOptions(opts)...)
	if err != nil {
		return nil, SafeError(ctx, err)
	}
	return out, nil
}
func (m *safeModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if m.gateStream {
		ready := make(chan struct{})
		ctx = context.WithValue(ctx, streamReadyKey{}, (<-chan struct{})(ready))
		defer close(ready)
	}
	in, err := m.base.Stream(ctx, input, m.callOptions(opts)...)
	if err != nil {
		return nil, SafeError(ctx, err)
	}
	return schema.StreamReaderWithConvert(in, func(msg *schema.Message) (*schema.Message, error) { return msg, nil }, schema.WithErrWrapper(func(err error) error { return SafeError(ctx, err) })), nil
}

func (m *safeModel) callOptions(opts []model.Option) []model.Option {
	if m.provider != "openai" {
		return opts
	}
	limit := model.GetCommonOptions(nil, opts...).MaxTokens
	if limit == nil {
		return opts
	}
	translated := make([]model.Option, 0, len(opts)+2)
	translated = append(translated, opts...)
	// Eino serializes the common limit as legacy max_tokens. Zero omits that field.
	return append(translated, model.WithMaxTokens(0), openai.WithMaxCompletionTokens(*limit))
}

func CompletedFinish(reason string) bool { return reason == "stop" || reason == "end_turn" }

func SafeError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var local UserError
	if errors.As(err, &local) {
		return local
	}
	status := 0
	var ae *anthropicsdk.Error
	if errors.As(err, &ae) {
		status = ae.StatusCode
	}
	var oe *openaisdk.APIError
	if errors.As(err, &oe) {
		status = oe.HTTPStatusCode
	}
	var adapterError *openai.APIError
	if errors.As(err, &adapterError) {
		status = adapterError.HTTPStatusCode
	}
	var re *openaisdk.RequestError
	if errors.As(err, &re) {
		status = re.HTTPStatusCode
	}
	if status == 403 || status == 404 {
		return UserError(fmt.Sprintf("provider returned HTTP %d; check account access to this model and the API endpoint; use /model to choose another model", status))
	}
	if status != 0 {
		return UserError(fmt.Sprintf("provider returned HTTP %d; check the model, endpoint, credentials, and quota", status))
	}
	return UserError("AI connection failed; check Settings (:s) and network access")
}

// The provider SDK must not inherit unrelated keys or automatically repeat a request.
type credentialTransport struct {
	base          http.RoundTripper
	key, provider string
}

func (t credentialTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c := r.Clone(r.Context())
	c.Header = r.Header.Clone()
	for _, h := range []string{"Authorization", "X-Api-Key", "OpenAI-Organization", "OpenAI-Project"} {
		c.Header.Del(h)
	}
	if t.key != "" {
		if t.provider == "anthropic" {
			c.Header.Set("X-Api-Key", t.key)
		} else {
			c.Header.Set("Authorization", "Bearer "+t.key)
		}
	}
	if retry := c.Header.Get("X-Stainless-Retry-Count"); retry != "" && retry != "0" {
		return failedResponse(c), nil
	}
	res, err := t.base.RoundTrip(c)
	if err != nil {
		if r.Context().Err() != nil {
			return nil, r.Context().Err()
		}
		return failedResponse(c), nil
	}
	res.Header.Set("X-Should-Retry", "false")
	if ready, ok := r.Context().Value(streamReadyKey{}).(<-chan struct{}); ok && res.StatusCode >= 200 && res.StatusCode < 300 {
		res.Body = &streamReadyBody{ReadCloser: res.Body, ready: ready, ctx: r.Context()}
	}
	return res, nil
}

// Claude adapter v0.1.26 writes its named error return from the reader goroutine.
// Start SSE reads only after Stream returns, establishing ordering without
// buffering output or forking the adapter. Retain the provider race regressions.
// Any future streaming callbacks must consume asynchronously, after initialization.
type streamReadyKey struct{}
type streamReadyBody struct {
	io.ReadCloser
	ready <-chan struct{}
	ctx   context.Context
}

func (b *streamReadyBody) Read(p []byte) (int, error) {
	select {
	case <-b.ready:
		return b.ReadCloser.Read(p)
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
}

func failedResponse(r *http.Request) *http.Response {
	return &http.Response{StatusCode: 502, Status: "502 Bad Gateway", Header: http.Header{"X-Should-Retry": []string{"false"}, "Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"connection_error","message":"connection failed"}}`)), Request: r}
}

// UserError is an application-authored message that is safe to show without provider details.
type UserError string

func (e UserError) Error() string { return string(e) }
