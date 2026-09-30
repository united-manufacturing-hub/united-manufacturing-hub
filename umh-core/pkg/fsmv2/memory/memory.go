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
	SourceCgroupLimit                  MemorySource = "cgroup_limit"
	SourceHostTotalNoCgroupLimit       MemorySource = "host_total_no_cgroup_limit"
	SourceHostFallbackCgroupUnreadable MemorySource = "host_fallback_cgroup_unreadable"
)

type MemoryStatus struct {
	Source      MemorySource `json:"source"`
	UsedBytes   int64        `json:"usedBytes"`
	TotalBytes  int64        `json:"totalBytes"`
	UsedPercent float64      `json:"usedPercent"`
	Message     string       `json:"message"`
	Measured    bool         `json:"measured"`
}

type HostMemoryReader func(ctx context.Context) (usedBytes, totalBytes uint64, err error)

type MemoryDeps struct {
	*deps.BaseDependencies

	fileSystem       filesystem.Service
	hostMemory       HostMemoryReader
	reportedFailures sync.Map
}

func NewDeps(_ deps.Identity, bd *deps.BaseDependencies) *MemoryDeps {
	fileSystem := register.GetDeps[filesystem.Service](FilesystemDepsKey)
	if fileSystem == nil {
		fileSystem = filesystem.NewDefaultService()
	}

	return &MemoryDeps{BaseDependencies: bd, fileSystem: fileSystem, hostMemory: readHostMemory}
}

func readHostMemory(ctx context.Context) (uint64, uint64, error) {
	virtualMemory, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return 0, 0, err
	}

	return virtualMemory.Used, virtualMemory.Total, nil
}

func Poll(ctx context.Context, d *MemoryDeps, _ MemoryConfig) (MemoryStatus, error) {
	cgroup, cgroupErr := ReadCgroupMemory(ctx, d.fileSystem, cgroupBase)
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
		d.reportZeroTotal(status.Source)

		return MemoryStatus{}, errZeroTotal
	}

	status.UsedPercent = usedPercent(status.UsedBytes, status.TotalBytes)
	status.Message = messageFor(status.UsedPercent)
	status.Measured = true

	recordMetrics(d.MetricsRecorder(), time.Now(), status)

	return status, nil
}

func recordMetrics(recorder *deps.MetricsRecorder, sampledAt time.Time, status MemoryStatus) {
	recorder.SetGauge(deps.GaugeMemoryLastSampleUnix, float64(sampledAt.Unix()))
	recorder.SetGauge(deps.GaugeMemoryUsedBytes, float64(status.UsedBytes))
	recorder.SetGauge(deps.GaugeMemoryTotalBytes, float64(status.TotalBytes))
	recorder.SetGauge(deps.GaugeMemoryUsedPercent, status.UsedPercent)
	recorder.SetGaugeFlag(deps.GaugeMemoryUsedFromCgroup, status.Source != SourceHostFallbackCgroupUnreadable)
	recorder.SetGaugeFlag(deps.GaugeMemoryTotalIsCgroupLimit, status.Source == SourceCgroupLimit)
}

func chooseSource(ctx context.Context, d *MemoryDeps, cgroup CgroupMemory, cgroupErr error) (MemoryStatus, error) {
	if cgroupErr != nil {
		return hostFallback(ctx, d, cgroupErr)
	}

	if cgroup.Unlimited || cgroup.LimitBytes == 0 {
		return cgroupAgainstHostTotal(ctx, d, cgroup)
	}

	return MemoryStatus{Source: SourceCgroupLimit, UsedBytes: cgroup.CurrentBytes, TotalBytes: cgroup.LimitBytes}, nil
}

func hostFallback(ctx context.Context, d *MemoryDeps, cgroupErr error) (MemoryStatus, error) {
	hostUsed, hostTotal, hostErr := d.hostMemory(ctx)
	if hostErr != nil {
		d.reportHostReadFailure(ctx, hostErr)

		return MemoryStatus{}, fmt.Errorf("cgroup memory is unreadable (%w) and host memory is unreadable: %w", cgroupErr, hostErr)
	}

	return MemoryStatus{Source: SourceHostFallbackCgroupUnreadable, UsedBytes: int64(hostUsed), TotalBytes: int64(hostTotal)}, nil
}

func cgroupAgainstHostTotal(ctx context.Context, d *MemoryDeps, cgroup CgroupMemory) (MemoryStatus, error) {
	_, hostTotal, hostErr := d.hostMemory(ctx)
	if hostErr != nil {
		d.reportHostReadFailure(ctx, hostErr)

		return MemoryStatus{}, fmt.Errorf("cgroup memory has no limit and host memory is unreadable: %w", hostErr)
	}

	return MemoryStatus{Source: SourceHostTotalNoCgroupLimit, UsedBytes: cgroup.CurrentBytes, TotalBytes: int64(hostTotal)}, nil
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
