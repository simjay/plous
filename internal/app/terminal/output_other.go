//go:build !darwin && !linux

package terminal

import (
	"errors"
	"os"
)

type Output struct {
	Renderer, Diagnostics *os.File
}

func ReserveOutput() (*Output, error) {
	return nil, errors.New("interactive Plous requires macOS or Linux")
}

func (t *Output) Close() error { return nil }
