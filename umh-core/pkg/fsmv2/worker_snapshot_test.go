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
		obs := fsmv2.Observation[workerTestStatus]{
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
		mockView := config.NewChildrenView([]config.ChildInfo{
			{Name: "alpha", StateName: "Connected", Phase: config.PhaseRunningHealthy, IsHealthy: true},
			{Name: "beta", StateName: "Connected", Phase: config.PhaseRunningHealthy, IsHealthy: true},
			{Name: "gamma", StateName: "Connected", Phase: config.PhaseRunningHealthy, IsHealthy: true},
			{Name: "delta", StateName: "TryingToConnect", Phase: config.PhaseStarting},
		})
		obs := fsmv2.Observation[workerTestStatus]{
			CollectedAt:       now,
			Status:            workerTestStatus{Reachable: true},
			ParentMappedState: "running",
			LastActionResults: actionResults,
			ChildrenHealthy:   3,
			ChildrenUnhealthy: 1,
			ChildrenView:      mockView,
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
		Expect(snap.ChildrenView).To(Equal(mockView))
		Expect(snap.IsShutdownRequested).To(BeTrue())

		// Parity: nested shape mirrors the deprecated flat aliases. Locks the
		// dual-population invariant before P3.0 deletes the flats.
		Expect(snap.Observed.ParentMappedState).To(Equal(snap.ParentMappedState))
		Expect(snap.Observed.LastActionResults).To(Equal(snap.LastActionResults))
		Expect(snap.Observed.ChildrenView).To(Equal(snap.ChildrenView))
		Expect(snap.Desired.IsShutdownRequested()).To(Equal(snap.IsShutdownRequested))
	})

	It("copies FrameworkMetrics from observed state", func() {
		obs := fsmv2.Observation[workerTestStatus]{
			CollectedAt: now,
			Status:      workerTestStatus{Reachable: true},
		}
		obs.Metrics.Framework = deps.FrameworkMetrics{
			TimeInCurrentStateMs:  12000,
			StateTransitionsTotal: 5,
			StartupCount:          2,
		}

		wds := &fsmv2.WrappedDesiredState[workerTestConfig]{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
		}

		raw := fsmv2.Snapshot{
			Observed: obs,
			Desired:  wds,
			Identity: identity,
		}

		snap := fsmv2.ConvertWorkerSnapshot[workerTestConfig, workerTestStatus](raw)

		Expect(snap.FrameworkMetrics.TimeInCurrentStateMs).To(Equal(int64(12000)))
		Expect(snap.FrameworkMetrics.StateTransitionsTotal).To(Equal(int64(5)))
		Expect(snap.FrameworkMetrics.StartupCount).To(Equal(int64(2)))
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
		}).To(PanicWith(ContainSubstring("Observation")))
	})

	It("panics with descriptive message on wrong desired state type", func() {
		raw := fsmv2.Snapshot{
			Observed: fsmv2.Observation[workerTestStatus]{},
			Desired:  "wrong-type",
			Identity: identity,
		}
		Expect(func() {
			fsmv2.ConvertWorkerSnapshot[workerTestConfig, workerTestStatus](raw)
		}).To(PanicWith(ContainSubstring("WrappedDesiredState")))
	})
})

var _ = Describe("ShouldStop", func() {
	It("returns true when IsShutdownRequested is true", func() {
		snap := fsmv2.WorkerSnapshot[workerTestConfig, workerTestStatus]{
			IsShutdownRequested: true,
		}
		Expect(snap.ShouldStop()).To(BeTrue())
	})

	It("returns true when ParentMappedState is stopped", func() {
		snap := fsmv2.WorkerSnapshot[workerTestConfig, workerTestStatus]{
			ParentMappedState: "stopped",
		}
		Expect(snap.ShouldStop()).To(BeTrue())
	})

	It("returns false when neither condition is met", func() {
		snap := fsmv2.WorkerSnapshot[workerTestConfig, workerTestStatus]{
			ParentMappedState: "running",
		}
		Expect(snap.ShouldStop()).To(BeFalse())
	})

	It("returns false when ParentMappedState is empty (root worker)", func() {
		snap := fsmv2.WorkerSnapshot[workerTestConfig, workerTestStatus]{
			IsShutdownRequested: false,
			ParentMappedState:   "",
		}
		Expect(snap.ShouldStop()).To(BeFalse())
	})

	It("returns true when both conditions are met", func() {
		snap := fsmv2.WorkerSnapshot[workerTestConfig, workerTestStatus]{
			IsShutdownRequested: true,
			ParentMappedState:   "stopped",
		}
		Expect(snap.ShouldStop()).To(BeTrue())
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
