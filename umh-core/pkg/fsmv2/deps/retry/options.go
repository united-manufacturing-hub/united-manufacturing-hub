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

package retry

import "time"

// ErrorOption configures error recording behavior.
type ErrorOption func(*errorConfig)

type errorConfig struct {
	class      string
	retryAfter time.Duration
}

// WithClass sets the error classification (protocol-specific).
func WithClass(class string) ErrorOption {
	return func(cfg *errorConfig) {
		cfg.class = class
	}
}

// WithRetryAfter sets the server-suggested retry delay.
func WithRetryAfter(d time.Duration) ErrorOption {
	return func(cfg *errorConfig) {
		cfg.retryAfter = d
	}
}
