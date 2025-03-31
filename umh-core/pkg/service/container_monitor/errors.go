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

package container_monitor

import (
	"errors"
	"fmt"
)

// Container monitor specific errors
var (
	// ErrCPUMetrics indicates a failure in collecting CPU metrics
	ErrCPUMetrics = errors.New("failed to collect CPU metrics")

	// ErrMemoryMetrics indicates a failure in collecting memory metrics
	ErrMemoryMetrics = errors.New("failed to collect memory metrics")

	// ErrDiskMetrics indicates a failure in collecting disk metrics
	ErrDiskMetrics = errors.New("failed to collect disk metrics")

	// ErrHWIDCollection indicates a failure in retrieving the hardware ID
	ErrHWIDCollection = errors.New("failed to retrieve hardware ID")

	// ErrUnsupportedOS indicates the operating system is not supported
	ErrUnsupportedOS = errors.New("unsupported operating system")
)

// WrapMetricsError wraps an error with additional context
func WrapMetricsError(baseErr error, message string) error {
	return fmt.Errorf("%s: %w", message, baseErr)
}
