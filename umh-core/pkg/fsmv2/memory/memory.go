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

package fsmv2memory

import (
	"context"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/memoryhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const (
	WorkerType        = "memory"
	InstanceName      = "memory"
	FilesystemDepsKey = WorkerType + ".filesystem"
	PollInterval      = 1 * time.Second

	cgroupBase = "/sys/fs/cgroup"
)

// Ref names the memory monitor child the config worker upserts.
var Ref = dynamicchildren.Ref{WorkerType: WorkerType, Name: InstanceName}

// MemoryConfig is empty: the memory monitor takes no configuration.
type MemoryConfig struct{}

// MemoryStatus is one poll's sample and the verdict judged from it.
type MemoryStatus struct {
	Source     memoryhealth.Source  `json:"source"`
	UsedBytes  int64                `json:"usedBytes"`
	TotalBytes int64                `json:"totalBytes"`
	Verdict    memoryhealth.Verdict `json:"verdict"`
	Details    memoryhealth.Details `json:"details"`
	Message    string               `json:"message"`
}

// MemoryDeps holds the sampler and the engine that keeps the memory windows.
type MemoryDeps struct {
	*deps.BaseDependencies

	sampler          *memoryhealth.Sampler
	engine           *diagnosis.Engine[memoryhealth.Sample]
	engineErr        error
	reportedFailures sync.Map
}

// FilesystemKey names the filesystem.Service the sampler reads the cgroup files
// through. NewDeps reads it first, then the global published under
// FilesystemDepsKey, and falls back to filesystem.NewDefaultService().
var FilesystemKey = config.NewDependencyKey[filesystem.Service](FilesystemDepsKey)

// NewDeps builds the memory deps, reading through the filesystem named by
// FilesystemKey.
func NewDeps(_ deps.Identity, bd *deps.BaseDependencies, dependencies map[string]any) *MemoryDeps {
	fileSystem, ok := config.LookupDependency(dependencies, FilesystemKey)
	if !ok {
		fileSystem = register.GlobalDeps[filesystem.Service](FilesystemDepsKey)
	}
	if fileSystem == nil {
		fileSystem = filesystem.NewDefaultService()
	}

	return newMemoryDeps(bd, memoryhealth.NewSampler(fileSystem, cgroupBase, memoryhealth.ReadHostMemory))
}

func newMemoryDeps(bd *deps.BaseDependencies, sampler *memoryhealth.Sampler) *MemoryDeps {
	engine, engineErr := memoryhealth.NewEngine()

	return &MemoryDeps{BaseDependencies: bd, sampler: sampler, engine: engine, engineErr: engineErr}
}

// Poll samples the memory once and judges it. It fails when no memory total
// could be measured.
func Poll(ctx context.Context, d *MemoryDeps, _ MemoryConfig) (MemoryStatus, error) {
	if d.engineErr != nil {
		return MemoryStatus{}, d.engineErr
	}

	sample, err := d.sampler.Read(ctx)
	d.reportFailedReads(ctx, sample, err)

	if err != nil {
		return MemoryStatus{}, err
	}

	verdict, details := memoryhealth.Decide(d.engine, sample)

	return MemoryStatus{
		Source:     sample.Source,
		UsedBytes:  sample.UsedBytes,
		TotalBytes: sample.TotalBytes,
		Verdict:    verdict,
		Details:    details,
		Message:    memoryhealth.ComposeMessage(verdict, details),
	}, nil
}

func healthFromStatus(_ MemoryConfig, status MemoryStatus) simple.Health {
	if status.Verdict.State == memoryhealth.StateDegraded || !status.Details.Measured() {
		return simple.Degraded(status.Message)
	}

	return simple.Healthy(status.Message)
}

var monitorSpec = simple.MonitorSpec[MemoryConfig, MemoryStatus, *MemoryDeps]{
	WorkerType: WorkerType,
	Interval:   PollInterval,
	NewDeps:    NewDeps,
	Poll:       Poll,
	Health:     healthFromStatus,
}

func init() {
	simple.Register(monitorSpec)
}
