package errorhandling

import "errors"

var (
	// ErrNoDeadline indicates the context doesn't have a deadline
	ErrNoDeadline = errors.New("context has no deadline")

	// ErrInsufficientTime indicates not enough time remains before deadline
	ErrInsufficientTime = errors.New("insufficient time remaining before deadline")
)
