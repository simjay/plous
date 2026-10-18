package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"

	bench "github.com/simjay/plous/benchmark"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/cli"
)

type mcpTools struct {
	bridge      *cli.Bridge
	client      *http.Client
	sequence    int
	definitions []agent.Tool
}

func openMCP(ctx context.Context, tools agent.Tools, id string) (*mcpTools, error) {
	b, err := cli.OpenBridge(ctx, tools, id)
	if err != nil {
		return nil, err
	}
	m := &mcpTools{bridge: b, client: &http.Client{Transport: &http.Transport{Proxy: nil}}}
	if _, err = m.rpc(ctx, "initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "plousbench", "version": "1"}}); err != nil {
		m.Close()
		return nil, err
	}
	raw, err := m.rpc(ctx, "tools/list", map[string]any{})
	if err != nil {
		m.Close()
		return nil, err
	}
	var listed struct {
		Tools []struct {
			Name, Description string
			InputSchema       json.RawMessage
		}
	}
	if err = json.Unmarshal(raw, &listed); err != nil {
		m.Close()
		return nil, err
	}
	for _, t := range listed.Tools {
		m.definitions = append(m.definitions, agent.Tool{Name: t.Name, Description: t.Description, Parameters: t.InputSchema})
	}
	var local, remote any
	_ = json.Unmarshal(bench.JSON(tools.Definitions()), &local)
	_ = json.Unmarshal(bench.JSON(m.definitions), &remote)
	if !reflect.DeepEqual(local, remote) {
		m.Close()
		return nil, fmt.Errorf("MCP tool definitions differ from the guarded session")
	}
	return m, nil
}

func (m *mcpTools) Close()                    { m.bridge.Close(); m.client.CloseIdleConnections() }
func (m *mcpTools) Definitions() []agent.Tool { return m.definitions }

func (m *mcpTools) rpc(ctx context.Context, method string, params any) (json.RawMessage, error) {
	m.sequence++
	request := bench.JSON(map[string]any{"jsonrpc": "2.0", "id": strconv.Itoa(m.sequence), "method": method, "params": params})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, m.bridge.URL, bytes.NewReader(request))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", m.bridge.Authorization())
	r.Header.Set("Content-Type", "application/json")
	response, err := m.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MCP HTTP status %d", response.StatusCode)
	}
	var reply struct {
		Result json.RawMessage
		Error  *struct {
			Code    int
			Message string
		}
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&reply); err != nil {
		return nil, err
	}
	if reply.Error != nil {
		return nil, fmt.Errorf("MCP %d: %s", reply.Error.Code, reply.Error.Message)
	}
	return reply.Result, nil
}

func (m *mcpTools) Call(ctx context.Context, call agent.ToolCall, _ int) (json.RawMessage, error) {
	raw, err := m.rpc(ctx, "tools/call", map[string]any{"name": call.Name, "arguments": call.Args})
	if err != nil {
		return nil, err
	}
	var reply struct {
		Content []struct{ Type, Text string }
		IsError bool
	}
	if err = json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}
	if reply.IsError || len(reply.Content) != 1 || reply.Content[0].Type != "text" {
		return nil, fmt.Errorf("invalid MCP tool result")
	}
	return json.RawMessage(reply.Content[0].Text), nil
}
