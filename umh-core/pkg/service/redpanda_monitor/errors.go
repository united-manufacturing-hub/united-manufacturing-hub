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

package redpanda_monitor

import "errors"

var (
	// ErrServiceNotExist indicates the requested service does not exist
	ErrServiceNotExist = errors.New("service does not exist")

	// ErrServiceAlreadyExists indicates the requested service already exists
	ErrServiceAlreadyExists = errors.New("service already exists")

	// ErrServiceNoLogFile indicates the health check had no logs to process
	ErrServiceNoLogFile = errors.New("log file not found")

	// ErrServiceNoSectionsFound indicates the redpanda service has no sections
	ErrServiceNoSectionsFound = errors.New("could not parse redpanda metrics/configuration: no sections found. This can happen when the redpanda service is not running, or the logs where rotated")
)
