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

package fsmv2cpu

import (
	"context"
	"io/fs"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// fileMap is a filesystem holding exactly the files in it. A file that is not
// in the map is missing, the way the kernel reports it.
type fileMap struct {
	filesystem.Service

	files map[string]string
}

func (f fileMap) ReadFile(_ context.Context, path string) ([]byte, error) {
	content, ok := f.files[path]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}

	return []byte(content), nil
}

func (f fileMap) FileExists(_ context.Context, path string) (bool, error) {
	_, ok := f.files[path]

	return ok, nil
}

func (fileMap) ReadDir(context.Context, string) ([]os.DirEntry, error) {
	return nil, nil
}

// cgroupV2WithLimit is a cgroup v2 container limited to 2 cores, with every
// file the CPU worker reads.
func cgroupV2WithLimit() map[string]string {
	return map[string]string{
		cgroupBase + "/cgroup.controllers":    "cpuset cpu io memory pids\n",
		"/proc/self/cgroup":                   "0::/\n",
		cgroupBase + "/cpu.pressure":          "some avg10=0.00 avg60=0.00 avg300=0.00 total=0\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n",
		cgroupBase + "/cpu.stat":              "usage_usec 1000\nuser_usec 600\nsystem_usec 400\nnr_periods 10\nnr_throttled 0\nthrottled_usec 0\n",
		"/proc/stat":                          "cpu  1000 0 500 10000 0 0 0 0 0 0\ncpu0 250 0 125 2500 0 0 0 0 0 0\ncpu1 250 0 125 2500 0 0 0 0 0 0\ncpu2 250 0 125 2500 0 0 0 0 0 0\ncpu3 250 0 125 2500 0 0 0 0 0 0\n",
		cgroupBase + "/cpuset.cpus.effective": "0-3\n",
		"/proc/cpuinfo":                       "processor\t: 0\nmodel name\t: test\n\n",
		cgroupBase + "/cpu.max":               "200000 100000\n",
	}
}

func cgroupV2WithoutLimit() map[string]string {
	files := cgroupV2WithLimit()
	files[cgroupBase+"/cpu.max"] = "max 100000\n"

	return files
}

func without(files map[string]string, paths ...string) map[string]string {
	for _, p := range paths {
		delete(files, p)
	}

	return files
}

var _ = Describe("a CPU file the worker cannot read", func() {
	poll := func(files map[string]string) (CPUStatus, error) {
		register.SetDeps[filesystem.Service](FilesystemDepsKey, fileMap{files: files})
		DeferCleanup(register.ClearDeps, FilesystemDepsKey)

		id := deps.Identity{ID: "cpu-unreadable", WorkerType: WorkerType}

		return Poll(context.Background(), NewDeps(id, deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, id)), CPUConfig{})
	}

	It("reads the fixtures as a container limited to 2 cores and one without a limit on 4", func() {
		limited, err := poll(cgroupV2WithLimit())
		Expect(err).NotTo(HaveOccurred())
		Expect(limited.Details.LimitApplies).To(BeTrue())
		Expect(limited.Details.CapacityCores).To(Equal(2.0))

		unlimited, err := poll(cgroupV2WithoutLimit())
		Expect(err).NotTo(HaveOccurred())
		Expect(unlimited.Details.LimitApplies).To(BeFalse())
		Expect(unlimited.Details.CapacityCores).To(Equal(4.0))
	})

	DescribeTable("keeps the same answer on every tick of one worker, after the windows fill",
		func(files map[string]string, wantErr bool) {
			register.SetDeps[filesystem.Service](FilesystemDepsKey, fileMap{files: files})
			DeferCleanup(register.ClearDeps, FilesystemDepsKey)

			id := deps.Identity{ID: "cpu-unreadable-ticks", WorkerType: WorkerType}
			d := NewDeps(id, deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, id))

			for tick := 1; tick <= 4; tick++ {
				_, err := Poll(context.Background(), d, CPUConfig{})
				if wantErr {
					Expect(err).To(HaveOccurred(), "tick %d", tick)
				} else {
					Expect(err).NotTo(HaveOccurred(), "tick %d", tick)
				}
			}
		},
		Entry("cpu.stat missing with a CPU limit", without(cgroupV2WithLimit(), cgroupBase+"/cpu.stat"), true),
		Entry("every file present", cgroupV2WithLimit(), false),
	)

	DescribeTable("fails the poll and names the file, so CPU health reads degraded",
		func(files map[string]string, file string) {
			_, err := poll(files)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(file))
		},
		Entry("cpu.stat missing on a container with a CPU limit",
			without(cgroupV2WithLimit(), cgroupBase+"/cpu.stat"), cgroupBase+"/cpu.stat"),
		Entry("/proc/stat missing on a container without a CPU limit",
			without(cgroupV2WithoutLimit(), "/proc/stat"), "/proc/stat"),
		Entry("cpuset.cpus.effective missing on a container without a CPU limit: nothing else counts the cores",
			without(cgroupV2WithoutLimit(), cgroupBase+"/cpuset.cpus.effective"), cgroupBase+"/cpuset.cpus.effective"),
		Entry("cpuset.cpus.effective and /proc/stat missing on a container without a CPU limit",
			without(cgroupV2WithoutLimit(), cgroupBase+"/cpuset.cpus.effective", "/proc/stat"), "/proc/stat"),
	)

	DescribeTable("stays healthy when every file the verdict needs was read",
		func(files map[string]string) {
			status, err := poll(files)

			Expect(err).NotTo(HaveOccurred())
			Expect(status.Verdict.State).To(Equal(cpuhealth.StateHealthy))
		},
		Entry("every file present, on the first tick while starting up", cgroupV2WithLimit()),
		Entry("cpu.pressure missing: the pressure rule is excused", without(cgroupV2WithLimit(), cgroupBase+"/cpu.pressure")),
		Entry("cpu.stat missing without a CPU limit: /proc/stat measures the machine",
			without(cgroupV2WithoutLimit(), cgroupBase+"/cpu.stat")),
	)
})
