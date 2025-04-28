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

package benthos_monitor

import "errors"

var (
	// ErrServiceNotExist indicates the requested service does not exist
	ErrServiceNotExist = errors.New("service does not exist")

	// ErrServiceAlreadyExists indicates the requested service already exists
	ErrServiceAlreadyExists = errors.New("service already exists")

	// ErrServiceNoLogFile indicates the health check had no logs to process
	ErrServiceNoLogFile = errors.New("log file not found")

	// ErrServiceConnectionRefused indicates the service connection was refused
	ErrServiceConnectionRefused = errors.New("connection refused, while attempting to fetch metrics from benthos. This is expected during the startup phase of the benthos service, when the service is not yet ready to receive connections")

	// ErrServiceConnectionTimedOut indicates the service connection timed out
	ErrServiceConnectionTimedOut = errors.New("connection timed out, while attempting to fetch metrics from benthos. This can happen if the benthos service or the system is experiencing high load")

	// ErrServiceNoSectionsFound indicates the benthos service has no sections
	ErrServiceNoSectionsFound = errors.New("could not parse benthos metrics/configuration: no sections found. This can happen when the benthos service is not running, or the logs where rotated")

	// ErrRemovalPending is returned by RemoveBenthosMonitorFromS6Manager while the
	// S6 manager is still busy shutting the service down.  Callers should
	// treat it as a *retryable* error.
	ErrRemovalPending = errors.New("service removal still in progress")
)
