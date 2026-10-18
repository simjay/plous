package integration

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	agent "github.com/simjay/plous/internal/ai"
)

func TestProjectedPaginationPreservesPartialCoverage(t *testing.T) {
	o, history := workspaceFixture(t)
	nativeOptions(t, &o)
	connection, requests := scopedKubernetesFixture(t)
	o.Connections = []agent.Connection{*connection}
	main := &scriptedModel{steps: []*schema.Message{
		call("kubernetes_resources_read", `{"purpose":"List Pods and collection coverage","connection_id":"captured","operation":"get","resource":"pods","namespace":"*"}`),
		answer("The API repeated its cursor, so the observed list is incomplete."),
	}}
	o.Model = main
	result, err := agent.Run(t.Context(), o)
	if err != nil || result.Text == "" || len(main.inputs) != 2 || requests.Load() != 3 {
		t.Fatal(result, err, len(main.inputs), requests.Load())
	}
	visible := main.inputs[1][len(main.inputs[1])-1].Content
	if !strings.Contains(visible, `"complete":false`) || strings.Contains(visible, `"kind":"PodList"`) || strings.Contains(visible, `"managedFields"`) {
		t.Fatal("projection lost partial coverage or retained raw result", visible)
	}
	evidence, err := history.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID)
	if err != nil || len(evidence) != 1 || evidence[0].Projection == nil || evidence[0].Projection.Coverage.Complete {
		t.Fatal(evidence, err)
	}
	if evidence[0].Projection.Coverage.Total != nil {
		t.Fatal("partial read invented total")
	}
}
