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

package communicator_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// MockStateReader implements deps.StateReader for testing.
type MockStateReader struct {
	mu    sync.RWMutex
	store map[string]interface{}
}

func NewMockStateReader() *MockStateReader {
	return &MockStateReader{
		store: make(map[string]interface{}),
	}
}

func (m *MockStateReader) LoadObservedTyped(_ context.Context, workerType, id string, result interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := workerType + "/" + id
	if stored, ok := m.store[key]; ok {
		data, err := json.Marshal(stored)
		if err != nil {
			return err
		}

		return json.Unmarshal(data, result)
	}

	return nil
}

// SaveObserved saves the observed state for later retrieval.
func (m *MockStateReader) SaveObserved(workerType, id string, observed interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := workerType + "/" + id
	m.store[key] = observed
}

func TestCommunicator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Communicator Suite")
}

var _ = Describe("CommunicatorWorker", func() {
	var (
		worker        *communicator.CommunicatorWorker
		ctx           context.Context
		mockTransport *MockTransport
		logger        depspkg.FSMLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = depspkg.NewNopFSMLogger()
		mockTransport = NewMockTransport()

		communicator.SetChannelProvider(NewMockChannelProvider())

		var err error
		worker, err = communicator.NewCommunicatorWorker(
			depspkg.Identity{ID: "test-id", Name: "Test Communicator", WorkerType: "communicator"},
			mockTransport,
			logger,
			nil,
			nil,
		)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		communicator.ClearChannelProvider()
	})

	Describe("Worker interface implementation", func() {
		It("should create a new CommunicatorWorker with channels", func() {
			Expect(worker).NotTo(BeNil())
		})
	})

	Describe("GetInitialState", func() {
		It("should return StoppedState", func() {
			initialState := worker.GetInitialState()
			Expect(initialState).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})
	})

	Describe("DeriveDesiredState", func() {
		Context("with nil spec", func() {
			It("should return default WrappedDesiredState with running state", func() {
				desiredIface, err := worker.DeriveDesiredState(nil)
				Expect(err).NotTo(HaveOccurred())

				desired, ok := desiredIface.(*fsmv2.WrappedDesiredState[communicator.CommunicatorConfig])
				Expect(ok).To(BeTrue(), "expected *fsmv2.WrappedDesiredState[CommunicatorConfig]")
				Expect(desired.IsShutdownRequested()).To(BeFalse())
			})

			It("should not populate ChildrenSpecs — transport child is declared in RenderChildren", func() {
				desiredIface, err := worker.DeriveDesiredState(nil)
				Expect(err).NotTo(HaveOccurred())

				desired := desiredIface.(*fsmv2.WrappedDesiredState[communicator.CommunicatorConfig])
				Expect(desired.GetChildrenSpecs()).To(BeNil())
			})
		})

		Context("with valid UserSpec", func() {
			It("should return typed WrappedDesiredState with all fields populated", func() {
				spec := fsmv2types.UserSpec{
					Config: `
relayURL: "https://relay.umh.app"
instanceUUID: "test-uuid-12345"
authToken: "test-auth-token-secret"
timeout: 15s
state: "running"
`,
				}

				desiredIface, err := worker.DeriveDesiredState(spec)
				Expect(err).NotTo(HaveOccurred())

				desired, ok := desiredIface.(*fsmv2.WrappedDesiredState[communicator.CommunicatorConfig])
				Expect(ok).To(BeTrue(), "expected *fsmv2.WrappedDesiredState[CommunicatorConfig]")

				Expect(desired.Config.RelayURL).To(Equal("https://relay.umh.app"))
				Expect(desired.Config.InstanceUUID).To(Equal("test-uuid-12345"))
				Expect(desired.Config.AuthToken).To(Equal("test-auth-token-secret"))
				Expect(desired.Config.Timeout).To(Equal(15 * time.Second))
				// ChildrenSpecs is nil: transport child is declared in RenderChildren (children.go),
				// not in DeriveDesiredState. RenderChildren is the single source of truth.
				Expect(desired.GetChildrenSpecs()).To(BeNil())
			})

			It("should apply default timeout when not specified", func() {
				spec := fsmv2types.UserSpec{
					Config: `
relayURL: "https://relay.umh.app"
instanceUUID: "test-uuid"
authToken: "test-token"
`,
				}

				desiredIface, err := worker.DeriveDesiredState(spec)
				Expect(err).NotTo(HaveOccurred())

				desired := desiredIface.(*fsmv2.WrappedDesiredState[communicator.CommunicatorConfig])
				Expect(desired.Config.Timeout).To(Equal(httpTransport.LongPollingDuration + httpTransport.LongPollingBuffer))
			})
		})

		Context("type assertion and roundtrip", func() {
			It("should preserve typed fields through marshal/unmarshal roundtrip", func() {
				spec := fsmv2types.UserSpec{
					Config: `
relayURL: "https://relay.example.com"
instanceUUID: "roundtrip-uuid-test"
authToken: "roundtrip-auth-token"
timeout: 30s
state: "running"
`,
				}

				desiredIface, err := worker.DeriveDesiredState(spec)
				Expect(err).NotTo(HaveOccurred())

				originalDesired, ok := desiredIface.(*fsmv2.WrappedDesiredState[communicator.CommunicatorConfig])
				Expect(ok).To(BeTrue())

				jsonBytes, err := json.Marshal(originalDesired)
				Expect(err).NotTo(HaveOccurred())

				var loadedDesired fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]
				err = json.Unmarshal(jsonBytes, &loadedDesired)
				Expect(err).NotTo(HaveOccurred())

				Expect(loadedDesired.Config.RelayURL).To(Equal("https://relay.example.com"))
				Expect(loadedDesired.Config.InstanceUUID).To(Equal("roundtrip-uuid-test"))
				Expect(loadedDesired.Config.AuthToken).To(Equal("roundtrip-auth-token"))
				Expect(loadedDesired.Config.Timeout).To(Equal(30 * time.Second))
			})

			It("should return clear error on invalid spec type", func() {
				_, err := worker.DeriveDesiredState("invalid-string-spec")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid spec type"))
			})

			It("should return error on invalid YAML config", func() {
				spec := fsmv2types.UserSpec{
					Config: `invalid: yaml: [missing bracket`,
				}

				_, err := worker.DeriveDesiredState(spec)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("config parse failed"))
			})
		})
	})

	Describe("CollectObservedState", func() {
		It("should return Observation with zero CollectedAt (collector sets it)", func() {
			observed, err := worker.CollectObservedState(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())

			communicatorObserved, ok := observed.(fsmv2.Observation[communicator.CommunicatorStatus])
			Expect(ok).To(BeTrue(), "expected fsmv2.Observation[CommunicatorStatus]")
			Expect(communicatorObserved.CollectedAt).To(BeZero(), "NewObservation leaves CollectedAt zero; collector fills it")
		})

		It("should set consecutive errors gauge on MetricsRecorder", func() {
			d := worker.GetDependencies()
			d.RecordError()
			d.RecordError()

			_, err := worker.CollectObservedState(ctx, nil)
			Expect(err).NotTo(HaveOccurred())

			drained := d.MetricsRecorder().Drain()
			Expect(drained.Gauges[string(depspkg.GaugeConsecutiveErrors)]).To(Equal(float64(2)))
		})

		It("should not drain MetricsRecorder into observation (collector handles accumulation)", func() {
			d := worker.GetDependencies()
			d.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
			d.MetricsRecorder().SetGauge(depspkg.GaugeLastPullLatencyMs, 100.0)

			observed, err := worker.CollectObservedState(ctx, nil)
			Expect(err).NotTo(HaveOccurred())

			communicatorObserved, ok := observed.(fsmv2.Observation[communicator.CommunicatorStatus])
			Expect(ok).To(BeTrue())
			Expect(communicatorObserved.Metrics.Worker.Counters).To(BeEmpty(),
				"NewObservation does not drain MetricsRecorder; collector handles accumulation")
		})

		It("should return DegradedEnteredAt in status when errors recorded", func() {
			d := worker.GetDependencies()
			d.RecordError()

			observed, err := worker.CollectObservedState(ctx, nil)
			Expect(err).NotTo(HaveOccurred())

			communicatorObserved, ok := observed.(fsmv2.Observation[communicator.CommunicatorStatus])
			Expect(ok).To(BeTrue())
			Expect(communicatorObserved.Status.DegradedEnteredAt).NotTo(BeZero(),
				"DegradedEnteredAt should be set once errors are recorded")
		})
	})
})

