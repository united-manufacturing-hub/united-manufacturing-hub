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

package constants

import "time"

const (
	RedpandaConfigFileName = "redpanda.yaml"
	RedpandaServiceName    = "redpanda"
)

const (
	RedpandaLogWindow = 10 * time.Minute
)

const (
	RedpandaUpdateObservedStateTimeout = 20 * time.Millisecond
	RedpandaProcessMetricsTimeout      = 8 * time.Millisecond // needs to be smaller than RedpandaUpdateObservedStateTimeout
)
const (
	// RedpandaExpectedMaxP95ExecutionTimePerInstance represents the maximum expected time for a Redpanda instance
	// to update its observed state. We reserve 50ms of buffer time to ensure the total execution time stays
	// under the alerting threshold. The system has a 100ms max ticker time with an 80% (80ms) alerting threshold.
	// By setting this to RedpandaUpdateObservedStateTimeout + 35ms, we ensure that Redpanda operations
	// complete with enough margin to avoid triggering alerts during normal operation.
	RedpandaExpectedMaxP95ExecutionTimePerInstance = RedpandaUpdateObservedStateTimeout + time.Millisecond*35 // needs to be higher than S6ExpectedMaxP95ExecutionTimePerInstance
)

var (
	// Set by build process via ldflags using -ldflags="-X github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants.RedpandaVersion=${REDPANDA_VERSION}"
	// This injects the version at build time from the environment, eliminating the need for hard-coded values.
	RedpandaVersion = "unknown"
)

const (
	DefaultRedpandaBaseDir = "/data"
)

const (
	RedpandaMaxMetricsAndConfigAge = 10 * time.Second
)
