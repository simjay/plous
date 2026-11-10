package cli

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCancellationBeforeDispatchDoesNotRunTool(t *testing.T) {
	tools := &fixtureTools{}
	b, err := OpenBridge(t.Context(), tools, "session")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	request := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", b.URL, strings.NewReader(body))
		r.Header.Set("Authorization", b.Authorization())
		w := httptest.NewRecorder()
		b.ServeHTTP(w, r)
		return w
	}
	request(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"pending"}}`)
	response := request(`{"jsonrpc":"2.0","id":"pending","method":"tools/call","params":{"name":"history_read","arguments":{}}}`)
	if tools.calls.Load() != 0 || !strings.Contains(response.Body.String(), `"error"`) {
		t.Fatal("cancelled request reached tool execution", response.Body.String(), tools.calls.Load())
	}
}
