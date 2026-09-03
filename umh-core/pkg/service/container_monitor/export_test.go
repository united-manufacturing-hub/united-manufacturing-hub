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

// CollectCPUFromWorker exposes the worker arm of GetStatus to the external test
// package so its cancelled-tick arm is reachable without a full GetStatus.
func (c *ContainerMonitorService) CollectCPUFromWorker(ctx context.Context) (*models.CPU, error) {
	return c.collectCPUFromWorker(ctx)
}

// JudgeWorkerCPUReadError and JudgeWorkerCPU expose the seam's two judgement
// arms to the external test package. Both underlying functions read nothing
// beyond their arguments, which is what lets a spec reach every arm without
// publishing an fsmv2 client or standing up a store; these hooks are what make
// that property exercised rather than merely true. Each returns the pair
// readWorkerCPUHealth returns, so a spec sees what the seam reports.
func JudgeWorkerCPUReadError(err error) (*models.Health, *models.CPUHealth) {
	v := judgeWorkerCPUReadError(err)

	return v.health(), v.cpuHealth
}

func JudgeWorkerCPU(status simple.Status[fsmv2cpu.CPUStatus], freshness fsmv2client.Freshness) (*models.Health, *models.CPUHealth) {
	v := judgeWorkerCPU(status, freshness)

	return v.health(), v.cpuHealth
}
