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

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthosmetrics"
)

// Type aliases re-export the value types, health types, and metrics-state
// types that now live in pkg/service/benthosmetrics. They exist so that the
// existing importers of benthos_monitor (the FSM layer, the benthos service,
// the protocol-converter / dataflow / topicbrowser mocks, and this package's
// own internal files) continue to compile unchanged.
//
// Go cannot alias unexported foreign identifiers, so the three previously
// unexported types connStatus / readyResponse / versionResponse were exported
// in benthosmetrics as ConnStatus / ReadyResponse / VersionResponse; the
// unexported aliases below keep the old lowercase names valid inside this
// package (e.g. mock.go's []connStatus).
type (
	Metrics             = benthosmetrics.Metrics
	InputInstance       = benthosmetrics.InputInstance
	OutputInstance      = benthosmetrics.OutputInstance
	ProcessMetrics      = benthosmetrics.ProcessMetrics
	ProcessorMetrics    = benthosmetrics.ProcessorMetrics
	Latency             = benthosmetrics.Latency
	HealthCheck         = benthosmetrics.HealthCheck
	BenthosMetrics      = benthosmetrics.BenthosMetrics
	BenthosMetricsState = benthosmetrics.BenthosMetricsState

	// Exported alias (future-proof) for the connection-status type.
	ConnStatus = benthosmetrics.ConnStatus
	// Unexported alias — mock.go uses []connStatus.
	connStatus = benthosmetrics.ConnStatus

	readyResponse   = benthosmetrics.ReadyResponse
	versionResponse = benthosmetrics.VersionResponse
)
