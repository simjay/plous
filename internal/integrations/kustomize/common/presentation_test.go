package common

import (
	"strings"
	"testing"
)

func TestRenderFailureExplainsPatchIdentityWithoutInventingFix(t *testing.T) {
	raw := `no resource matches strategic merge patch "Deployment.v1.apps/api-v1.[noNs]": no matches for Id Deployment.v1.apps/api-v1.[noNs]; failed to find unique target`
	got := RenderFailure(raw)
	if got != "A patch targets Deployment api-v1, but no matching resource was found." {
		t.Fatal(got)
	}
	if RenderFailure("arbitrary private failure") != "" {
		t.Fatal("invented explanation")
	}
	if got := RenderFailure(strings.ReplaceAll(raw, "api-v1", "api.v1")); !strings.Contains(got, "Deployment api.v1,") {
		t.Fatal("truncated resource name", got)
	}
}
func TestChangeDescriptionsDoNotExposeInlineValues(t *testing.T) {
	e := Entry{Configuration: map[string]any{"namespace": "production-eu", "patches": []any{map[string]any{"patch": "secret content"}}, "secretGenerator": []any{map[string]any{"literals": []any{"private=value"}}}}}
	got := strings.Join(e.ChangeDescriptions(), "\n")
	if !strings.Contains(got, "Sets namespace to production-eu") || !strings.Contains(got, "1 patch") || strings.Contains(got, "secret content") || strings.Contains(got, "private=value") {
		t.Fatal(got)
	}
}
