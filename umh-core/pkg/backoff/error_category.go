package backoff

import "errors"

// ErrorCategory indicates how the system (FSM + manager) should respond to a given error.
type ErrorCategory int

const (
	// CategoryIgnored indicates an error that is expected or benign
	// in the current context and should NOT trigger any backoff or removal.
	// Example: Healthcheck failures while still "starting."
	CategoryIgnored ErrorCategory = iota

	// CategoryTransient indicates an error that is unexpected but recoverable.
	// The FSM calls SetError(...) to mark this instance in backoff for
	// potential retries. If the same error repeats too many times (max retries),
	// we escalate to permanent failure.
	CategoryTransient

	// CategoryPermanent indicates a fatal, unrecoverable error.
	// The manager or FSM typically removes the instance forcibly—without
	// further grace or retries—once this error is received.
	//
	// N.B.: It's generally recommended that an instance *attempt* to gracefully
	// remove or stop itself before returning a permanent error,
	// since the manager might skip "stop commands" and forcibly remove the instance.
	CategoryPermanent
)

// CategorizedError is a wrapper that includes the underlying error plus a Category.
type CategorizedError struct {
	Err      error
	Category ErrorCategory
}

// Error returns the original error message.
func (ce *CategorizedError) Error() string {
	return ce.Err.Error()
}

// Unwrap returns the underlying wrapped error.
func (ce *CategorizedError) Unwrap() error {
	return ce.Err
}

// IsCategory checks if the CategorizedError has the specified category.
func (ce *CategorizedError) IsCategory(category ErrorCategory) bool {
	return ce.Category == category
}

// NewIgnoredError wraps err as CategoryIgnored.
func NewIgnoredError(err error) error {
	return &CategorizedError{Err: err, Category: CategoryIgnored}
}

// NewTransientError wraps err as CategoryTransient.
func NewTransientError(err error) error {
	return &CategorizedError{Err: err, Category: CategoryTransient}
}

// NewPermanentError wraps err as CategoryPermanent.
func NewPermanentError(err error) error {
	return &CategorizedError{Err: err, Category: CategoryPermanent}
}

// CategorizeError ensures that every error is at least Transient if not already a CategorizedError.
func CategorizeError(err error) error {
	if err == nil {
		return nil
	}
	var ce *CategorizedError
	if errors.As(err, &ce) {
		// Already categorized, so keep it as is.
		return err
	}
	// Otherwise, treat it as Transient by default.
	return NewTransientError(err)
}

// IsIgnoredError is a convenience checker for CategoryIgnored.
func IsIgnoredError(err error) bool {
	var ce *CategorizedError
	return errors.As(err, &ce) && ce.IsCategory(CategoryIgnored)
}

// IsTransientError is a convenience checker for CategoryTransient.
func IsTransientError(err error) bool {
	var ce *CategorizedError
	return errors.As(err, &ce) && ce.IsCategory(CategoryTransient)
}

// IsPermanentError is a convenience checker for CategoryPermanent.
func IsPermanentError(err error) bool {
	var ce *CategorizedError
	return errors.As(err, &ce) && ce.IsCategory(CategoryPermanent)
}
