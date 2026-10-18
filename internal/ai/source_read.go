package ai

import (
	"context"
	"errors"

	"github.com/simjay/plous/internal/source"
)

func (op *operation) readSource(ctx context.Context, input string) (any, error) {
	id, args, err := connectionCommand(input)
	if err != nil {
		return nil, denyRead(err)
	}
	captured, err := op.connection(id, "source")
	if err != nil {
		return nil, denyRead(err)
	}
	var request source.ReadRequest
	if err := decode(args, &request); err != nil {
		return nil, denyRead(err)
	}
	if op.o.Store == nil {
		return nil, denyRead(errors.New("source reads require a captured profile boundary"))
	}
	profile, err := op.o.Store.DataRoot()
	if err != nil {
		return nil, err
	}
	if op.sourceReads == nil {
		op.sourceReads = map[string]*source.ReadSession{}
	}
	if op.sourceReads[id] == nil {
		op.sourceReads[id] = source.NewReadSession()
	}
	result, err := source.GuardedRead(ctx, source.ReadScope{
		ScanRules: op.o.Store.SourceIgnore, View: captured.SourceView, Session: op.sourceReads[id], Root: captured.Path, ProfileRoot: profile, ConnectionID: id, ConnectionName: captured.Name,
	}, request)
	if errors.Is(err, source.ErrReadDenied) {
		return nil, denyRead(err)
	}
	return result, err
}
