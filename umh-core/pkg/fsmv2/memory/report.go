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
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

const cgroupReadFailedTag = "memory::cgroup_read_failed"

type reportedRead struct {
	file    string
	outcome readOutcome
}

func (d *MemoryDeps) reportCgroupReadFailure(err error) {
	var readErr *cgroupReadError
	if !errors.As(err, &readErr) {
		readErr = &cgroupReadError{Outcome: readFailed, Err: err}
	}

	key := reportedRead{file: readErr.File, outcome: readErr.Outcome}
	if _, reportedBefore := d.reportedReads.LoadOrStore(key, struct{}{}); reportedBefore {
		return
	}

	d.GetLogger().SentryWarn(deps.FeatureSupportMemory, d.GetHierarchyPath(),
		cgroupReadFailedTag+"::"+string(readErr.Outcome),
		deps.String("file", readErr.File), deps.String("cgroup_base", cgroupBase), deps.Err(readErr.Err))
}
