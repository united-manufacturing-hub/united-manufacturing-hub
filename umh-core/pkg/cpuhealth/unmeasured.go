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

package cpuhealth

import (
	"fmt"
	"slices"
)

// CPUNotMeasuredError returns an error naming the file that could not be read,
// when that file left the CPU capacity or the CPU usage missing. It returns nil
// when no read failed. Right after a start, usage is missing because too few
// polls have run; that is not an error.
func CPUNotMeasuredError(s Sample, d Details) error {
	if d.CapacityCores == 0 {
		if err := firstFailedRead(s, OperationCpusetCPUs, OperationProcStat); err != nil {
			return err
		}
	}

	if !UsageMeasured(d) {
		usageReads := []ReadOperation{OperationProcStat}
		if d.LimitApplies {
			usageReads = []ReadOperation{OperationCPUStat, OperationCPUAcctUsage}
		}

		return firstFailedRead(s, usageReads...)
	}

	return nil
}

func firstFailedRead(s Sample, operations ...ReadOperation) error {
	for _, read := range s.Troubleshooting.Reads {
		if !slices.Contains(operations, read.Operation) || read.Outcome == ReadOK || read.Outcome == ReadNotAttempted {
			continue
		}

		if err := s.Troubleshooting.ReadErrors[read.Operation]; err != nil {
			return fmt.Errorf("CPU not measured: %w", err)
		}

		return fmt.Errorf("CPU not measured: cannot read %s (%s)", s.Troubleshooting.ReadPaths[read.Operation], read.Outcome)
	}

	return nil
}
