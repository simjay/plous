//go:build !darwin && !linux

package app

import (
	"errors"
	"os"
)

type terminalOutput struct {
	renderer, diagnostics *os.File
}

func reserveTerminalOutput() (*terminalOutput, error) {
	return nil, errors.New("interactive Plous requires macOS or Linux")
}

func (t *terminalOutput) Close() error { return nil }
