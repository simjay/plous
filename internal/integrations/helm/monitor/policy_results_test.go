package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
)

func TestProjectionDoesNotPersistLargeOriginalValues(t *testing.T) {
	want := strings.Repeat("large permitted value ", 5000)
	c := &Client{Config: fixtureConfig(), Runner: runnerFunc(func(_ context.Context, _ Config, args []string, _ string) ([]byte, error) {
		switch args[0] {
		case "status":
			return []byte(`{"name":"shop","namespace":"apps","version":1,"chart":{"metadata":{"name":"shop","version":"1.0.0"}},"info":{"status":"deployed"}}`), nil
		case "get":
			return json.Marshal(map[string]string{"description": want, "password": "must-not-survive"})
		case "history":
			return []byte(`[]`), nil
		default:
			return nil, errors.New("unexpected command")
		}
	})}
	executor, err := NewProjector(c, "apps")
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(t.Context(), Command{Operation: "values", Name: "shop", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	body := string(manifestutil.JSON(result))
	if strings.Contains(body, want) || strings.Contains(body, "must-not-survive") || strings.Contains(body, "exceeded 64 KiB") {
		t.Fatal("raw result or credential was retained")
	}
}
