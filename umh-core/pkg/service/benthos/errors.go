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

package benthos

import "errors"

var (
	// ErrServiceNotExist indicates the requested service does not exist.
	ErrServiceNotExist = errors.New("service does not exist")

	// ErrServiceAlreadyExists indicates the requested service already exists.
	ErrServiceAlreadyExists = errors.New("service already exists")

	// ErrHealthCheckConnectionRefused indicates the health check connection was refused.
	ErrHealthCheckConnectionRefused = errors.New("health check connection refused")

	// ErrServiceNoLogFile indicates the health check had no logs to process.
	ErrServiceNoLogFile = errors.New("log file not found")

	// ErrBenthosMonitorNotRunning indicates the benthos monitor is not running.
	ErrBenthosMonitorNotRunning = errors.New("benthos monitor is not running")

	// ErrLastObservedStateNil indicates the last observed state is nil.
	ErrLastObservedStateNil = errors.New("last observed state is nil")
)
