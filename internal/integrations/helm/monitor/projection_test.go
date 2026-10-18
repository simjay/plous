package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/simjay/plous/internal/integrations/observation"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestConfigurationProjectionPreservesExactNumbersAndUnknownValues(t *testing.T) {
	r := observation.New("helm.release.v1", "values", time.Now().UTC())
	projectConfiguration(&r, map[string]any{"sequence": json.Number("9007199254740993"), "large": strings.Repeat("x", 1025), "fraction": json.Number("0.125")})
	got := map[string]observation.Value{}
	for _, row := range r.Rows {
		got[row.Identity.Name] = row.Facts["value"]
	}
	if got["/sequence"].String() != "9007199254740993" || got["/fraction"].String() != "0.125" || !got["/large"].Unknown {
		t.Fatal(got)
	}
}

func TestReleaseProjectionPreservesCompletedPagesOnFailure(t *testing.T) {
	for _, failure := range []string{"denied", "cancelled", "invalid"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			pages := 0
			client := &Client{Config: fixtureConfig(), Runner: runnerFunc(func(_ context.Context, _ Config, args []string, _ string) ([]byte, error) {
				if args[0] == "get" {
					return json.Marshal(Release{Name: args[2], Namespace: "apps", Revision: 1, Status: "deployed"})
				}
				pages++
				if pages == 1 {
					var rows []map[string]string
					for i := range 100 {
						rows = append(rows, map[string]string{"name": fmt.Sprintf("release-%d", i), "namespace": "apps", "revision": "1", "status": "deployed"})
					}
					return json.Marshal(rows)
				}
				switch failure {
				case "denied":
					return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("private server details"))
				case "cancelled":
					cancel()
					return nil, ctx.Err()
				default:
					return []byte(`{"malformed":"private server details"}`), nil
				}
			})}
			projector, err := NewProjector(client, "apps")
			if err != nil {
				t.Fatal(err)
			}
			r, err := projector.Execute(ctx, Command{Operation: "list"})
			if (err != nil) != (failure != "denied") || failure == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("wrong failure propagation", err)
			}
			if len(r.Rows) != 100 || r.Coverage.Complete || r.Coverage.Total != nil || r.Outcome != "partial" || strings.Contains(r.SearchText(), "private server details") {
				t.Fatal("partial inventory lost or misrepresented", r)
			}
		})
	}
}

func TestHelmReadErrorsStayDetailedOutsideProjection(t *testing.T) {
	failure := apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("private server details"))
	client := &Client{Config: fixtureConfig(), Runner: runnerFunc(func(context.Context, Config, []string, string) ([]byte, error) {
		return nil, failure
	})}
	_, err := client.History(t.Context(), "shop", "apps")
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "private server details") {
		t.Fatal("client lost original error details", err)
	}
	projector, err := NewProjector(client, "apps")
	if err != nil {
		t.Fatal(err)
	}
	r, err := projector.Execute(t.Context(), Command{Operation: "history", Name: "shop"})
	if err != nil || r.Outcome != "denied" || !strings.Contains(r.SearchText(), "RBAC") || strings.Contains(r.SearchText(), "private server details") {
		t.Fatal("projected error is not specific and safe", r, err)
	}
}

func TestProjectionComparesExplicitRetainedRevisions(t *testing.T) {
	for _, pair := range [][2]int{{1, 3}, {3, 1}} {
		t.Run(fmt.Sprint(pair), func(t *testing.T) {
			var reads []string
			client := &Client{Config: fixtureConfig(), Runner: runnerFunc(func(_ context.Context, _ Config, args []string, _ string) ([]byte, error) {
				if len(args) < 6 || args[0] != "status" || args[1] != "shop" || args[2] != "--revision" {
					t.Fatalf("unexpected acquisition %v", args)
				}
				reads = append(reads, args[3])
				return []byte(fmt.Sprintf(`{"name":"shop","namespace":"apps","version":%s,"chart":{"metadata":{"name":"shop","version":"1.0.0"}},"config":{"replicas":%s,"password":"private-%s"}}`, args[3], args[3], args[3])), nil
			})}
			projector, err := NewProjector(client, "apps")
			if err != nil {
				t.Fatal(err)
			}
			r, err := projector.Execute(t.Context(), Command{Operation: "compare", Name: "shop", Revision: pair[1], CompareRevision: &pair[0]})
			if err != nil || !r.Coverage.Complete || fmt.Sprint(reads) != fmt.Sprintf("[%d %d]", pair[1], pair[0]) {
				t.Fatal(r, err, reads)
			}
			if r.Facts["before_revision"].String() != fmt.Sprint(pair[0]) || r.Facts["revision"].String() != fmt.Sprint(pair[1]) {
				t.Fatal(r)
			}
			found := false
			for _, row := range r.Rows {
				if row.Identity.Name == "/suppliedValues/replicas" {
					found = true
					if row.Facts["before"].String() != fmt.Sprint(pair[0]) || row.Facts["after"].String() != fmt.Sprint(pair[1]) {
						t.Fatal(row)
					}
				}
			}
			if !found || strings.Contains(r.SearchText(), "private-") {
				t.Fatal("comparison lost values or exposed a secret", r)
			}
		})
	}
	for _, input := range []string{`{"operation":"compare","name":"shop","compare_revision":1}`, `{"operation":"compare","name":"shop","revision":1,"compare_revision":1}`, `{"operation":"compare","name":"shop","revision":3,"compare_revision":0}`, `{"operation":"values","name":"shop","revision":3,"compare_revision":1}`} {
		c, err := DecodeCommand(input)
		if err == nil {
			err = ValidateRead(c, fixtureConfig())
		}
		if err == nil {
			t.Fatal("invalid comparison accepted", input)
		}
	}
}
