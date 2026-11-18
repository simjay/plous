package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type confirmationKey struct{}

// Confirm asks the MCP client to present a decision directly to the user.
func Confirm(ctx context.Context, message string) (bool, error) {
	prompts, _ := ctx.Value(confirmationKey{}).(*stdioPrompts)
	if prompts == nil {
		return false, errors.New("This client cannot confirm the target in chat.")
	}
	return prompts.confirm(ctx, message)
}

func stdioProtocol(params json.RawMessage) string {
	var request struct{ ProtocolVersion string }
	_ = json.Unmarshal(params, &request)
	switch request.ProtocolVersion {
	case "2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25":
		return request.ProtocolVersion
	default:
		return "2025-11-25"
	}
}

type stdioPrompts struct {
	mu          sync.Mutex
	form, ready bool
	next        uint64
	pending     chan promptReply
	send        func([]byte)
}

type stdioPacket struct {
	JSONRPC, Method           string
	ID, Params, Result, Error json.RawMessage
}

type promptReply struct {
	approved bool
	err      error
}

// receive handles negotiation and routes replies to the one outstanding prompt.
func (p *stdioPrompts) receive(packet stdioPacket) bool {
	if packet.JSONRPC != "2.0" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	switch packet.Method {
	case "initialize":
		var request struct {
			Capabilities struct{ Elicitation map[string]json.RawMessage }
		}
		if !p.ready && canonicalID(packet.ID) != "" && json.Unmarshal(packet.Params, &request) == nil {
			caps := request.Capabilities.Elicitation
			version := stdioProtocol(packet.Params)
			p.form = caps != nil && (len(caps) == 0 || caps["form"] != nil) &&
				(version == "2025-06-18" || version == "2025-11-25")
		}
	case "notifications/initialized":
		p.ready = true
	case "":
		var id string
		if json.Unmarshal(packet.ID, &id) != nil || !strings.HasPrefix(id, "plous-confirm-") {
			return false
		}
		if p.pending != nil && id == fmt.Sprintf("plous-confirm-%d", p.next) {
			var result struct {
				Action  string
				Content struct{ Confirm bool }
			}
			var reply promptReply
			if len(packet.Error) != 0 || json.Unmarshal(packet.Result, &result) != nil || result.Action == "" {
				reply.err = errors.New("The MCP client could not present a target confirmation.")
			}
			reply.approved = result.Action == "accept" && result.Content.Confirm
			select {
			case p.pending <- reply:
			default:
			}
		}
		return true
	}
	return false
}

func (p *stdioPrompts) confirm(ctx context.Context, message string) (bool, error) {
	p.mu.Lock()
	if !p.ready || !p.form || p.pending != nil {
		p.mu.Unlock()
		return false, errors.New("Target confirmation is unavailable in this chat.")
	}
	p.next++
	id := fmt.Sprintf("plous-confirm-%d", p.next)
	reply := make(chan promptReply, 1)
	p.pending = reply
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.pending = nil
		p.mu.Unlock()
	}()
	request, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": "elicitation/create",
		"params": map[string]any{"message": message, "requestedSchema": json.RawMessage(
			`{"type":"object","properties":{"confirm":{"type":"boolean","title":"Use this Kubernetes target"}},"required":["confirm"]}`)},
	})
	if err := ctx.Err(); err != nil {
		return false, err
	}
	p.send(request)
	wait, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	select {
	case result := <-reply:
		return result.approved, errors.Join(result.err, ctx.Err())
	case <-wait.Done():
		return false, errors.New("Target confirmation expired or was cancelled.")
	}
}
