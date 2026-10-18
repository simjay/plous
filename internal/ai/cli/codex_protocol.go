package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

type codexProtocol struct {
	*process
}

type rpcMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int
		Message string
	} `json:"error"`
}

func (p *codexProtocol) request(id int, method string, params any) (json.RawMessage, error) {
	if err := p.send(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		var m rpcMessage
		if err := p.next(&m); err != nil {
			return nil, err
		}
		if m.Method != "" && len(m.ID) > 0 {
			return nil, errors.New("Codex requested an unsupported interaction during setup")
		}
		if string(m.ID) != strconv.Itoa(id) {
			continue
		}
		if m.Error != nil {
			return nil, fmt.Errorf("Codex rejected %s; check the installed version, configuration and sign-in", method)
		}
		return m.Result, nil
	}
}
