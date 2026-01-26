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
	"go.uber.org/zap"

	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
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

var _ = Describe("CommunicatorWorker", func() {
	var (
		worker        *communicator.CommunicatorWorker
		ctx           context.Context
		mockTransport *MockTransport
		logger        *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop().Sugar()
		mockTransport = NewMockTransport()

		// Phase 1: Set up ChannelProvider singleton BEFORE creating worker
		communicator.SetChannelProvider(NewMockChannelProvider())

		var err error
		worker, err = communicator.NewCommunicatorWorker(
			"test-id",
			"Test Communicator",
			mockTransport,
			logger,
			nil,
		)
		Expect(err).ToNot(HaveOccurred())
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
			It("should return default CommunicatorDesiredState", func() {
				desiredIface, err := worker.DeriveDesiredState(nil)
				Expect(err).NotTo(HaveOccurred())

				// Type assert to typed CommunicatorDesiredState
				desired, ok := desiredIface.(*snapshot.CommunicatorDesiredState)
				Expect(ok).To(BeTrue(), "expected *snapshot.CommunicatorDesiredState")
				Expect(desired.GetState()).To(Equal("running"))
				Expect(desired.IsShutdownRequested()).To(BeFalse())
			})
		})

		Context("with valid UserSpec", func() {
			It("should return typed CommunicatorDesiredState with all fields populated", func() {
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

				// Type assert to typed CommunicatorDesiredState
				desired, ok := desiredIface.(*snapshot.CommunicatorDesiredState)
				Expect(ok).To(BeTrue(), "expected *snapshot.CommunicatorDesiredState")

				// Verify typed fields are populated
				Expect(desired.RelayURL).To(Equal("https://relay.umh.app"))
				Expect(desired.InstanceUUID).To(Equal("test-uuid-12345"))
				Expect(desired.AuthToken).To(Equal("test-auth-token-secret"))
				Expect(desired.Timeout).To(Equal(15 * time.Second))
				Expect(desired.GetState()).To(Equal("running"))
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

				desired := desiredIface.(*snapshot.CommunicatorDesiredState)
				// Default timeout = LongPollingDuration (30s) + LongPollingBuffer (1s) = 31s
				Expect(desired.Timeout).To(Equal(httpTransport.LongPollingDuration + httpTransport.LongPollingBuffer))
			})
		})

		Context("type assertion and roundtrip", func() {
			It("should preserve typed fields through marshal/unmarshal roundtrip", func() {
				// Create spec with all typed fields
				spec := fsmv2types.UserSpec{
					Config: `
relayURL: "https://relay.example.com"
instanceUUID: "roundtrip-uuid-test"
authToken: "roundtrip-auth-token"
timeout: 30s
state: "running"
`,
				}

				// Derive desired state
				desiredIface, err := worker.DeriveDesiredState(spec)
				Expect(err).NotTo(HaveOccurred())

				// Type assert to verify type
				originalDesired, ok := desiredIface.(*snapshot.CommunicatorDesiredState)
				Expect(ok).To(BeTrue())

				// Marshal to JSON (simulates what supervisor does)
				jsonBytes, err := json.Marshal(originalDesired)
				Expect(err).NotTo(HaveOccurred())

				// Unmarshal back (simulates what supervisor does when loading)
				var loadedDesired snapshot.CommunicatorDesiredState
				err = json.Unmarshal(jsonBytes, &loadedDesired)
				Expect(err).NotTo(HaveOccurred())

				// Verify ALL typed fields survived the roundtrip
				Expect(loadedDesired.RelayURL).To(Equal("https://relay.example.com"))
				Expect(loadedDesired.InstanceUUID).To(Equal("roundtrip-uuid-test"))
				Expect(loadedDesired.AuthToken).To(Equal("roundtrip-auth-token"))
				Expect(loadedDesired.Timeout).To(Equal(30 * time.Second))
				Expect(loadedDesired.GetState()).To(Equal("running"))
			})

			It("should return clear error on invalid spec type", func() {
				// Pass wrong type
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
		It("should return observed state with CollectedAt timestamp", func() {
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())

			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.CollectedAt).NotTo(BeZero())
		})

		It("should return the JWT token stored in dependencies", func() {
			deps := worker.GetDependencies()
			expectedToken := "test-jwt-token-12345"
			deps.SetJWT(expectedToken, time.Now().Add(1*time.Hour))

			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.JWTToken).To(Equal(expectedToken))
		})

		It("should return the JWT expiry stored in dependencies", func() {
			deps := worker.GetDependencies()
			expectedExpiry := time.Now().Add(2 * time.Hour).Truncate(time.Second)
			deps.SetJWT("some-token", expectedExpiry)

			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.JWTExpiry.Truncate(time.Second)).To(Equal(expectedExpiry))
		})

		It("should return the pulled messages stored in dependencies", func() {
			deps := worker.GetDependencies()
			expectedMessages := []*transportpkg.UMHMessage{
				{Content: "message-1"},
				{Content: "message-2"},
			}
			deps.SetPulledMessages(expectedMessages)

			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.MessagesReceived).To(HaveLen(2))
			Expect(communicatorObserved.MessagesReceived[0].Content).To(Equal("message-1"))
			Expect(communicatorObserved.MessagesReceived[1].Content).To(Equal("message-2"))
		})

		It("should return the consecutive error count from dependencies", func() {
			deps := worker.GetDependencies()
			deps.RecordError()
			deps.RecordError()
			deps.RecordError()

			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.ConsecutiveErrors).To(Equal(3))
		})

		It("should set Authenticated to true when JWT token is present and not expired", func() {
			deps := worker.GetDependencies()
			deps.SetJWT("valid-token", time.Now().Add(1*time.Hour))

			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.Authenticated).To(BeTrue())
		})

		It("should set Authenticated to false when JWT token is empty", func() {
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.Authenticated).To(BeFalse())
		})

		Context("metrics accumulation", func() {
			type metricsTestContext struct {
				worker      *communicator.CommunicatorWorker
				stateReader *MockStateReader
			}

			createWorkerWithStateReader := func() *metricsTestContext {
				stateReader := NewMockStateReader()
				w, err := communicator.NewCommunicatorWorker(
					"test-metrics-id",
					"Test Metrics Worker",
					mockTransport,
					logger,
					stateReader,
				)
				Expect(err).NotTo(HaveOccurred())

				return &metricsTestContext{worker: w, stateReader: stateReader}
			}

			// Helper to collect observed state and save it (simulates collector)
			collectAndSave := func(tc *metricsTestContext) snapshot.CommunicatorObservedState {
				observed, err := tc.worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())
				communicatorObserved := observed.(snapshot.CommunicatorObservedState)
				// Simulate collector saving observed state
				tc.stateReader.SaveObserved("communicator", "test-metrics-id", communicatorObserved)

				return communicatorObserved
			}

			It("should accumulate pull metrics on successful pull", func() {
				deps := worker.GetDependencies()
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullSuccess, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPulled, 5)
				deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPullLatencyMs, 100.0)

				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				communicatorObserved := observed.(snapshot.CommunicatorObservedState)
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterPullOps)]).To(Equal(int64(1)))
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterPullSuccess)]).To(Equal(int64(1)))
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterPullFailures)]).To(Equal(int64(0)))
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterMessagesPulled)]).To(Equal(int64(5)))
				Expect(communicatorObserved.Metrics.Worker.Gauges[string(depspkg.GaugeLastPullLatencyMs)]).To(Equal(100.0))
			})

			It("should accumulate pull metrics on failed pull", func() {
				deps := worker.GetDependencies()
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullFailures, 1)
				deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPullLatencyMs, 50.0)

				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				communicatorObserved := observed.(snapshot.CommunicatorObservedState)
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterPullOps)]).To(Equal(int64(1)))
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterPullSuccess)]).To(Equal(int64(0)))
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterPullFailures)]).To(Equal(int64(1)))
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterMessagesPulled)]).To(Equal(int64(0)))
				Expect(communicatorObserved.Metrics.Worker.Gauges[string(depspkg.GaugeLastPullLatencyMs)]).To(Equal(50.0))
			})

			It("should accumulate push metrics on successful push", func() {
				deps := worker.GetDependencies()
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPushOps, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPushSuccess, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPushed, 3)
				deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPushLatencyMs, 200.0)

				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				communicatorObserved := observed.(snapshot.CommunicatorObservedState)
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterPushOps)]).To(Equal(int64(1)))
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterPushSuccess)]).To(Equal(int64(1)))
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterPushFailures)]).To(Equal(int64(0)))
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterMessagesPushed)]).To(Equal(int64(3)))
				Expect(communicatorObserved.Metrics.Worker.Gauges[string(depspkg.GaugeLastPushLatencyMs)]).To(Equal(200.0))
			})

			It("should accumulate push metrics on failed push", func() {
				deps := worker.GetDependencies()
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPushOps, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPushFailures, 1)
				deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPushLatencyMs, 150.0)

				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				communicatorObserved := observed.(snapshot.CommunicatorObservedState)
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterPushOps)]).To(Equal(int64(1)))
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterPushSuccess)]).To(Equal(int64(0)))
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterPushFailures)]).To(Equal(int64(1)))
				Expect(communicatorObserved.Metrics.Worker.Counters[string(depspkg.CounterMessagesPushed)]).To(Equal(int64(0)))
			})

			It("should clear per-tick results after CollectObservedState", func() {
				tc := createWorkerWithStateReader()
				deps := tc.worker.GetDependencies()

				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPulled, 2)

				observed1 := collectAndSave(tc)
				Expect(observed1.Metrics.Worker.Counters[string(depspkg.CounterPullOps)]).To(Equal(int64(1)))

				observed2 := collectAndSave(tc)
				Expect(observed2.Metrics.Worker.Counters[string(depspkg.CounterPullOps)]).To(Equal(int64(1))) // Still 1, no new tick
			})

			It("should track last latency as gauge", func() {
				tc := createWorkerWithStateReader()
				deps := tc.worker.GetDependencies()

				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
				deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPullLatencyMs, 100.0)
				observed1 := collectAndSave(tc)
				Expect(observed1.Metrics.Worker.Gauges[string(depspkg.GaugeLastPullLatencyMs)]).To(Equal(100.0))

				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
				deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPullLatencyMs, 200.0)
				observed2 := collectAndSave(tc)
				Expect(observed2.Metrics.Worker.Counters[string(depspkg.CounterPullOps)]).To(Equal(int64(2)))
				Expect(observed2.Metrics.Worker.Gauges[string(depspkg.GaugeLastPullLatencyMs)]).To(Equal(200.0))
			})

			It("should accumulate message counts across multiple pulls", func() {
				tc := createWorkerWithStateReader()
				deps := tc.worker.GetDependencies()

				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPulled, 5)
				collectAndSave(tc)

				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPulled, 3)
				observed2 := collectAndSave(tc)
				Expect(observed2.Metrics.Worker.Counters[string(depspkg.CounterMessagesPulled)]).To(Equal(int64(8)))

				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPulled, 2)
				observed3 := collectAndSave(tc)
				Expect(observed3.Metrics.Worker.Counters[string(depspkg.CounterMessagesPulled)]).To(Equal(int64(10)))
			})

			It("should track both successful and failed ops separately", func() {
				tc := createWorkerWithStateReader()
				deps := tc.worker.GetDependencies()

				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullSuccess, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPulled, 1)
				collectAndSave(tc)

				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullFailures, 1)
				collectAndSave(tc)

				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullSuccess, 1)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPulled, 1)
				observed := collectAndSave(tc)

				Expect(observed.Metrics.Worker.Counters[string(depspkg.CounterPullOps)]).To(Equal(int64(3)))
				Expect(observed.Metrics.Worker.Counters[string(depspkg.CounterPullSuccess)]).To(Equal(int64(2)))
				Expect(observed.Metrics.Worker.Counters[string(depspkg.CounterPullFailures)]).To(Equal(int64(1)))
			})
		})

		// Phase 2: AuthenticatedUUID in ObservedState tests
		Context("authenticated UUID tracking", func() {
			It("should return the authenticated UUID stored in dependencies", func() {
				deps := worker.GetDependencies()
				expectedUUID := "backend-authenticated-uuid-12345"
				deps.SetAuthenticatedUUID(expectedUUID)

				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				communicatorObserved := observed.(snapshot.CommunicatorObservedState)
				Expect(communicatorObserved.AuthenticatedUUID).To(Equal(expectedUUID))
			})

			It("should return empty string when no UUID has been set", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				communicatorObserved := observed.(snapshot.CommunicatorObservedState)
				Expect(communicatorObserved.AuthenticatedUUID).To(BeEmpty())
			})

			It("should update AuthenticatedUUID when SetAuthenticatedUUID is called multiple times", func() {
				deps := worker.GetDependencies()
				deps.SetAuthenticatedUUID("first-uuid")
				deps.SetAuthenticatedUUID("second-uuid-after-reauth")

				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				communicatorObserved := observed.(snapshot.CommunicatorObservedState)
				Expect(communicatorObserved.AuthenticatedUUID).To(Equal("second-uuid-after-reauth"))
			})
		})
	})
})
