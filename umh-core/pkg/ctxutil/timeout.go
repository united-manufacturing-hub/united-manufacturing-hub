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

package ctxutil

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNoDeadline indicates the context doesn't have a deadline.
	ErrNoDeadline = errors.New("context has no deadline")

	// ErrInsufficientTime indicates not enough time remains before deadline.
	ErrInsufficientTime = errors.New("insufficient time remaining before deadline")
)

// HasSufficientTime checks if context has enough remaining time
// Returns:
// - remaining: time remaining until deadline (0 if no deadline)
// - sufficient: true if enough time remains
// - err: error if no deadline or other issue.
func HasSufficientTime(ctx context.Context, requiredTime time.Duration) (remaining time.Duration, sufficient bool, err error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0, false, ErrNoDeadline
	}

	remaining = time.Until(deadline)
	if remaining < requiredTime {
		return remaining, false, nil // no error, because we want to continue
	}

	return remaining, true, nil
}
