package plousbench

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
)

// Fixture contains public input data only. Expected results live in tools.json.
type Fixture struct {
	NextRoutes []Route           `json:"next_routes,omitempty"`
	Scope      string            `json:"scope"`
	Routes     []Route           `json:"routes,omitempty"`
	Files      map[string]string `json:"files,omitempty"`
}

type Route struct {
	Path   string            `json:"path"`
	Query  map[string]string `json:"query,omitempty"`
	Status int               `json:"status,omitempty"`
	Body   json.RawMessage   `json:"body,omitempty"`
	Text   *string           `json:"text,omitempty"`
}

type Access struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Query   string `json:"query"`
	Status  int    `json:"status"`
	Matched bool   `json:"matched"`
}

type Engine struct {
	mu     sync.Mutex
	routes []Route
	access []Access
}

func LoadFixture(path string) (Fixture, error) {
	var f Fixture
	if err := readJSON(path, &f); err != nil {
		return f, err
	}
	if f.Scope != "cluster" && f.Scope != "source" && f.Scope != "general" {
		return f, fmt.Errorf("unknown fixture scope %q", f.Scope)
	}
	for path := range f.Files {
		if !filepath.IsLocal(path) {
			return f, fmt.Errorf("fixture path escapes its root: %s", path)
		}
	}
	for _, r := range append(append([]Route{}, f.Routes...), f.NextRoutes...) {
		if r.Path == "" || r.Text == nil && !json.Valid(r.Body) || r.Status != 0 && (r.Status < 200 || r.Status > 599) {
			return f, fmt.Errorf("invalid fixture route %s", r.Path)
		}
	}
	return f, nil
}

func (e *Engine) Advance(routes []Route) {
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = json.Unmarshal(JSON(routes), &e.routes)
}

func (e *Engine) Accesses() []Access {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Access(nil), e.access...)
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	status, matched := http.StatusNotFound, false
	defer func() { e.access = append(e.access, Access{r.Method, r.URL.Path, r.URL.RawQuery, status, matched}) }()
	if r.Method != http.MethodGet {
		status = http.StatusForbidden
		http.Error(w, "fixture is read-only", status)
		return
	}
	for _, route := range e.routes {
		if r.URL.Path != route.Path {
			continue
		}
		matches := true
		for key, value := range route.Query {
			if r.URL.Query().Get(key) != value {
				matches = false
			}
		}
		if !matches {
			continue
		}
		status, matched = route.Status, true
		if status == 0 {
			status = http.StatusOK
		}
		if route.Text != nil {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(status)
			fmt.Fprint(w, *route.Text)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write(route.Body)
		}
		return
	}
	http.Error(w, "request has no fixture", status)
}

// Serve creates a loopback server and a kubeconfig with no external credentials.
func Serve(root string, routes []Route) (*Engine, *httptest.Server, string, error) {
	e := &Engine{}
	_ = json.Unmarshal(JSON(routes), &e.routes)
	server := httptest.NewServer(e)
	config := map[string]any{"apiVersion": "v1", "kind": "Config", "current-context": "fixture", "clusters": []any{map[string]any{"name": "fixture", "cluster": map[string]any{"server": server.URL + "/cluster"}}}, "contexts": []any{map[string]any{"name": "fixture", "context": map[string]any{"cluster": "fixture", "user": "fixture", "namespace": "apps"}}}, "users": []any{map[string]any{"name": "fixture", "user": map[string]any{}}}}
	path := filepath.Join(root, "kubeconfig.json")
	if err := os.WriteFile(path, JSON(config), 0600); err != nil {
		server.Close()
		return nil, nil, "", err
	}
	return e, server, path, nil
}
