package terminal

import (
	"context"
	"fmt"
	"io"
)

// Drain terminal replies before the connection completes so keyboard input and
// screen rendering cannot block on network setup or a stopped remote reader.
type Input struct {
	Ctx     context.Context
	Chunks  chan []byte
	Pending []byte
}

func (b *Input) Write(p []byte) (int, error) {
	copy := append([]byte(nil), p...)
	select {
	case <-b.Ctx.Done():
		return 0, io.EOF
	case b.Chunks <- copy:
		return len(p), nil
	default:
		return 0, fmt.Errorf("Terminal input buffer is full.")
	}
}

func (b *Input) Read(p []byte) (int, error) {
	if len(b.Pending) == 0 {
		select {
		case <-b.Ctx.Done():
			return 0, io.EOF
		case b.Pending = <-b.Chunks:
		}
	}
	n := copy(p, b.Pending)
	b.Pending = b.Pending[n:]
	return n, nil
}
