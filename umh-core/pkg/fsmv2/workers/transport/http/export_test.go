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

package transport

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// NewTransportErrorForTest creates a TransportError with explicit fields for testing.
// This is an internal test helper (export_test.go is only compiled during testing).
func NewTransportErrorForTest(errType types.ErrorType, statusCode int, message string, retryAfter time.Duration, inner error) *types.TransportError {
	return &types.TransportError{
		Type:       errType,
		StatusCode: statusCode,
		Message:    message,
		RetryAfter: retryAfter,
		Err:        inner,
	}
}
