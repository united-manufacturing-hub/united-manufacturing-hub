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
	"time"

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

type MemoryDeps struct {
	*deps.BaseDependencies

	filesystem filesystem.Service
}

func NewDeps(_ deps.Identity, bd *deps.BaseDependencies) *MemoryDeps {
	fs := register.GetDeps[filesystem.Service](FilesystemDepsKey)
	if fs == nil {
		fs = filesystem.NewDefaultService()
	}

	return &MemoryDeps{BaseDependencies: bd, filesystem: fs}
}

func Poll(ctx context.Context, d *MemoryDeps, _ MemoryConfig) (MemoryStatus, error) {
	cgroup, err := ReadCgroupMemory(ctx, d.filesystem, cgroupBase)
	if err != nil {
		return MemoryStatus{}, err
	}

	if cgroup.Unlimited || cgroup.LimitBytes == 0 {
		return MemoryStatus{}, errors.New("cgroup memory has no limit")
	}

	status := MemoryStatus{
		Source:     SourceCgroup,
		UsedBytes:  cgroup.CurrentBytes,
		TotalBytes: cgroup.LimitBytes,
	}
	status.UsedPercent = usedPercent(status.UsedBytes, status.TotalBytes)
	status.Message = messageFor(status.UsedPercent)

	return status, nil
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
