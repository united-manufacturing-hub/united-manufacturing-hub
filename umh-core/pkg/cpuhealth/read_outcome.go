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
	"errors"
	"fmt"
	"io/fs"
)

// ReadOutcome names one read's cause.
type ReadOutcome string

const (
	// ReadOK means the file was read and its content parsed.
	ReadOK ReadOutcome = "ok"
	// ReadMissing means the file does not exist (ENOENT).
	ReadMissing ReadOutcome = "missing"
	// ReadPermissionDenied means the file exists but could not be opened
	// (EACCES).
	ReadPermissionDenied ReadOutcome = "permission_denied"
	// ReadEmpty means the file was read and held nothing.
	ReadEmpty ReadOutcome = "empty"
	// ReadUnparsable means content was present but did not parse.
	ReadUnparsable ReadOutcome = "unparsable"
	// ReadError means no other outcome names the cause.
	ReadError ReadOutcome = "error"
	// ReadNotAttempted means no read happened.
	ReadNotAttempted ReadOutcome = "not_attempted"
)

// A read can fail with no errno: the syscall succeeds and the content is
// unusable. Returning nil there would report a failed read as a good one.
var (
	errEmptyRead      = errors.New("cpuhealth: file empty")
	errUnparsableRead = errors.New("cpuhealth: content did not parse")
	// errNoPressureFile is what a hierarchy with no per-cgroup pressure file
	// returns. It wraps fs.ErrNotExist so it classifies as ReadMissing, the
	// cause a kernel without PSI already produces, which reports nothing.
	errNoPressureFile = fmt.Errorf("cpuhealth: hierarchy publishes no cpu.pressure: %w", fs.ErrNotExist)
)

// classifyRead maps an unrecognised error to ReadError rather than to the
// closest-looking cause, so nothing arrives at a report misattributed.
func classifyRead(err error) ReadOutcome {
	switch {
	case err == nil:
		return ReadOK
	case errors.Is(err, fs.ErrNotExist):
		return ReadMissing
	case errors.Is(err, fs.ErrPermission):
		return ReadPermissionDenied
	case errors.Is(err, errEmptyRead):
		return ReadEmpty
	case errors.Is(err, errUnparsableRead):
		return ReadUnparsable
	default:
		return ReadError
	}
}

// ReadOp names one reported read by its file, not by the function reading it.
type ReadOp string

const (
	// OpProcStat is the /proc/stat read.
	OpProcStat ReadOp = "proc_stat"
	// OpProcCpuinfo is the /proc/cpuinfo read.
	OpProcCpuinfo ReadOp = "proc_cpuinfo"
	// OpCPUStat is the cgroup's cpu.stat read.
	OpCPUStat ReadOp = "cpu_stat"
	// OpCPUMax is the cgroup's cpu.max read.
	OpCPUMax ReadOp = "cpu_max"
	// OpCPUPressure is the cgroup's cpu.pressure read.
	OpCPUPressure ReadOp = "cpu_pressure"
	// OpCpusetCPUs is the cgroup's cpuset.cpus.effective read.
	OpCpusetCPUs ReadOp = "cpuset_cpus_effective"

	// The three below are evidence: they ride on a report and mint none of
	// their own, so reportedReadOps omits them.

	// OpCgroupControllers is the cgroup.controllers read.
	OpCgroupControllers ReadOp = "cgroup_controllers"
	// OpProcSelfCgroup is the /proc/self/cgroup read.
	OpProcSelfCgroup ReadOp = "proc_self_cgroup"
	// OpBaseDir is the base directory listing, kept only as an entry count.
	OpBaseDir ReadOp = "cgroup_base_dir"
)

// allReadOps is every read, in the order Read performs them. The DMI reads
// (/sys/class/dmi/id/product_name, sys_vendor) are absent: product_name is
// normally missing in a container, so reporting it would alert on correct
// absence.
var allReadOps = []ReadOp{
	OpCgroupControllers,
	OpProcSelfCgroup,
	OpBaseDir,
	OpCPUPressure,
	OpCPUStat,
	OpProcStat,
	OpCpusetCPUs,
	OpProcCpuinfo,
	OpCPUMax,
}

// ReadResult pairs one read with what it produced.
type ReadResult struct {
	Op      ReadOp
	Outcome ReadOutcome
}
