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
	BenthosConfigFileName = "benthos.yaml"
)

const (
	BenthosLogWindow = time.Minute * 10
)

const (
	BenthosUpdateObservedStateTimeout = time.Millisecond * 5
)

const (
	// BenthosExpectedMaxP95ExecutionTimePerInstance means that an instance will not reconcile if not 30ms are left
	// Note: in the intergation test, we defined an alerting threshold of 80% of the max ticker time, which is 100ms
	// So by setting this to 30 ms, we can ensure that an instance will never start if it triggers the alerting threshold
	BenthosExpectedMaxP95ExecutionTimePerInstance = time.Millisecond * 40 // needs to be higher than S6ExpectedMaxP95ExecutionTimePerInstance and also higher than benthos monitor
)

const (
	// DefaultBenthosLogLevel is the default log level for Benthos services when none is specified
	DefaultBenthosLogLevel = "INFO"
)

var (
	// Set by build process via ldflags using -ldflags="-X github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants.BenthosVersion=${BENTHOS_VERSION}"
	// This injects the version at build time from the environment, eliminating the need for hard-coded values.
	BenthosVersion = "unknown"
)

const (
	BenthosMaxMetricsAndConfigAge = time.Second * 10
)

const (
	BenthosTimeUntilConfigLoadedInSeconds = 5
	BenthosTimeUntilRunningInSeconds      = 10
)
