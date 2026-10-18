package history

// QueryError describes a repairable history-tool argument error.
// Storage, integrity, scope and cancellation errors keep their original type.
type QueryError struct{ Message string }

func (e *QueryError) Error() string     { return e.Message }
func invalidQuery(message string) error { return &QueryError{Message: message} }
