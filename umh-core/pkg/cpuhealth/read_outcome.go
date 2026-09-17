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

// Which file each read opens (ReadOperation, PathOf), and how it ended (ReadOutcome).
// Every read on a Sample carries an outcome, though a reader may return an
// error that classifyRead turns into one. The fsmv2 CPU worker reports the
// failures to Sentry.

package cpuhealth

import (
	"context"
	"errors"
	"io/fs"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
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

// readRawFile returns a file's text verbatim, with no parsing. Both sources
// use it, so it takes the filesystem rather than hanging off either one.
//
// A file that exists and holds nothing reads ReadEmpty, not ReadOK, which is
// what readQuota, readCpuset and readPSI report for the same case.
func readRawFile(ctx context.Context, fsys filesystem.Service, path string) (string, ReadOutcome) {
	data, err := fsys.ReadFile(ctx, path)
	if err != nil {
		return "", classifyRead(err)
	}

	if len(data) == 0 {
		return "", ReadEmpty
	}

	return string(data), ReadOK
}

// ReadOperation names one reported read by its file, not by the function
// reading it.
type ReadOperation string

const (
	// OperationProcStat is the /proc/stat read.
	OperationProcStat ReadOperation = "proc_stat"
	// OperationProcCpuinfo is the /proc/cpuinfo read.
	OperationProcCpuinfo ReadOperation = "proc_cpuinfo"
	// OperationCPUStat is the cgroup's cpu.stat read.
	OperationCPUStat ReadOperation = "cpu_stat"
	// OperationCPUMax is the cgroup's cpu.max read.
	OperationCPUMax ReadOperation = "cpu_max"
	// OperationCPUPressure is the cgroup's cpu.pressure read.
	OperationCPUPressure ReadOperation = "cpu_pressure"
	// OperationCpusetCPUs is the cgroup's cpuset.cpus.effective read.
	OperationCpusetCPUs ReadOperation = "cpuset_cpus_effective"

	// The operations below get no Sentry event of their own. They travel as
	// fields on one that does, describing the machine around it, which is how a
	// reader tells a broken mount from a broken file.

	// OperationCgroupControllers is the cgroup.controllers read.
	OperationCgroupControllers ReadOperation = "cgroup_controllers"
	// OperationProcSelfCgroup is the /proc/self/cgroup read.
	OperationProcSelfCgroup ReadOperation = "proc_self_cgroup"
	// OperationCgroupBaseDir is the base directory listing, kept only as an
	// entry count.
	OperationCgroupBaseDir ReadOperation = "cgroup_base_dir"
)

// readOperationSpec pairs one read with the file it opens. A cgroupRelative
// file's name is appended to the sampler's cgroup base; every other name is an
// absolute machine-wide path. OperationCgroupBaseDir opens the base directory
// itself, so it carries no name.
type readOperationSpec struct {
	Operation      ReadOperation
	name           string
	cgroupRelative bool
}

// allReadOperations is every read, in the order Read performs them. The DMI reads
// (/sys/class/dmi/id/product_name, sys_vendor) are absent: product_name is
// normally missing in a container, so reporting it would alert on correct
// absence.
var allReadOperations = []readOperationSpec{
	{Operation: OperationCgroupControllers, name: "/cgroup.controllers", cgroupRelative: true},
	{Operation: OperationProcSelfCgroup, name: "/proc/self/cgroup"},
	{Operation: OperationCgroupBaseDir, cgroupRelative: true},
	{Operation: OperationCPUPressure, name: "/cpu.pressure", cgroupRelative: true},
	{Operation: OperationCPUStat, name: "/cpu.stat", cgroupRelative: true},
	{Operation: OperationProcStat, name: "/proc/stat"},
	{Operation: OperationCpusetCPUs, name: "/cpuset.cpus.effective", cgroupRelative: true},
	{Operation: OperationProcCpuinfo, name: "/proc/cpuinfo"},
	{Operation: OperationCPUMax, name: "/cpu.max", cgroupRelative: true},
}

// PathOf returns the file this operation opens under base, so a reader and a
// report of that read name the same path by construction. base is ignored for a
// machine-wide file. An operation with no entry returns "".
func PathOf(base string, operation ReadOperation) string {
	for _, spec := range allReadOperations {
		if spec.Operation != operation {
			continue
		}

		if spec.cgroupRelative {
			return base + spec.name
		}

		return spec.name
	}

	return ""
}

// ReadResult pairs one read with what it produced.
type ReadResult struct {
	Operation ReadOperation
	Outcome   ReadOutcome
}
