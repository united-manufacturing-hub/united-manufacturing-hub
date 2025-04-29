package standarderrors

import "errors"

var (
	// ErrInstanceRemoved is returned when an instance has been successfully removed
	ErrInstanceRemoved = errors.New("instance removed")

	// ErrRemovalPending is returned by RemoveBenthosFromS6Manager while the
	// S6 manager is still busy shutting the service down.  Callers should
	// treat it as a *retryable* error.
	ErrRemovalPending = errors.New("service removal still in progress")
)
