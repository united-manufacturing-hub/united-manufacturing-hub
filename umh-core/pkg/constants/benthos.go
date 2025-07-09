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
	BenthosBaseDir        = "/data/benthos"
	BenthosConfigDirName  = "config"
	BenthosLogBaseDir     = "/data/logs"
	BenthosConfigFileName = "benthos.yaml"
	BenthosLogWindow      = time.Minute * 10
)

const (
	// Benthos Operation Timeouts - Level 1 Service (depends on S6)
	// Benthos depends on S6 for service management
	BenthosUpdateObservedStateTimeout = 10 * time.Millisecond
	BenthosRemoveTimeout              = 10 * time.Millisecond
	BenthosMaxLines                   = 10000
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
	// BenthosHealthCheckStableDurationInTicks represents the debounce period for
	// Benthos readiness.
	//
	// ⚠️  Why we need it
	// ------------------
	// Benthos exposes two booleans per instance:
	//
	//   • IsLive  – process is running
	//   • IsReady – *both* input and output are connected **right now**
	//
	// A pod can therefore oscillate like:
	//
	//   live=true, ready=true   ← connection succeeds
	//   live=true, ready=false  ← broker drops a socket a few ms later
	//
	// Our FSM used to consume IsReady verbatim, so a 1-frame "true" was enough to
	// advance the state machine.
	//
	// Change this constant if you need a different stability window.
	BenthosHealthCheckStableDurationInTicks = uint64(5 * time.Second / DefaultTickerTime) // 5 seconds
)
