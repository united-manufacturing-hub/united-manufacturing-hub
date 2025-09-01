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
	// Redpanda Operation Timeouts - Level 1 Service (depends on S6)
	// RedpandaUpdateObservedStateTimeout is the timeout for updating the observed state.
	RedpandaUpdateObservedStateTimeout = 40 * time.Millisecond
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

const (
	AdminAPIPort    = 9644
	AdminAPITimeout = 25 * time.Millisecond
)

const (
	DefaultRedpandaTopicDefaultTopicRetentionMs          = 604800000 // 7 days
	DefaultRedpandaTopicDefaultTopicRetentionBytes       = 0
	DefaultRedpandaTopicDefaultTopicCompressionAlgorithm = "snappy"
	DefaultRedpandaTopicDefaultTopicCleanupPolicy        = "compact"
	DefaultRedpandaTopicDefaultTopicSegmentMs            = 3600000 // 1 hour in milliseconds
)

const (
	SchemaRegistryPort    = 8081
	SchemaRegistryTimeout = 20 * time.Millisecond
)
