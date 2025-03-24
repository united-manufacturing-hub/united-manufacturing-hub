package backoff

import (
	"errors"
	"strings"
)

// IsTemporaryBackoffError checks if the error is a temporary backoff error
func IsTemporaryBackoffError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), TemporaryBackoffError)
}

// IsPermanentFailureError checks if the error is a permanent failure error
func IsPermanentFailureError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), PermanentFailureError)
}

// IsBackoffError checks if the error is any type of backoff error
func IsBackoffError(err error) bool {
	return IsTemporaryBackoffError(err) || IsPermanentFailureError(err)
}

// ExtractOriginalError attempts to unwrap all nested errors to get the root cause
func ExtractOriginalError(err error) error {
	if err == nil {
		return nil
	}

	// Keep unwrapping until we can't unwrap anymore
	var unwrapped error = err
	for {
		// Try to unwrap further
		next := errors.Unwrap(unwrapped)
		if next == nil {
			// We've reached the bottom of the chain
			return unwrapped
		}
		unwrapped = next
	}
}
