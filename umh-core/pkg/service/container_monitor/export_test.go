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

	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// SetCPUUsageProvider exposes the test seam to the external test package. It
// compiles only under `go test`, so the production API does not carry it.
func (c *ContainerMonitorService) SetCPUUsageProvider(fn func(ctx context.Context) (float64, error)) {
	c.setCPUUsageProvider(fn)
}

// CPUGaugeInputs exposes the gauge-source selection to the external test
// package. It decides whether the three CPU Prometheus series are read from the
// worker's evidence or the legacy fields, so it needs a test of its own.
func CPUGaugeInputs(cpu *models.CPU, useFSMv2CPU bool) (usageMCores, cores float64, ok bool) {
	return cpuGaugeInputs(cpu, useFSMv2CPU)
}

// CollectCPUFromWorker exposes GetStatus's fsmv2-worker CPU path to the external test
// package, so a spec can reach its cancelled-tick return without a full
// GetStatus.
func (c *ContainerMonitorService) CollectCPUFromWorker(ctx context.Context) (*models.CPU, error) {
	return c.collectCPUFromWorker(ctx)
}

// JudgeWorkerCPUReadError exposes to the external test package the seam's
// verdict on an observation the store could not return. It returns the health
// and the evidence that verdict renders, which is what the seam reports in
// that case. The judgement cannot fail, so the hook does not carry the error
// readWorkerCPUHealth returns for a cancelled tick.
func JudgeWorkerCPUReadError(err error) (*models.Health, *models.CPUHealth) {
	v := judgeWorkerCPUReadError(err)

	return v.health(), v.cpuHealth
}

// JudgeWorkerCPU exposes to the external test package the seam's verdict on an
// observation the store did return. It returns the same pair as
// JudgeWorkerCPUReadError, for whichever verdict the freshness and the status
// select.
func JudgeWorkerCPU(status simple.Status[fsmv2cpu.CPUStatus], freshness fsmv2client.Freshness) (*models.Health, *models.CPUHealth) {
	v := judgeWorkerCPU(status, freshness)

	return v.health(), v.cpuHealth
}
