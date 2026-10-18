package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	journal "github.com/simjay/plous/internal/ai/history"
	aimodel "github.com/simjay/plous/internal/ai/model"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/integrations/observation"
	"github.com/simjay/plous/internal/workspace"
	"k8s.io/client-go/rest"
)

func deliveryFixture(t *testing.T, handler http.HandlerFunc) *operation {
	t.Helper()
	o, _ := workspaceFixture(t)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	projector, err := kubernetes.NewProjector(client, kubernetes.CommandScope{ConnectionID: "east", Context: "fixture", Namespace: "payments"})
	if err != nil {
		t.Fatal(err)
	}
	o.Connections = []Connection{{ConnectionID: "east", Tool: "kubernetes", Namespace: "payments", Context: "fixture"}}
	op := &operation{o: o, runtime: DefaultRuntimeConfig(), deliveryBytes: 65536, kube: map[string]*kubernetes.Projector{"east\x00payments": projector}}
	if err := op.setupHistory(); err != nil {
		t.Fatal(err)
	}
	return op
}
func deliveryDiscovery(w http.ResponseWriter) {
	fmt.Fprint(w, `{"apiVersion":"v1","kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"pods","kind":"Pod","namespaced":true,"verbs":["get","list"]}]}`)
}
func deliveryPods(w http.ResponseWriter, count int, next string) {
	rows := []any{}
	for i := range count {
		rows = append(rows, map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": fmt.Sprintf("pod-%d", i), "namespace": "payments", "uid": fmt.Sprintf("uid-%d", i)}, "status": map[string]any{"phase": "Running"}})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": "PodList", "metadata": map[string]any{"resourceVersion": "1", "continue": next}, "items": rows})
}
func deliveryRead() aimodel.ToolCall {
	return aimodel.ToolCall{ID: "live", Name: "kubernetes_resources_read", Args: json.RawMessage(`{"purpose":"List names","connection_id":"east","operation":"get","resource":"pods","fields":[]}`)}
}

func TestDeliveryUsesSameSavedPageForMainAndHistory(t *testing.T) {
	var requests atomic.Int32
	op := deliveryFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/api/v1" {
			deliveryDiscovery(w)
			return
		}
		deliveryPods(w, 109, "")
	})
	body, err := op.toolResult(t.Context(), deliveryRead())
	if err != nil {
		t.Fatal(err)
	}
	var live observation.Delivery
	if err = json.Unmarshal([]byte(body), &live); err != nil || live.Page == nil || live.Page.Returned != 109 || live.Page.NextOffset != nil {
		t.Fatal(body, err)
	}
	for _, row := range live.Observation.Rows {
		if len(row.Facts) != 0 {
			t.Fatal("name list included row facts")
		}
	}
	reads := requests.Load()
	// Saved pagination and calculations must not refetch live state.
	out, _, err := op.historyTool(t.Context(), marshal(map[string]any{"operation": "read", "ref": live.Ref, "fields": []string{}}))
	if err != nil {
		t.Fatal(err)
	}
	got := out.(journal.HistoryReply).Evidence[0].Delivery
	if string(marshal(got)) != string(marshal(live)) || requests.Load() != reads {
		t.Fatal("history changed delivery or fetched new API data")
	}
	sources, err := op.o.History.TurnEvidence(t.Context(), op.o.WorkspaceID, op.o.ConversationID, op.o.TurnID)
	if err != nil || len(sources[0].Projection.Rows[0].Facts) == 0 {
		t.Fatal("field selection changed saved evidence", err)
	}
	op.deliveryBytes = 2400
	returned, offset := 0, 0
	for {
		out, _, err := op.historyTool(t.Context(), marshal(map[string]any{"operation": "read", "ref": live.Ref, "offset": offset, "fields": []string{}}))
		if err != nil {
			t.Fatal(err)
		}
		if len(marshal(out)) > op.deliveryBytes {
			t.Fatal("history envelope exceeded its allowance")
		}
		page := out.(journal.HistoryReply).Evidence[0].Page
		returned += page.Returned
		if page.NextOffset == nil {
			break
		}
		offset = *page.NextOffset
	}
	if returned != 109 || requests.Load() != reads {
		t.Fatal("saved pagination lost rows or refetched the API")
	}
	op.deliveryBytes = 65536

}

func TestInterruptedProjectionSurvivesRestart(t *testing.T) {
	for _, kind := range []string{"denied", "partial", "cancelled", "invalid"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			op := deliveryFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1" {
					deliveryDiscovery(w)
					return
				}
				if kind != "denied" && r.URL.Query().Get("continue") == "" {
					deliveryPods(w, 50, "next")
					return
				}
				if kind == "cancelled" {
					cancel()
					<-r.Context().Done()
					return
				}
				if kind == "invalid" {
					fmt.Fprint(w, `{"items":"invalid private fixture text"}`)
					return
				}
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403,"message":"private fixture credential"}`)
			})
			_, err := op.toolResult(ctx, deliveryRead())
			if kind == "cancelled" && !errors.Is(err, context.Canceled) || kind == "invalid" && err == nil || (kind == "denied" || kind == "partial") && err != nil {
				t.Fatal(kind, err)
			}
			root, err := op.o.Store.DataRoot()
			if err != nil {
				t.Fatal(err)
			}
			if err = op.o.Store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err := workspace.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			h, err := journal.Open(store)
			if err != nil {
				t.Fatal(err)
			}
			sources, err := h.TurnEvidence(t.Context(), op.o.WorkspaceID, op.o.ConversationID, op.o.TurnID)
			if err != nil || len(sources) != 1 {
				t.Fatal(sources, err)
			}
			saved := sources[0].Projection
			if saved == nil || saved.Coverage.Complete || saved.Coverage.Total != nil || saved.Connection != "east" || saved.Namespace != "payments" {
				t.Fatal(saved)
			}
			if kind == "denied" {
				if saved.Outcome != "denied" || !strings.Contains(saved.Facts["error"].String(), "RBAC") {
					t.Fatal(saved)
				}
			} else if saved.Outcome != "partial" || len(saved.Rows) != 50 || saved.Facts["observed_total"].String() != "50" || !saved.Facts["total"].Unknown {
				t.Fatal(saved)
			}
			if strings.Contains(string(marshal(saved)), "private fixture") {
				t.Fatal("raw error body entered saved evidence")
			}
		})
	}
}
