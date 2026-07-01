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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthosmetrics"
)

// BenthosMonitorConfig holds the user-provided configuration for the benthos
// monitor worker. Embeds BaseUserSpec so state files read user-facing State via
// Config.GetState() (the YAML "state" field, defaulting to "running").
type BenthosMonitorConfig struct {
	config.BaseUserSpec `yaml:",inline"`

	// MetricsPort is the benthos /metrics (and /ping, /ready, /version) port.
	MetricsPort uint16 `json:"metricsPort" yaml:"metricsPort"`
}

// BenthosMonitorStatus holds the runtime observation data for the benthos
// monitor worker.
type BenthosMonitorStatus struct {
	// Scan is the most recent parsed observation of the benthos instance.
	Scan benthosmetrics.Scan `json:"scan"`
	// Stopped is true when the worker's desired State is "stopped" and the
	// scrape was therefore skipped this tick. It distinguishes an
	// admin-paused instance (Stopped=true, Scan is the zero value) from a
	// benthos that is down or unreachable (Stopped=false, Scan.IsLive=false),
	// which the scrape path also reports as a zero Scan. State files and read
	// paths must consult Stopped before treating a zero Scan as a benthos
	// outage, or a stopped bridge reads as crashed.
	Stopped bool `json:"stopped"`
}
