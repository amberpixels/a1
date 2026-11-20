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
	// ErrInvalid: the response decoded but the caller's validator (see
	// Validate) rejected it. Retryable unless the validator returned a
	// Permanent error. The returned error also wraps the validator's error.
	ErrInvalid = errors.New("a1: response rejected by validation")
)

// retryable marks an error worth another attempt (transient content
// failures). It wraps, so errors.Is still matches the underlying sentinel.
type retryable struct{ error }

func (r retryable) Unwrap() error { return r.error }

func isRetryable(err error) bool {
	var r retryable
	return errors.As(err, &r)
}

// permanent marks a validator error that resampling will not fix.
type permanent struct{ error }

func (p permanent) Unwrap() error { return p.error }

// Permanent marks an error returned by a Validate func as final: JSON stops
// retrying and returns it at once. The error JSON returns still wraps err,
// so errors.Is and errors.As keep matching it. Permanent(nil) is nil.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanent{err}
}

func isPermanent(err error) bool {
	var p permanent
	return errors.As(err, &p)
}
