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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// publishCPUClient publishes a real fsmv2 client whose store serves stub. With
// registered false, the CPU worker is missing from the client's registry.
func publishCPUClient(stub *cpuStubStateReader, registered bool) {
	writer := dynamicchildren.NewWriter()
	if registered {
		Expect(writer.Upsert(fsmv2cpu.Ref, map[string]any{})).To(Succeed())
	}

	previous := fsmv2client.GetClient()

	fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, stub))
	DeferCleanup(func() { fsmv2client.SetClient(previous) })
}

// observedNow is one observation of status, collected just now so that it
// counts as fresh.
func observedNow(status simple.Status[fsmv2cpu.CPUStatus]) *cpuStubStateReader {
	return &cpuStubStateReader{obs: &fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
		CollectedAt: time.Now(),
		Status:      status,
	}}
}

// A healthy verdict is staged wherever a row has room for one. No row may
// report it, so a branch that starts trusting it turns its row healthy and
// fails the table. Each row also names its case in the message it expects, so
// a row that reaches a different degraded branch fails too.
var _ = Describe("the CPU seam without a usable reading", func() {
	DescribeTable("reports degraded, with no measurement attached",
		func(stage func(), messageNamesCase string) {
			stage()
			container_monitor.ResetCPUWorkerNeverStartedWarning()

			service := container_monitor.NewContainerMonitorServiceWithPath(filesystem.NewMockFileSystem(), GinkgoT().TempDir())

			health, cpuHealth, err := service.ReadWorkerCPUHealth(context.Background())

			Expect(err).NotTo(HaveOccurred())
			Expect(health).NotTo(BeNil())
			Expect(health.Category).To(Equal(models.Degraded))
			Expect(health.ObservedState).To(Equal(models.Degraded.String()))
			Expect(health.Message).To(ContainSubstring(messageNamesCase))
			Expect(cpuHealth).To(BeNil())
		},
		Entry("no worker client", func() {
			previous := fsmv2client.GetClient()

			fsmv2client.SetClient(nil)
			DeferCleanup(func() { fsmv2client.SetClient(previous) })
		}, "no fsmv2 client is reachable"),
		Entry("no reading yet", func() {
			publishCPUClient(&cpuStubStateReader{err: persistence.ErrNotFound}, true)
		}, "never observed"),
		Entry("a stale reading", func() {
			stale := observedNow(healthyWorkerStatus())
			stale.obs.CollectedAt = time.Now().Add(-4 * fsmv2cpu.PollInterval)
			publishCPUClient(stale, true)
		}, "stale"),
		Entry("an unregistered worker", func() {
			publishCPUClient(observedNow(healthyWorkerStatus()), false)
		}, "not registered"),
		Entry("a read error", func() {
			publishCPUClient(&cpuStubStateReader{err: errors.New("store read failed")}, true)
		}, "store read failed"),
		Entry("a failed poll", func() {
			publishCPUClient(observedNow(simple.Status[fsmv2cpu.CPUStatus]{
				Degraded: true,
				Reason:   "poll error: read cpu.stat: permission denied",
			}), true)
		}, "permission denied"),
		Entry("a timed-out poll that kept a healthy partial result", func() {
			publishCPUClient(observedNow(simple.Status[fsmv2cpu.CPUStatus]{
				Result:   healthyWorkerStatus().Result,
				Degraded: true,
				Reason:   "poll error: " + context.DeadlineExceeded.Error(),
			}), true)
		}, "deadline exceeded"),
		Entry("a poll without a verdict", func() {
			publishCPUClient(observedNow(simple.Status[fsmv2cpu.CPUStatus]{
				Result: fsmv2cpu.CPUStatus{Verdict: cpuhealth.Verdict{}},
			}), true)
		}, "no verdict"),
	)
})
