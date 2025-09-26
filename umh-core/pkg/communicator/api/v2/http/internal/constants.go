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

package internal

import "time"

const (
	// HTTP Retry Configuration Constants.

	// MaxRetryAttempts defines the maximum number of retry attempts for HTTP requests
	// before giving up. This helps prevent infinite retry loops while still providing
	// resilience against transient network issues.
	MaxRetryAttempts = 10

	// MinRetryWait defines the minimum wait time between retry attempts.
	// This prevents overwhelming the server with rapid-fire retries.
	MinRetryWait = 1 * time.Second

	// MaxRetryWait defines the maximum wait time between retry attempts.
	// This caps the exponential backoff to prevent excessively long waits.
	MaxRetryWait = 5 * time.Second

	// HTTP Long Poll Configuration Constants.

	// ConnectionKeepAlive is the value set for the Connection header when long polling is enabled.
	// This keeps the connection alive to reduce overhead for subsequent requests.
	ConnectionKeepAlive = "keep-alive"

	// KeepAliveTimeout defines the keep-alive timeout and maximum connections for long polling.
	// Format: "timeout=30, max=1000" where timeout is in seconds and max is the maximum
	// number of requests per connection.
	KeepAliveTimeout = "timeout=30, max=1000"

	// HTTP Header Constants.

	// ContentTypeJSON is the MIME type for JSON content, used in POST requests.
	ContentTypeJSON = "application/json"

	// LongPollFeatureHeader is the value set for the X-Features header when long polling is enabled.
	// This indicates to the server that the client supports long polling functionality.
	LongPollFeatureHeader = "longpoll;"
)
