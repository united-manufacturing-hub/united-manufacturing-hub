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

package fsmv2_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

var _ = Describe("ConvertWorkerSnapshot", func() {
	var (
		identity deps.Identity
		now      time.Time
	)

	BeforeEach(func() {
		identity = deps.Identity{
			ID:         "worker-1",
			Name:       "test-worker",
			WorkerType: "test",
		}
		now = time.Now()
	})

	It("extracts typed Config and Status from a raw Snapshot", func() {
		obs := fsmv2.WrappedObservedState[workerTestStatus]{
			CollectedAt: now,
			Status:      workerTestStatus{Reachable: true, LatencyMs: 42},
		}

		wds := &fsmv2.WrappedDesiredState[workerTestConfig]{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
			Config:           workerTestConfig{Host: "10.0.0.1", Port: 502},
		}

		raw := fsmv2.Snapshot{
			Observed: obs,
			Desired:  wds,
			Identity: identity,
		}

		snap := fsmv2.ConvertWorkerSnapshot[workerTestConfig, workerTestStatus](raw)

		Expect(snap.Config.Host).To(Equal("10.0.0.1"))
		Expect(snap.Config.Port).To(Equal(502))
		Expect(snap.Status.Reachable).To(BeTrue())
		Expect(snap.Status.LatencyMs).To(Equal(int64(42)))
		Expect(snap.Identity).To(Equal(identity))
		Expect(snap.CollectedAt).To(Equal(now))
		Expect(snap.IsShutdownRequested).To(BeFalse())
	})

	It("copies framework fields from observed state", func() {
		actionResults := []deps.ActionResult{
			{ActionType: "connect", Success: true, Timestamp: now},
		}
		obs := fsmv2.WrappedObservedState[workerTestStatus]{
			CollectedAt:       now,
			Status:            workerTestStatus{Reachable: true},
			ParentMappedState: "running",
			LastActionResults: actionResults,
			ChildrenHealthy:   3,
			ChildrenUnhealthy: 1,
			ChildrenView:      "mock-view",
		}

		wds := &fsmv2.WrappedDesiredState[workerTestConfig]{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
		}
		wds.SetShutdownRequested(true)

		raw := fsmv2.Snapshot{
			Observed: obs,
			Desired:  wds,
			Identity: identity,
		}

		snap := fsmv2.ConvertWorkerSnapshot[workerTestConfig, workerTestStatus](raw)

		Expect(snap.ParentMappedState).To(Equal("running"))
		Expect(snap.LastActionResults).To(HaveLen(1))
		Expect(snap.LastActionResults[0].ActionType).To(Equal("connect"))
		Expect(snap.ChildrenHealthy).To(Equal(3))
		Expect(snap.ChildrenUnhealthy).To(Equal(1))
		Expect(snap.ChildrenView).To(Equal("mock-view"))
		Expect(snap.IsShutdownRequested).To(BeTrue())
	})

	It("panics with descriptive message on non-Snapshot input", func() {
		Expect(func() {
			fsmv2.ConvertWorkerSnapshot[workerTestConfig, workerTestStatus]("not-a-snapshot")
		}).To(PanicWith(ContainSubstring("expected fsmv2.Snapshot")))
	})

	It("panics with descriptive message on wrong observed state type", func() {
		raw := fsmv2.Snapshot{
			Observed: "wrong-type",
			Desired:  &fsmv2.WrappedDesiredState[workerTestConfig]{},
			Identity: identity,
		}
		Expect(func() {
			fsmv2.ConvertWorkerSnapshot[workerTestConfig, workerTestStatus](raw)
		}).To(PanicWith(ContainSubstring("WrappedObservedState")))
	})

	It("panics with descriptive message on wrong desired state type", func() {
		raw := fsmv2.Snapshot{
			Observed: fsmv2.WrappedObservedState[workerTestStatus]{},
			Desired:  "wrong-type",
			Identity: identity,
		}
		Expect(func() {
			fsmv2.ConvertWorkerSnapshot[workerTestConfig, workerTestStatus](raw)
		}).To(PanicWith(ContainSubstring("WrappedDesiredState")))
	})
})

var _ = Describe("ExtractConfig", func() {
	It("returns typed config from WrappedDesiredState", func() {
		wds := &fsmv2.WrappedDesiredState[workerTestConfig]{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
			Config:           workerTestConfig{Host: "192.168.1.100", Port: 8080},
		}

		cfg := fsmv2.ExtractConfig[workerTestConfig](wds)
		Expect(cfg.Host).To(Equal("192.168.1.100"))
		Expect(cfg.Port).To(Equal(8080))
	})

	It("panics with descriptive message on wrong type", func() {
		Expect(func() {
			fsmv2.ExtractConfig[workerTestConfig](&mockDesiredState{})
		}).To(PanicWith(ContainSubstring("WrappedDesiredState")))
	})
})
