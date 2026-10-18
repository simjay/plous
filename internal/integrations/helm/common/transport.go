package common

import (
	"context"
	"io"
	"net/http"
)

type OperationTransport struct {
	Context context.Context
	Base    http.RoundTripper
}

func (t OperationTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := t.Context.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(t.Context, cancel)
	response, err := t.Base.RoundTrip(r.Clone(ctx))
	if err != nil {
		stop()
		cancel()
		return nil, err
	}
	response.Body = &operationBody{ReadCloser: response.Body, close: func() { stop(); cancel() }}
	return response, nil
}

type operationBody struct {
	io.ReadCloser
	close func()
}

func (b *operationBody) Close() error { defer b.close(); return b.ReadCloser.Close() }
