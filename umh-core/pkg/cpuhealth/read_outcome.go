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
func readRawFile(ctx context.Context, fsys filesystem.Service, path string) (string, ReadOutcome) {
	data, err := fsys.ReadFile(ctx, path)
	if err != nil {
		return "", classifyRead(err)
	}

	return string(data), ReadOK
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

	// The ops below mint no report of their own. They ride on one that does,
	// describing the machine around it, which is how a reader tells a broken
	// mount from a broken file.

	// OpCgroupControllers is the cgroup.controllers read.
	OpCgroupControllers ReadOp = "cgroup_controllers"
	// OpProcSelfCgroup is the /proc/self/cgroup read.
	OpProcSelfCgroup ReadOp = "proc_self_cgroup"
	// OpBaseDir is the base directory listing, kept only as an entry count.
	OpBaseDir ReadOp = "cgroup_base_dir"
)

// readOpSpec pairs one read with the file it opens. underBase marks a file
// that hangs off the sampler's cgroup base; the rest are machine-wide and
// absolute. OpBaseDir opens the base directory itself, so it carries no name.
type readOpSpec struct {
	Op        ReadOp
	name      string
	underBase bool
}

// allReadOps is every read, in the order Read performs them. The DMI reads
// (/sys/class/dmi/id/product_name, sys_vendor) are absent: product_name is
// normally missing in a container, so reporting it would alert on correct
// absence.
var allReadOps = []readOpSpec{
	{Op: OpCgroupControllers, name: "/cgroup.controllers", underBase: true},
	{Op: OpProcSelfCgroup, name: "/proc/self/cgroup"},
	{Op: OpBaseDir, underBase: true},
	{Op: OpCPUPressure, name: "/cpu.pressure", underBase: true},
	{Op: OpCPUStat, name: "/cpu.stat", underBase: true},
	{Op: OpProcStat, name: "/proc/stat"},
	{Op: OpCpusetCPUs, name: "/cpuset.cpus.effective", underBase: true},
	{Op: OpProcCpuinfo, name: "/proc/cpuinfo"},
	{Op: OpCPUMax, name: "/cpu.max", underBase: true},
}

// PathOf returns the file op opens under base, so a reader and a report of
// that read name the same path by construction. base is ignored for a
// machine-wide file. An op with no entry returns "".
func PathOf(base string, op ReadOp) string {
	for _, spec := range allReadOps {
		if spec.Op != op {
			continue
		}

		if spec.underBase {
			return base + spec.name
		}

		return spec.name
	}

	return ""
}

// ReadResult pairs one read with what it produced.
type ReadResult struct {
	Op      ReadOp
	Outcome ReadOutcome
}
