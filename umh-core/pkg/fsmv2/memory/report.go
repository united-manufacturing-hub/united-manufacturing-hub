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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/memoryhealth"
)

const (
	cgroupReadFailedTag = "memory::cgroup_read_failed"
	hostReadFailedTag   = "memory::host_read_failed"
	zeroTotalTag        = "memory::zero_total"
)

type reportedFailure struct {
	source  string
	outcome string
}

func (d *MemoryDeps) reportFailedReads(ctx context.Context, sample memoryhealth.Sample, readErr error) {
	if ctx.Err() != nil {
		return
	}

	if sample.CgroupReadErr != nil {
		d.reportCgroupReadFailure(sample.CgroupReadErr)
	}

	if sample.HostReadErr != nil {
		d.reportOnce(reportedFailure{source: "host", outcome: string(memoryhealth.ReadFailed)}, hostReadFailedTag, deps.Err(sample.HostReadErr))
	}

	if errors.Is(readErr, memoryhealth.ErrZeroTotal) {
		d.reportOnce(reportedFailure{source: string(sample.Source), outcome: "zero_total"}, zeroTotalTag,
			deps.String("source", string(sample.Source)))
	}
}

func (d *MemoryDeps) reportCgroupReadFailure(err error) {
	var cgroupErr *memoryhealth.CgroupReadError
	if !errors.As(err, &cgroupErr) {
		cgroupErr = &memoryhealth.CgroupReadError{Outcome: memoryhealth.ReadFailed, Err: err}
	}

	d.reportOnce(reportedFailure{source: cgroupErr.File, outcome: string(cgroupErr.Outcome)},
		cgroupReadFailedTag+"::"+string(cgroupErr.Outcome),
		deps.String("file", cgroupErr.File), deps.String("cgroup_base", cgroupBase), deps.Err(cgroupErr.Err))
}

func (d *MemoryDeps) reportOnce(key reportedFailure, message string, fields ...deps.Field) {
	if _, reportedBefore := d.reportedFailures.LoadOrStore(key, struct{}{}); reportedBefore {
		return
	}

	d.GetLogger().SentryWarn(deps.FeatureSupportMemory, d.GetHierarchyPath(), message, fields...)
}
