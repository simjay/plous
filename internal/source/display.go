package source

import (
	"context"
	"errors"
)

// DisplayFile uses the agent's file boundary for presentation, with the same
// credential and path checks as reads. The bytes never become a tool receipt.
func DisplayFile(ctx context.Context, scope ReadScope, name, identity string) (string, string, error) {
	q := ReadRequest{Operation: "read", Path: name}
	if err := validateFileRead(&q); err != nil {
		return "", "", err
	}
	if err := validateReadScope(scope); err != nil {
		return "", "", err
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	reader, close, err := sourceReader(scope, nil)
	if err != nil {
		return "", "", err
	}
	defer close()
	data, found, err := reader.text(name)
	if err != nil {
		return "", "", err
	}
	if identity != "" && found != identity {
		return "", "", errors.New("source changed since the requested observation")
	}
	return string(data), found, ctx.Err()
}
