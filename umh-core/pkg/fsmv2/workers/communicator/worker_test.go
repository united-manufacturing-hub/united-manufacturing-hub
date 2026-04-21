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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// MockStateReader implements deps.StateReader for testing metrics accumulation.
// It stores observed state and returns it on subsequent LoadObservedTyped calls.
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
		// Marshal and unmarshal to copy the data
		data, err := json.Marshal(stored)
		if err != nil {
			return err
		}

		return json.Unmarshal(data, result)
	}

	return nil // No previous state
}

// SaveObserved saves the observed state for later retrieval (simulates collector behavior).
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

// createCommunicatorWorker creates a CommunicatorWorker using the new WorkerBase API.
func createCommunicatorWorker(id string, logger depspkg.FSMLogger, sr depspkg.StateReader) *communicator.CommunicatorWorker {
	identity := depspkg.Identity{ID: id, WorkerType: "communicator", Name: id}
	w, err := communicator.NewCommunicatorWorker(identity, logger, sr, register.NoDeps{})
	Expect(err).ToNot(HaveOccurred())

	return w.(*communicator.CommunicatorWorker)
}

var _ = Describe("CommunicatorWorker", func() {
	var (
		worker *communicator.CommunicatorWorker
		ctx    context.Context
		logger depspkg.FSMLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = depspkg.NewNopFSMLogger()

		// Phase 1: Set up ChannelProvider singleton BEFORE creating worker
		communicator.SetChannelProvider(NewMockChannelProvider())

		worker = createCommunicatorWorker("test-id", logger, nil)
	})

	AfterEach(func() {
		// Clean up singleton after each test
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
			It("should return default WrappedDesiredState", func() {
				desiredIface, err := worker.DeriveDesiredState(nil)
				Expect(err).NotTo(HaveOccurred())

				desired, ok := desiredIface.(*fsmv2.WrappedDesiredState[communicator.CommunicatorConfig])
				Expect(ok).To(BeTrue(), "expected *fsmv2.WrappedDesiredState[CommunicatorConfig]")
				Expect(desired.GetState()).To(Equal("running"))
				Expect(desired.IsShutdownRequested()).To(BeFalse())
			})

			It("should include TransportWorker child spec", func() {
				desiredIface, err := worker.DeriveDesiredState(nil)
				Expect(err).NotTo(HaveOccurred())

				desired := desiredIface.(*fsmv2.WrappedDesiredState[communicator.CommunicatorConfig])
				specs := desired.GetChildrenSpecs()
				Expect(specs).To(HaveLen(1))
				Expect(specs[0].Name).To(Equal("transport"))
				Expect(specs[0].WorkerType).To(Equal("transport"))
				Expect(specs[0].ChildStartStates).To(ConsistOf("Syncing", "Recovering"))
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
				Expect(desired.GetState()).To(Equal("running"))

				specs := desired.GetChildrenSpecs()
				Expect(specs).To(HaveLen(1))
				Expect(specs[0].Name).To(Equal("transport"))
				Expect(specs[0].WorkerType).To(Equal("transport"))
				Expect(specs[0].ChildStartStates).To(ConsistOf("Syncing", "Recovering"))
				Expect(specs[0].UserSpec.Config).To(Equal(spec.Config))
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
				// Default timeout = LongPollingDuration (30s) + LongPollingBuffer (1s) = 31s
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
				Expect(loadedDesired.GetState()).To(Equal("running"))
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
				Expect(err.Error()).To(ContainSubstring("config unmarshal failed"))
			})
		})
	})

	Describe("CollectObservedState", func() {
		It("should return observed state with zero CollectedAt (collector sets it)", func() {
			observed, err := worker.CollectObservedState(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())

			communicatorObserved := observed.(fsmv2.Observation[communicator.CommunicatorStatus])
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

			communicatorObserved := observed.(fsmv2.Observation[communicator.CommunicatorStatus])
			Expect(communicatorObserved.Metrics.Worker.Counters).To(BeEmpty(),
				"NewObservation does not drain MetricsRecorder; collector handles accumulation")
			Expect(communicatorObserved.Metrics.Worker.Gauges).To(BeEmpty())
		})

	})
})
