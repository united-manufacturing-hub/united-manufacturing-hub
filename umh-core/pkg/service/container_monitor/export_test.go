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
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// SetCPUUsageProvider exposes the test seam to the external test package. It
// compiles only under `go test`, so the production API does not carry it.
func (c *ContainerMonitorService) SetCPUUsageProvider(fn func(ctx context.Context) (float64, error)) {
	c.setCPUUsageProvider(fn)
}

// CPUGaugeInputs exposes the gauge-source selection to the external test
// package. It decides which generation the three CPU Prometheus series are read
// from, so it needs a test of its own.
func CPUGaugeInputs(cpu *models.CPU, useFSMv2CPU bool) (usageMCores, cores float64, ok bool) {
	return cpuGaugeInputs(cpu, useFSMv2CPU)
}

// CollectCPUFromWorker exposes the worker arm of GetStatus to the external test
// package so its cancelled-tick arm is reachable without a full GetStatus.
func (c *ContainerMonitorService) CollectCPUFromWorker(ctx context.Context) (*models.CPU, error) {
	return c.collectCPUFromWorker(ctx)
}
