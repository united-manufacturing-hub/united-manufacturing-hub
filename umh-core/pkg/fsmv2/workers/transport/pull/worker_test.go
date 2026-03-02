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

package pull_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/state"
)

var _ fsmv2.Worker = (*pull.PullWorker)(nil)

var _ = Describe("PullWorker", func() {
	var (
		worker     *pull.PullWorker
		logger     depspkg.FSMLogger
		identity   depspkg.Identity
		parentDeps *transport.TransportDependencies
	)

	BeforeEach(func() {
		logger = depspkg.NewNopFSMLogger()
		identity = depspkg.Identity{ID: "test-pull", Name: "Test Pull"}
		transport.SetChannelProvider(newTestChannelProvider())
		parentDeps = createParentDeps(logger)
	})

	AfterEach(func() {
		transport.ClearChannelProvider()
	})

	Describe("Compile-time interface check", func() {
		It("should implement fsmv2.Worker interface", func() {
			var _ fsmv2.Worker = (*pull.PullWorker)(nil)
		})
	})

	Describe("NewPullWorker", func() {
		It("should create a worker with valid args", func() {
			var err error
			worker, err = pull.NewPullWorker(identity, logger, nil, parentDeps)
			Expect(err).ToNot(HaveOccurred())
			Expect(worker).NotTo(BeNil())
		})

		It("should reject nil logger", func() {
			_, err := pull.NewPullWorker(identity, nil, nil, parentDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("logger"))
		})

		It("should reject nil parentDeps", func() {
			_, err := pull.NewPullWorker(identity, logger, nil, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CollectObservedState", func() {
		BeforeEach(func() {
			var err error
			worker, err = pull.NewPullWorker(identity, logger, nil, parentDeps)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return valid observed state with timestamp", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx)

			Expect(err).ToNot(HaveOccurred())
			Expect(observed).NotTo(BeNil())
			Expect(observed.GetTimestamp()).NotTo(BeZero())
		})

		It("should handle context cancellation", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err := worker.CollectObservedState(ctx)

			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
		})

		It("should report transport availability from parent deps", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(snapshot.PullObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs.HasTransport).To(BeTrue())
		})

		It("should report JWT token availability from parent deps", func() {
			parentDeps.SetJWT("test-token", time.Now().Add(time.Hour))

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(snapshot.PullObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs.HasValidToken).To(BeTrue())
		})

		It("should report consecutive errors from child deps", func() {
			worker.GetDependencies().RecordError()
			worker.GetDependencies().RecordError()

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(snapshot.PullObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs.ConsecutiveErrors).To(Equal(2))
		})

		It("should report pending message count", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(snapshot.PullObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs.PendingMessageCount).To(Equal(0))
		})

		It("should report backpressure state", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(snapshot.PullObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs.IsBackpressured).To(BeFalse())
		})

		It("should include framework metrics", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(snapshot.PullObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs.Metrics).NotTo(BeNil())
		})

		It("should drain worker metrics from MetricsRecorder into ObservedState", func() {
			d := worker.GetDependencies()
			d.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPulled, 10)
			d.MetricsRecorder().IncrementCounter(depspkg.CounterBytesPulled, 2048)
			d.MetricsRecorder().SetGauge(depspkg.GaugeLastPullLatencyMs, 35.0)

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(snapshot.PullObservedState)
			Expect(ok).To(BeTrue())

			Expect(typedObs.Metrics.Worker.Counters).NotTo(BeNil())
			Expect(typedObs.Metrics.Worker.Counters["messages_pulled"]).To(Equal(int64(10)))
			Expect(typedObs.Metrics.Worker.Counters["bytes_pulled"]).To(Equal(int64(2048)))

			Expect(typedObs.Metrics.Worker.Gauges).NotTo(BeNil())
			Expect(typedObs.Metrics.Worker.Gauges["last_pull_latency_ms"]).To(Equal(35.0))
		})

		It("should drain metrics from recorder buffer on each tick", func() {
			ctx := context.Background()

			d := worker.GetDependencies()
			d.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
			observed1, err := worker.CollectObservedState(ctx)
			Expect(err).ToNot(HaveOccurred())
			typedObs1, ok := observed1.(snapshot.PullObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs1.Metrics.Worker.Counters["pull_ops"]).To(Equal(int64(1)))

			d.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 2)
			observed2, err := worker.CollectObservedState(ctx)
			Expect(err).ToNot(HaveOccurred())
			typedObs2, ok := observed2.(snapshot.PullObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs2.Metrics.Worker.Counters["pull_ops"]).To(Equal(int64(2)))
		})
	})

	Describe("DeriveDesiredState", func() {
		BeforeEach(func() {
			var err error
			worker, err = pull.NewPullWorker(identity, logger, nil, parentDeps)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return running state for nil spec", func() {
			desired, err := worker.DeriveDesiredState(nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired).NotTo(BeNil())
			Expect(desired.GetState()).To(Equal("running"))
		})

		It("should return correct state for valid spec", func() {
			spec := fsmv2config.UserSpec{
				Config:    `state: stopped`,
				Variables: fsmv2config.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired).NotTo(BeNil())
			Expect(desired.GetState()).To(Equal("stopped"))
		})

		It("should return running state for empty config", func() {
			spec := fsmv2config.UserSpec{
				Config:    "",
				Variables: fsmv2config.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired.GetState()).To(Equal("running"))
		})

		It("should be deterministic", func() {
			spec := fsmv2config.UserSpec{
				Config:    `state: running`,
				Variables: fsmv2config.VariableBundle{},
			}

			desired1, err1 := worker.DeriveDesiredState(spec)
			desired2, err2 := worker.DeriveDesiredState(spec)

			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())
			Expect(desired1.GetState()).To(Equal(desired2.GetState()))
		})

		It("should return error for invalid spec type", func() {
			_, err := worker.DeriveDesiredState("invalid-string-spec")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid spec type"))
		})

		It("should return error for malformed YAML", func() {
			spec := fsmv2config.UserSpec{
				Config:    "state: [invalid yaml: {",
				Variables: fsmv2config.VariableBundle{},
			}

			_, err := worker.DeriveDesiredState(spec)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetInitialState", func() {
		BeforeEach(func() {
			var err error
			worker, err = pull.NewPullWorker(identity, logger, nil, parentDeps)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return StoppedState", func() {
			initialState := worker.GetInitialState()

			Expect(initialState).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})
	})

	Describe("Factory registration", func() {
		It("should be registered in the factory as 'pull'", func() {
			registeredTypes := factory.ListRegisteredTypes()
			Expect(registeredTypes).To(ContainElement("pull"))
		})

		It("should have both worker and supervisor factories registered", func() {
			workerOnly, supervisorOnly := factory.ValidateRegistryConsistency()
			Expect(workerOnly).NotTo(ContainElement("pull"))
			Expect(supervisorOnly).NotTo(ContainElement("pull"))
		})
	})
})
