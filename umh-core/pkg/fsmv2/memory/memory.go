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
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/mem"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const (
	WorkerType        = "memory"
	InstanceName      = "memory"
	FilesystemDepsKey = WorkerType + ".filesystem"
	PollInterval      = 1 * time.Second

	cgroupBase = "/sys/fs/cgroup"

	messageNormal   = "Memory utilization normal"
	messageWarning  = "Memory utilization warning"
	messageCritical = "Memory utilization critical"
)

var Ref = dynamicchildren.Ref{WorkerType: WorkerType, Name: InstanceName}

var errZeroTotal = errors.New("memory total is zero")

type MemoryConfig struct{}

type MemorySource string

const (
	SourceCgroup MemorySource = "cgroup"
	SourceHost   MemorySource = "host"
)

type MemoryStatus struct {
	Source      MemorySource `json:"source"`
	UsedBytes   int64        `json:"usedBytes"`
	TotalBytes  int64        `json:"totalBytes"`
	Unlimited   bool         `json:"unlimited"`
	UsedPercent float64      `json:"usedPercent"`
	Message     string       `json:"message"`
}

type HostMemoryReader func(ctx context.Context) (usedBytes, totalBytes uint64, err error)

type MemoryDeps struct {
	*deps.BaseDependencies

	filesystem    filesystem.Service
	hostMemory    HostMemoryReader
	reportedReads sync.Map
}

func NewDeps(_ deps.Identity, bd *deps.BaseDependencies) *MemoryDeps {
	fs := register.GetDeps[filesystem.Service](FilesystemDepsKey)
	if fs == nil {
		fs = filesystem.NewDefaultService()
	}

	return &MemoryDeps{BaseDependencies: bd, filesystem: fs, hostMemory: readHostMemory}
}

func readHostMemory(ctx context.Context) (uint64, uint64, error) {
	virtualMemory, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return 0, 0, err
	}

	return virtualMemory.Used, virtualMemory.Total, nil
}

func Poll(ctx context.Context, d *MemoryDeps, _ MemoryConfig) (MemoryStatus, error) {
	cgroup, cgroupErr := ReadCgroupMemory(ctx, d.filesystem, cgroupBase)
	if ctx.Err() != nil {
		return MemoryStatus{}, ctx.Err()
	}

	if cgroupErr != nil {
		d.reportCgroupReadFailure(cgroupErr)
	}

	status, err := chooseSource(ctx, d, cgroup, cgroupErr)
	if err != nil {
		return MemoryStatus{}, err
	}

	if status.TotalBytes <= 0 {
		return MemoryStatus{}, errZeroTotal
	}

	status.UsedPercent = usedPercent(status.UsedBytes, status.TotalBytes)
	status.Message = messageFor(status.UsedPercent)

	recordMetrics(d.MetricsRecorder(), time.Now(), status)

	return status, nil
}

func recordMetrics(recorder *deps.MetricsRecorder, sampledAt time.Time, status MemoryStatus) {
	recorder.SetGauge(deps.GaugeMemoryLastSampleUnix, float64(sampledAt.Unix()))
	recorder.SetGauge(deps.GaugeMemoryUsedBytes, float64(status.UsedBytes))
	recorder.SetGauge(deps.GaugeMemoryTotalBytes, float64(status.TotalBytes))
	recorder.SetGauge(deps.GaugeMemoryUsedPercent, status.UsedPercent)
	recorder.SetGaugeFlag(deps.GaugeMemorySourceIsCgroup, status.Source == SourceCgroup)
}

func chooseSource(ctx context.Context, d *MemoryDeps, cgroup CgroupMemory, cgroupErr error) (MemoryStatus, error) {
	cgroupHasLimit := cgroupErr == nil && !cgroup.Unlimited && cgroup.LimitBytes > 0
	if cgroupHasLimit {
		return MemoryStatus{Source: SourceCgroup, UsedBytes: cgroup.CurrentBytes, TotalBytes: cgroup.LimitBytes}, nil
	}

	hostUsed, hostTotal, hostErr := d.hostMemory(ctx)

	if cgroupErr != nil {
		if hostErr != nil {
			return MemoryStatus{}, errors.Join(cgroupErr, hostErr)
		}

		return MemoryStatus{Source: SourceHost, UsedBytes: int64(hostUsed), TotalBytes: int64(hostTotal)}, nil
	}

	if hostErr != nil {
		return MemoryStatus{}, fmt.Errorf("cgroup has no memory limit and the host total is unreadable: %w", hostErr)
	}

	return MemoryStatus{
		Source:     SourceCgroup,
		UsedBytes:  cgroup.CurrentBytes,
		TotalBytes: int64(hostTotal),
		Unlimited:  cgroup.Unlimited,
	}, nil
}

func usedPercent(usedBytes, totalBytes int64) float64 {
	return float64(usedBytes) * 100 / float64(totalBytes)
}

func isCritical(percent float64) bool {
	return percent >= constants.MemoryHighThresholdPercent
}

func messageFor(percent float64) string {
	if isCritical(percent) {
		return messageCritical
	}

	if percent >= constants.MemoryMediumThresholdPercent {
		return messageWarning
	}

	return messageNormal
}

func healthFromStatus(_ MemoryConfig, status MemoryStatus) simple.Health {
	if isCritical(status.UsedPercent) {
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
