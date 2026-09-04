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

// Read outcomes: the vocabulary for WHY one file read failed. Every reader in
// this package returns its cause — an errno-carrying error from the
// filesystem, or one of the two sentinels here for the failures that have no
// errno — so a caller can report the reason instead of only the fact that a
// read did not work.

package cpuhealth

import (
	"errors"
	"io/fs"
)

// ReadOutcome names the cause of a single file read. It is a string so the
// value can be reported as-is.
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
	// ReadError means the read failed for a reason none of the others name. It
	// is never a guess: an error that does not match a known cause classifies
	// here rather than to the nearest-looking one.
	ReadError ReadOutcome = "error"
	// ReadNotAttempted means no read happened, so there is no outcome to
	// report — a cached sticky fact republished without re-reading its file.
	ReadNotAttempted ReadOutcome = "not_attempted"
)

// The two failures that carry no errno. A zero-byte cpuset and a cpu.pressure
// whose avg60 will not parse both read successfully at the syscall layer, so
// neither has a filesystem error to return, and returning nil would report a
// failed read as a good one.
var (
	errEmptyRead      = errors.New("cpuhealth: file empty")
	errUnparsableRead = errors.New("cpuhealth: content did not parse")
)

// classifyRead names the cause of err. It is total: every error classifies,
// and one it does not recognise classifies as ReadError rather than as the
// closest-looking cause.
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

// ReadOp names one read the sampler performs and reports on. It is a string so
// the value can be reported as-is, and it names the file rather than the
// function, since the file is what an operator would go and look at.
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
	// OpCpusetCPUs is the cgroup's cpuset.cpus.effective read, the CPUs this
	// container may run on.
	OpCpusetCPUs ReadOp = "cpuset_cpus_effective"
)

// allReadOps is every read that is reported on, in the order Read performs
// them.
//
// The two DMI reads (/sys/class/dmi/id/product_name and sys_vendor) are
// deliberately absent. They are excluded from reporting, because a missing
// product_name inside a container is the normal case and an event about it
// would be alerting on correct absence. So there is no outcome to put here:
// recording one would mean either inventing it, or claiming not_attempted for
// a read that did happen.
var allReadOps = []ReadOp{
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
