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

package container_monitor_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// cgroupFixture is a filesystem that holds only the files in its map.
type cgroupFixture struct {
	filesystem.Service

	files map[string]string
}

func (f cgroupFixture) ReadFile(_ context.Context, path string) ([]byte, error) {
	content, ok := f.files[path]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}

	return []byte(content), nil
}

func (f cgroupFixture) FileExists(_ context.Context, path string) (bool, error) {
	_, ok := f.files[path]

	return ok, nil
}

func (cgroupFixture) ReadDir(context.Context, string) ([]os.DirEntry, error) {
	return nil, nil
}

func containerLimitedTo2Of4Cores() cgroupFixture {
	return cgroupFixture{files: map[string]string{
		"/sys/fs/cgroup/cgroup.controllers":    "cpuset cpu io memory pids\n",
		"/proc/self/cgroup":                    "0::/\n",
		"/sys/fs/cgroup/cpu.pressure":          "some avg10=0.00 avg60=0.00 avg300=0.00 total=0\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n",
		"/sys/fs/cgroup/cpu.stat":              "usage_usec 1000\nuser_usec 600\nsystem_usec 400\nnr_periods 10\nnr_throttled 0\nthrottled_usec 0\n",
		"/proc/stat":                           "cpu  1000 0 500 10000 0 0 0 0 0 0\ncpu0 250 0 125 2500 0 0 0 0 0 0\ncpu1 250 0 125 2500 0 0 0 0 0 0\ncpu2 250 0 125 2500 0 0 0 0 0 0\ncpu3 250 0 125 2500 0 0 0 0 0 0\n",
		"/sys/fs/cgroup/cpuset.cpus.effective": "0-3\n",
		"/proc/cpuinfo":                        "processor\t: 0\nmodel name\t: test\n\n",
		"/sys/fs/cgroup/cpu.max":               "200000 100000\n",
	}}
}

var _ = Describe("CollectCPUFromWorker while the CPU worker is starting up", func() {
	It("reports CPU degraded with the starting-up message and sends no cpuHealth key until the usage is measured", func() {
		register.SetDeps[filesystem.Service](fsmv2cpu.FilesystemDepsKey, containerLimitedTo2Of4Cores())
		DeferCleanup(register.ClearDeps, fsmv2cpu.FilesystemDepsKey)

		// The real registered worker, so the stored status is the one the
		// framework writes: Poll, then the worker's Health function.
		id := deps.Identity{ID: "cpu-starting-up", WorkerType: fsmv2cpu.WorkerType}
		worker, err := factory.NewWorkerByType(fsmv2cpu.WorkerType, id, deps.NewNopFSMLogger(), nil, nil)
		Expect(err).NotTo(HaveOccurred())

		stub := &cpuStubStateReader{}
		publishCPUClient(stub)

		service := container_monitor.NewContainerMonitorServiceWithPath(filesystem.NewMockFileSystem(), GinkgoT().TempDir())

		poll := func() (fsmv2cpu.CPUStatus, *models.CPU) {
			observed, err := worker.CollectObservedState(context.Background(), &fsmv2.WrappedDesiredState[fsmv2cpu.CPUConfig]{})
			Expect(err).NotTo(HaveOccurred())

			obs, ok := observed.(fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]])
			Expect(ok).To(BeTrue())
			Expect(obs.Status.Reason).NotTo(HavePrefix("poll error"), "this spec needs every poll to read the fixture")

			// GetFresh judges freshness by CollectedAt, which the collector
			// normally stamps.
			obs.CollectedAt = time.Now()
			stub.obs = &obs

			cpu, err := service.CollectCPUFromWorker(context.Background())
			Expect(err).NotTo(HaveOccurred())

			return obs.Status.Result, cpu
		}

		result, cpu := poll()
		Expect(result.Details.UsageRingActive).To(BeFalse(), "the first poll after a start has not measured the usage")

		for polls := 1; !result.Details.UsageRingActive; polls++ {
			Expect(polls).To(BeNumerically("<", 3), "the usage must be measured within three polls")

			Expect(result.Verdict.State == cpuhealth.StateDegraded && len(result.Verdict.Causes) == 0).To(BeFalse(),
				"the stored verdict must not be degraded without a cause")

			Expect(cpu.Health.Category).To(Equal(models.Degraded))
			Expect(cpu.Health.Message).To(HavePrefix("CPU: starting up."))
			Expect(cpu.CPUHealth).To(BeNil())

			data, err := json.Marshal(cpu)
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]any
			Expect(json.Unmarshal(data, &raw)).To(Succeed())
			Expect(raw).NotTo(HaveKey("cpuHealth"))

			result, cpu = poll()
		}

		// Once the usage is measured, CollectCPUFromWorker sends the verdict.
		// So the missing cpuHealth key above comes from the unmeasured usage,
		// not from the test setup.
		Expect(cpu.Health.Category).To(Equal(models.Active))
		Expect(cpu.CPUHealth).NotTo(BeNil())
	})
})
