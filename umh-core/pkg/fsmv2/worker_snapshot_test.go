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

// Tests for WorkerSnapshot construction, ShouldStop semantics, and ExtractConfig.
// ShouldStop covers the 2-way OR:
//
//	ShouldStop() == IsShutdownRequested || IsDisabled

package fsmv2_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// mockDesiredState satisfies fsmv2.DesiredState for the wrong-type panic test.
type mockDesiredState struct{}

func (d *mockDesiredState) IsShutdownRequested() bool { return false }
func (d *mockDesiredState) IsDisabled() bool          { return false }

// workerTestConfig and workerTestStatus are lightweight types used exclusively
// for WorkerSnapshot construction and ShouldStop tests.
type workerTestConfig struct {
	Host string
	Port int
}

type workerTestStatus struct {
	Reachable bool
	LatencyMs int64
}

var _ = Describe("ConvertWorkerSnapshot (happy paths)", func() {
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
			Config: workerTestConfig{Host: "10.0.0.1", Port: 502},
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

	It("copies IsShutdownRequested from WrappedDesiredState", func() {
		obs := fsmv2.Observation[workerTestStatus]{CollectedAt: now}
		wds := &fsmv2.WrappedDesiredState[workerTestConfig]{}
		wds.SetShutdownRequested(true)

		raw := fsmv2.Snapshot{Observed: obs, Desired: wds, Identity: identity}
		snap := fsmv2.ConvertWorkerSnapshot[workerTestConfig, workerTestStatus](raw)

		Expect(snap.IsShutdownRequested).To(BeTrue())
	})

	It("copies framework fields (action results, children counts)", func() {
		actionResults := []deps.ActionResult{
			{ActionType: "connect", Success: true, Timestamp: now},
		}
		obs := fsmv2.Observation[workerTestStatus]{
			CollectedAt:       now,
			Status:            workerTestStatus{Reachable: true},
			LastActionResults: actionResults,
			ChildrenHealthy:   3,
			ChildrenUnhealthy: 1,
		}
		wds := &fsmv2.WrappedDesiredState[workerTestConfig]{}

		raw := fsmv2.Snapshot{Observed: obs, Desired: wds, Identity: identity}
		snap := fsmv2.ConvertWorkerSnapshot[workerTestConfig, workerTestStatus](raw)

		Expect(snap.LastActionResults).To(HaveLen(1))
		Expect(snap.LastActionResults[0].ActionType).To(Equal("connect"))
		Expect(snap.ChildrenHealthy).To(Equal(3))
		Expect(snap.ChildrenUnhealthy).To(Equal(1))
	})

	It("surfaces the desired ChildrenSpecs from WrappedDesiredState", func() {
		children := []config.ChildSpec{
			{Name: "child-a", WorkerType: "test", Enabled: true},
			{Name: "child-b", WorkerType: "test", Enabled: true},
		}
		obs := fsmv2.Observation[workerTestStatus]{CollectedAt: now}
		wds := &fsmv2.WrappedDesiredState[workerTestConfig]{
			ChildrenSpecs: children,
		}

		raw := fsmv2.Snapshot{Observed: obs, Desired: wds, Identity: identity}
		snap := fsmv2.ConvertWorkerSnapshot[workerTestConfig, workerTestStatus](raw)

		Expect(snap.ChildrenSpecs).To(Equal(children))
	})
})

var _ = Describe("ShouldStop", func() {
	// ShouldStop covers 2-way OR: IsShutdownRequested || IsDisabled.

	It("returns false when both signals are absent", func() {
		snap := fsmv2.WorkerSnapshot[workerTestConfig, workerTestStatus]{}
		Expect(snap.ShouldStop()).To(BeFalse())
	})

	It("returns true when IsShutdownRequested is true", func() {
		snap := fsmv2.WorkerSnapshot[workerTestConfig, workerTestStatus]{
			IsShutdownRequested: true,
		}
		Expect(snap.ShouldStop()).To(BeTrue())
	})

	It("returns true when IsDisabled is true (resident disable)", func() {
		snap := fsmv2.WorkerSnapshot[workerTestConfig, workerTestStatus]{
			IsDisabled: true,
		}
		Expect(snap.ShouldStop()).To(BeTrue())
	})

	It("returns true when both signals are present (OR semantics)", func() {
		snap := fsmv2.WorkerSnapshot[workerTestConfig, workerTestStatus]{
			IsShutdownRequested: true,
			IsDisabled:          true,
		}
		Expect(snap.ShouldStop()).To(BeTrue())
	})
})

var _ = Describe("ExtractConfig", func() {
	It("returns typed config from WrappedDesiredState", func() {
		wds := &fsmv2.WrappedDesiredState[workerTestConfig]{
			Config: workerTestConfig{Host: "192.168.1.100", Port: 8080},
		}
		cfg := fsmv2.ExtractConfig[workerTestConfig](wds)
		Expect(cfg.Host).To(Equal("192.168.1.100"))
		Expect(cfg.Port).To(Equal(8080))
	})

	It("panics with descriptive message on wrong desired state type", func() {
		Expect(func() {
			fsmv2.ExtractConfig[workerTestConfig](&mockDesiredState{})
		}).To(PanicWith(ContainSubstring("ExtractConfig")))
	})
})
