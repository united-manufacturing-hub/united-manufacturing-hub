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

// Read outcomes: the vocabulary for WHY one file read failed. Every reader here
// returns its cause — an errno, or a sentinel below where there is none.

package cpuhealth

import (
	"errors"
	"io/fs"
)

// ReadOutcome names one read's cause, as a string so it reports as-is.
type ReadOutcome string

const (
	// ReadOK means the file was read and its content parsed.
	ReadOK ReadOutcome = "ok"
	// ReadENOENT means the file does not exist.
	ReadENOENT ReadOutcome = "enoent"
	// ReadEACCES means the file exists but could not be opened.
	ReadEACCES ReadOutcome = "eacces"
	// ReadEmpty means the file was read and held nothing.
	ReadEmpty ReadOutcome = "empty"
	// ReadUnparsable means content was present but did not parse.
	ReadUnparsable ReadOutcome = "unparsable"
	// ReadError means no other outcome names the cause — see classifyRead.
	ReadError ReadOutcome = "error"
	// ReadNotAttempted means no read happened: a cached fact, republished.
	ReadNotAttempted ReadOutcome = "not_attempted"
)

// The failures that carry no errno: a zero-byte cpuset, a cpu.pressure whose
// avg60 will not parse. Both succeed at the syscall layer, so returning nil
// would report a failed read as a good one.
var (
	errEmptyRead      = errors.New("cpuhealth: file empty")
	errUnparsableRead = errors.New("cpuhealth: content did not parse")
)

// classifyRead is total: every error classifies, and an unrecognised one
// reaches ReadError rather than the closest-looking cause. Readers hand it the
// error unwrapped, since wrapping hides the errno it reads.
func classifyRead(err error) ReadOutcome {
	switch {
	case err == nil:
		return ReadOK
	case errors.Is(err, fs.ErrNotExist):
		return ReadENOENT
	case errors.Is(err, fs.ErrPermission):
		return ReadEACCES
	case errors.Is(err, errEmptyRead):
		return ReadEmpty
	case errors.Is(err, errUnparsableRead):
		return ReadUnparsable
	default:
		return ReadError
	}
}

// ReadOp names one reported read. The value names the file, not the function,
// because the file is what an operator goes and looks at.
type ReadOp string

const (
	// OpProcStat is the /proc/stat read: the machine's busy, steal and CPU count.
	OpProcStat ReadOp = "proc_stat"
	// OpProcCpuinfo is the /proc/cpuinfo read behind the virtualisation fact.
	OpProcCpuinfo ReadOp = "proc_cpuinfo"
	// OpCPUStat is the cgroup's cpu.stat read: usage and both throttle counters.
	OpCPUStat ReadOp = "cpu_stat"
	// OpCPUMax is the cgroup's cpu.max read, the container's CPU limit.
	OpCPUMax ReadOp = "cpu_max"
	// OpCPUPressure is the cgroup's cpu.pressure read, this tick's PSI fraction.
	OpCPUPressure ReadOp = "cpu_pressure"
	// OpCpusetCPUs is cpuset.cpus.effective: the CPUs this container may use.
	OpCpusetCPUs ReadOp = "cpuset_cpus_effective"

	// Evidence, not measurement: these tell one failure shape from another, and
	// mint no report (see reportedReadOps).

	// OpCgroupControllers is cgroup.controllers: what was delegated here.
	OpCgroupControllers ReadOp = "cgroup_controllers"
	// OpProcSelfCgroup is /proc/self/cgroup: the path this process is in.
	OpProcSelfCgroup ReadOp = "proc_self_cgroup"
	// OpBaseDir is the base directory listing, kept only as an entry count.
	OpBaseDir ReadOp = "cgroup_base_dir"
)

// allReadOps is every reported read, in the order Read performs them. The DMI
// reads (/sys/class/dmi/id/product_name, sys_vendor) are deliberately absent:
// a missing product_name in a container is normal, so an event would alert on
// correct absence.
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
