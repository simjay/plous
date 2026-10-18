package observation

import (
	"context"
	"errors"
)

// ReadFailure carries an integration-sanitized operational failure. Other
// errors stop execution instead of becoming successful tool replies.
type ReadFailure struct {
	Message string
	Denied  bool
}

func (e *ReadFailure) Error() string { return e.Message }

// FinishRead retains accepted evidence even when acquisition stops. Integrations
// call it before finalizing their aggregate facts.
func (r *Result) FinishRead(err error) error {
	if err == nil {
		return nil
	}
	outcome, message := "error", "The read stopped because its response could not be validated."
	var failure *ReadFailure
	expected := errors.As(err, &failure)
	switch {
	case errors.Is(err, context.Canceled):
		message = "The read was cancelled."
		expected = false
	case errors.Is(err, context.DeadlineExceeded):
		message = "The read timed out."
		expected = false
	case expected:
		message = failure.Message
		if failure.Denied {
			outcome = "denied"
		}
	}
	r.Partial(message)
	if r.Coverage.Observed == 0 && len(r.Rows) == 0 {
		r.Outcome = outcome
	}
	r.Facts["error"] = Text(message)
	if expected {
		return nil
	}
	return err
}
