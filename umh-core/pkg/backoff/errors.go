// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	var unwrapped = err
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
