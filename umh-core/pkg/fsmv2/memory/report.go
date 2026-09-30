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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
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

func (d *MemoryDeps) reportCgroupReadFailure(err error) {
	var readErr *cgroupReadError
	if !errors.As(err, &readErr) {
		readErr = &cgroupReadError{Outcome: readFailed, Err: err}
	}

	d.reportOnce(reportedFailure{source: readErr.File, outcome: string(readErr.Outcome)},
		cgroupReadFailedTag+"::"+string(readErr.Outcome),
		deps.String("file", readErr.File), deps.String("cgroup_base", cgroupBase), deps.Err(readErr.Err))
}

func (d *MemoryDeps) reportHostReadFailure(ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}

	d.reportOnce(reportedFailure{source: "host", outcome: string(readFailed)}, hostReadFailedTag, deps.Err(err))
}

func (d *MemoryDeps) reportZeroTotal(source MemorySource) {
	d.reportOnce(reportedFailure{source: string(source), outcome: "zero_total"}, zeroTotalTag,
		deps.String("source", string(source)))
}

func (d *MemoryDeps) reportOnce(key reportedFailure, message string, fields ...deps.Field) {
	if _, reportedBefore := d.reportedFailures.LoadOrStore(key, struct{}{}); reportedBefore {
		return
	}

	d.GetLogger().SentryWarn(deps.FeatureSupportMemory, d.GetHierarchyPath(), message, fields...)
}
