package source

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	maxFile           = 2 << 20
	maxSource         = 32 << 20
	maxFiles          = 4096 // Captured render inputs only.
	sourceFileBytes   = maxFile
	sourcePageEntries = 20000
)

// sourceWork bounds a unit of source work. Search yields a continuation at its
// work boundary; discovery and capture return explicit errors at storage limits.
type sourceWork struct {
	ctx            context.Context
	deadline       time.Time
	bytes, entries int
	retained       int
}

func newSourceWork(ctx context.Context) *sourceWork {
	return &sourceWork{ctx: ctx, deadline: time.Now().Add(5 * time.Second)}
}
func (w *sourceWork) exhausted() bool {
	return w.bytes >= maxSource || w.entries >= sourcePageEntries || time.Now().After(w.deadline)
}
func (w *sourceWork) retain(size int) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	w.retained += size
	if w.retained > maxSource {
		return errors.New("source metadata exceeds 32 MiB; choose a smaller directory")
	}
	return nil
}
func entryBytes(entries []sourceEntry) int {
	total := 0
	for _, entry := range entries {
		total += 128 + len(entry.Path)
	}
	return total
}

// Metadata size is an encoded-size estimate, not an exact Go heap measurement.
func (w *sourceWork) record(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.retain(len(data) + 128)
}
