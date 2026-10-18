package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddingWireAndFailureBoundary(t *testing.T) {
	var response = `{"data":[{"index":1,"embedding":[0,1]},{"index":0,"embedding":[1,0]}],"usage":{"total_tokens":7}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/embeddings" || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("embedding request changed")
		}
		var request struct {
			Model    string
			Input    []string
			Encoding string `json:"encoding_format"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Model != "fixture-embedding" || request.Encoding != "float" || len(request.Input) != 2 {
			t.Error("embedding model or input changed")
		}
		w.Write([]byte(response))
	}))
	defer server.Close()
	c := Config{Name: "fixture", Provider: "openai", Model: "gpt-4.1", Endpoint: server.URL}
	got, tokens, err := Embed(t.Context(), c, "fixture-key", "fixture-embedding", []string{"one", "two"})
	if err != nil || tokens != 7 || len(got) != 2 || got[0][0] != 1 || got[1][1] != 1 {
		t.Fatalf("%v %d %v", got, tokens, err)
	}
	response = `{"data":[{"index":0,"embedding":[1]},{"index":0,"embedding":[2]}]}`
	if _, _, err = Embed(t.Context(), c, "fixture-key", "fixture-embedding", []string{"one", "two"}); err == nil {
		t.Fatal("duplicate embedding index accepted")
	}
	response = `fixture-key private provider response`
	if _, _, err = Embed(t.Context(), c, "fixture-key", "fixture-embedding", []string{"one", "two"}); err == nil || strings.Contains(err.Error(), "fixture-key") {
		t.Fatal("provider response escaped boundary", err)
	}
}