// countingChannelProvider implements communicator.ChannelProvider and counts
// how often each of its methods is called, so a test can tell which of two
// providers the worker actually used. The fixed-value test providers cannot
// show that: they return the same values whichever provider is asked.
type countingChannelProvider struct {
	getChannelsCalls     int
	getInboundStatsCalls int
}

func newCountingChannelProvider() *countingChannelProvider {
	return &countingChannelProvider{}
}

func (p *countingChannelProvider) GetChannels(_ string) (
	inbound chan<- *types.UMHMessage,
	outbound <-chan *types.UMHMessage,
) {
	p.getChannelsCalls++

	return make(chan *types.UMHMessage, 1), make(chan *types.UMHMessage, 1)
}

func (p *countingChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	p.getInboundStatsCalls++

	return 1, 0
}

var _ = Describe("CommunicatorWorker channel provider dependency", func() {
	It("uses the provider from the dependency map over the global one", func() {
		globalProvider := newCountingChannelProvider()
		mapProvider := newCountingChannelProvider()

		previous := communicator.GetChannelProvider()

		communicator.SetChannelProvider(globalProvider)
		DeferCleanup(func() { communicator.SetChannelProvider(previous) })

		dependencyMap := map[string]any{}

		var mapProviderAsProvider communicator.ChannelProvider = mapProvider
		fsmv2types.SetDependency(dependencyMap, communicator.ChannelProviderKey, mapProviderAsProvider)

		identity := depspkg.Identity{ID: "map-provider-worker", WorkerType: "communicator"}
		built, err := factory.NewWorkerByType("communicator", identity, depspkg.NewNopFSMLogger(), nil, dependencyMap)
		Expect(err).NotTo(HaveOccurred())

		commWorker, ok := built.(*communicator.CommunicatorWorker)
		Expect(ok).To(BeTrue(), "expected *communicator.CommunicatorWorker, got %T", built)

		workerDeps := commWorker.GetDependencies()

		_, _ = workerDeps.GetInboundChanStats()

		Expect(mapProvider.getChannelsCalls).To(Equal(1),
			"the construction-time reader should take its channels from the map's provider")
		Expect(mapProvider.getInboundStatsCalls).To(Equal(1),
			"the inbound-stats reader should read through the map's provider")
		Expect(globalProvider.getChannelsCalls).To(Equal(0),
			"the construction-time reader must not read the global provider")
		Expect(globalProvider.getInboundStatsCalls).To(Equal(0),
			"the inbound-stats reader must not read the global provider")

		Expect(communicator.GetChannelProvider()).To(BeIdenticalTo(globalProvider))
	})

	It("falls back to the global provider when the dependency map holds none", func() {
		globalProvider := newCountingChannelProvider()

		previous := communicator.GetChannelProvider()

		communicator.SetChannelProvider(globalProvider)
		DeferCleanup(func() { communicator.SetChannelProvider(previous) })

		identity := depspkg.Identity{ID: "global-provider-worker", WorkerType: "communicator"}
		built, err := factory.NewWorkerByType("communicator", identity, depspkg.NewNopFSMLogger(), nil, nil)
		Expect(err).NotTo(HaveOccurred())

		commWorker, ok := built.(*communicator.CommunicatorWorker)
		Expect(ok).To(BeTrue(), "expected *communicator.CommunicatorWorker, got %T", built)

		workerDeps := commWorker.GetDependencies()

		_, _ = workerDeps.GetInboundChanStats()

		Expect(globalProvider.getChannelsCalls).To(Equal(1),
			"the construction-time reader should take its channels from the global provider")
		Expect(globalProvider.getInboundStatsCalls).To(Equal(1),
			"the inbound-stats reader should read through the global provider")
	})
})
