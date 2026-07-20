package a1

import "errors"

// Typed outcomes of a completed API call. API/transport errors from the SDK
// are returned as-is (see the SDK's typed error values for those).
var (
	// ErrRefused: the model (or a safety classifier) declined the request —
	// stop_reason "refusal". Not retryable; retrying the same prompt refuses
	// again.
	ErrRefused = errors.New("a1: request refused")
	// ErrTruncated: the response hit MaxTokens before finishing — stop_reason
	// "max_tokens". Retryable (a rerun usually stays in budget), but a
	// persistent truncation means MaxTokens is too low for the task.
	ErrTruncated = errors.New("a1: response truncated (max_tokens)")
	// ErrEmpty: the response carried no text. Observed rarely in bulk runs;
	// retryable.
	ErrEmpty = errors.New("a1: empty response")
)

// retryable marks an error worth another attempt (transient content
// failures). It wraps, so errors.Is still matches the underlying sentinel.
type retryable struct{ error }

func (r retryable) Unwrap() error { return r.error }

func isRetryable(err error) bool {
	var r retryable
	return errors.As(err, &r)
}
