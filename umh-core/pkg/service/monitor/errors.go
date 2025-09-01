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

package monitor

import "errors"

var (
	// ErrServiceConnectionRefused indicates the service connection was refused.
	ErrServiceConnectionRefused = errors.New("connection refused, while attempting to fetch metrics via curl. This is expected during the startup phase of the service, when the service is not yet ready to receive connections")

	// ErrServiceConnectionTimedOut indicates the service connection timed out.
	ErrServiceConnectionTimedOut = errors.New("connection timed out, while attempting to fetch metrics via curl. This can happen if the service or the system is experiencing high load")

	// ErrServiceStopped indicates the service is stopped.
	ErrServiceStopped = errors.New("service is stopped")
)
