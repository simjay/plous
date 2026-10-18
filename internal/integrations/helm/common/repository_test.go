package common

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSDKChartDownloadsCancelAndBoundResponses(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/large" {
			w.Write([]byte(strings.Repeat("x", MaxOutput+1)))
			return
		}
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := pullHTTPChart(ctx, RepositoryConfig{}, server.URL+"/slow", "", ""); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("download ignored cancellation")
	}
	if _, err := pullHTTPChart(context.Background(), RepositoryConfig{}, server.URL+"/large", "", ""); !errors.Is(err, ErrOutputLimit) {
		t.Fatal("unbounded download", err)
	}
}
