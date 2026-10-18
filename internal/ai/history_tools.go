package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	journal "github.com/simjay/plous/internal/ai/history"
	aimodel "github.com/simjay/plous/internal/ai/model"
	"github.com/simjay/plous/internal/integrations/observation"
)

func (op *operation) setupHistory() error {
	options := journal.SearchOptions{Monitor: op.hasTool("kubernetes") || op.hasTool("helm")}
	if options.Monitor && op.runtime.Embedding.Connection != "" {
		options.EmbeddingID = op.runtime.Embedding.Connection + "/" + op.runtime.Embedding.Model
		embed, identity, err := op.embeddingProvider()
		if err != nil {
			options.Embed = func(context.Context, []string) ([][]float64, int64, error) {
				return nil, 0, errors.New("configured embedding provider unavailable")
			}
		} else {
			options.Embed, options.EmbeddingID = embed, identity
		}
	}

	index, err := op.o.History.SearchIndex(op.o.WorkspaceID, op.o.ConversationID, options)
	// A derived-index failure does not invalidate the durable evidence or live reads.
	if err != nil {
		index = nil
	}
	op.historyAccess, err = journal.NewAccess(op.o.History, op.o.WorkspaceID, op.o.ConversationID, index)
	return err
}
func (op *operation) embeddingProvider() (journal.EmbedFunc, string, error) {
	settings, err := op.o.Store.ReadSettings()
	if err != nil {
		return nil, "", err
	}
	for _, connection := range settings.Configs() {
		if connection.Name != op.runtime.Embedding.Connection {
			continue
		}
		if connection.Provider != "openai" {
			return nil, "", errors.New("embedding connection must support the OpenAI embeddings API")
		}
		if err = connection.Validate(); err != nil {
			return nil, "", err
		}
		key, err := op.o.Store.Credential(connection)
		if err != nil {
			return nil, "", err
		}
		op.keys = append(op.keys, key)
		selected := op.runtime.Embedding.Model
		return func(ctx context.Context, texts []string) ([][]float64, int64, error) {
			return aimodel.Embed(ctx, connection, key, selected, texts)
		}, connection.Name + "/" + connection.BaseURL() + "/" + selected, nil
	}
	return nil, "", errors.New("configured embedding connection does not exist")
}

func historyDefinitions() []aimodel.Tool {
	parameters := journal.ReadSchema()
	parameters["properties"].(map[string]any)["purpose"] = map[string]any{"type": "string", "minLength": 1}
	parameters["required"] = append(parameters["required"].([]string), "purpose")
	return []aimodel.Tool{{Name: "history_read", Description: journal.ReadHelp, Parameters: marshal(parameters)}}
}
func (op *operation) historyTool(ctx context.Context, args json.RawMessage) (any, observation.Result, error) {
	r := observation.New("history.read.v1", "history", time.Now().UTC())
	r.Facts["meaning"] = observation.Text("selected saved evidence; observation times belong to original references")
	var reply any
	var q journal.HistoryRequest
	if err := decode(string(args), &q); err != nil {
		return nil, r, denyRead(err)
	}
	out, err := op.historyAccess.Call(ctx, q, op.toolBytes())
	if err != nil {
		var invalid *journal.QueryError
		if errors.As(err, &invalid) {
			return nil, r, denyRead(err)
		}
		return nil, r, err
	}
	reply = out
	for _, e := range out.Evidence {
		facts := map[string]observation.Value{"role": observation.Text(e.Role), "original_time": observation.Text(e.Time.Format(time.RFC3339Nano))}
		if e.Observation != nil {
			facts["selected_facts"] = observation.Text(e.Observation.SearchText())
		} else {
			facts["excerpt"] = observation.Text(e.Text)
		}
		r.Rows = append(r.Rows, observation.Row{Identity: observation.Identity{Kind: "SavedEvidence", Name: e.Ref}, Facts: facts})
	}
	if out.Search != nil {
		for _, hit := range out.Search.Hits {
			r.Rows = append(r.Rows, observation.Row{Identity: observation.Identity{Kind: "SavedExcerpt", Name: hit.Ref}, Facts: map[string]observation.Value{"excerpt": observation.Text(hit.Text), "role": observation.Text(hit.Role), "offset": observation.Number(int64(hit.Fragment))}})
		}
		r.Facts["lexical_complete"] = observation.Boolean(out.Search.LexicalComplete)
		r.Facts["semantic"] = observation.Text(out.Search.Semantic)
	}
	if out.Calculation != nil {
		c := out.Calculation
		r.Facts["observed"] = observation.Number(int64(c.Observed))
		r.Facts["matched"] = observation.Number(int64(c.Matched))
		r.Facts["unknown"] = observation.Number(int64(c.Unknown))
		r.Facts["shared"] = observation.Number(int64(c.Shared))
		r.Facts["added"] = observation.Number(int64(c.Added))
		r.Facts["removed"] = observation.Number(int64(c.Removed))
		r.Facts["complete"] = observation.Boolean(c.Complete)
		for i, ref := range c.Refs {
			r.Facts[fmt.Sprintf("source_%d", i)] = observation.Text(ref)
		}
	}
	r.Coverage.Observed = len(r.Rows)
	r.Partial("selected history evidence does not establish complete historical coverage")
	return reply, r, nil
}
