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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// MockStateReader implements fsmv2.StateReader for testing metrics accumulation.
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
				Expect(desired.Timeout).To(Equal(10 * time.Second))
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
			// Arrange: Set JWT token in dependencies
			deps := worker.GetDependencies()
			expectedToken := "test-jwt-token-12345"
			deps.SetJWT(expectedToken, time.Now().Add(1*time.Hour))

			// Act
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert
			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.JWTToken).To(Equal(expectedToken))
		})

		It("should return the JWT expiry stored in dependencies", func() {
			// Arrange: Set JWT expiry in dependencies
			deps := worker.GetDependencies()
			expectedExpiry := time.Now().Add(2 * time.Hour).Truncate(time.Second)
			deps.SetJWT("some-token", expectedExpiry)

			// Act
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert
			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			// Compare truncated times to avoid nanosecond precision issues
			Expect(communicatorObserved.JWTExpiry.Truncate(time.Second)).To(Equal(expectedExpiry))
		})

		It("should return the pulled messages stored in dependencies", func() {
			// Arrange: Set pulled messages in dependencies
			deps := worker.GetDependencies()
			expectedMessages := []*transportpkg.UMHMessage{
				{Content: "message-1"},
				{Content: "message-2"},
			}
			deps.SetPulledMessages(expectedMessages)

			// Act
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert
			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.MessagesReceived).To(HaveLen(2))
			Expect(communicatorObserved.MessagesReceived[0].Content).To(Equal("message-1"))
			Expect(communicatorObserved.MessagesReceived[1].Content).To(Equal("message-2"))
		})

		It("should return the consecutive error count from dependencies", func() {
			// Arrange: Record some errors in dependencies
			deps := worker.GetDependencies()
			deps.RecordError()
			deps.RecordError()
			deps.RecordError()

			// Act
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert
			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.ConsecutiveErrors).To(Equal(3))
		})

		It("should set Authenticated to true when JWT token is present and not expired", func() {
			// Arrange: Set valid JWT in dependencies
			deps := worker.GetDependencies()
			deps.SetJWT("valid-token", time.Now().Add(1*time.Hour))

			// Act
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert
			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.Authenticated).To(BeTrue())
		})

		It("should set Authenticated to false when JWT token is empty", func() {
			// Arrange: No JWT token set (default state)
			// Dependencies start with empty JWT

			// Act
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert
			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.Authenticated).To(BeFalse())
		})

		Context("metrics accumulation", func() {
			// Helper to create worker with MockStateReader and simulate collector save
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
				// Arrange: Record a successful pull
				deps := worker.GetDependencies()
				deps.RecordPullSuccess(100*time.Millisecond, 5)

				// Act
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Assert
				communicatorObserved := observed.(snapshot.CommunicatorObservedState)
				Expect(communicatorObserved.Metrics.Pull.TotalOps).To(Equal(1))
				Expect(communicatorObserved.Metrics.Pull.SuccessfulOps).To(Equal(1))
				Expect(communicatorObserved.Metrics.Pull.FailedOps).To(Equal(0))
				Expect(communicatorObserved.Metrics.Pull.MessagesPulled).To(Equal(5))
				Expect(communicatorObserved.Metrics.Pull.LastLatency).To(Equal(100 * time.Millisecond))
				Expect(communicatorObserved.Metrics.Pull.AvgLatency).To(Equal(100 * time.Millisecond))
			})

			It("should accumulate pull metrics on failed pull", func() {
				// Arrange: Record a failed pull
				deps := worker.GetDependencies()
				deps.RecordPullFailure(50 * time.Millisecond)

				// Act
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Assert
				communicatorObserved := observed.(snapshot.CommunicatorObservedState)
				Expect(communicatorObserved.Metrics.Pull.TotalOps).To(Equal(1))
				Expect(communicatorObserved.Metrics.Pull.SuccessfulOps).To(Equal(0))
				Expect(communicatorObserved.Metrics.Pull.FailedOps).To(Equal(1))
				Expect(communicatorObserved.Metrics.Pull.MessagesPulled).To(Equal(0))
				Expect(communicatorObserved.Metrics.Pull.LastLatency).To(Equal(50 * time.Millisecond))
			})

			It("should accumulate push metrics on successful push", func() {
				// Arrange: Record a successful pull (required for sync tick) and push
				deps := worker.GetDependencies()
				deps.RecordPullSuccess(50*time.Millisecond, 0)
				deps.RecordPushSuccess(200*time.Millisecond, 3)

				// Act
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Assert
				communicatorObserved := observed.(snapshot.CommunicatorObservedState)
				Expect(communicatorObserved.Metrics.Push.TotalOps).To(Equal(1))
				Expect(communicatorObserved.Metrics.Push.SuccessfulOps).To(Equal(1))
				Expect(communicatorObserved.Metrics.Push.FailedOps).To(Equal(0))
				Expect(communicatorObserved.Metrics.Push.MessagesPushed).To(Equal(3))
				Expect(communicatorObserved.Metrics.Push.LastLatency).To(Equal(200 * time.Millisecond))
			})

			It("should accumulate push metrics on failed push", func() {
				// Arrange: Record a successful pull and failed push
				deps := worker.GetDependencies()
				deps.RecordPullSuccess(50*time.Millisecond, 0)
				deps.RecordPushFailure(150 * time.Millisecond)

				// Act
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Assert
				communicatorObserved := observed.(snapshot.CommunicatorObservedState)
				Expect(communicatorObserved.Metrics.Push.TotalOps).To(Equal(1))
				Expect(communicatorObserved.Metrics.Push.SuccessfulOps).To(Equal(0))
				Expect(communicatorObserved.Metrics.Push.FailedOps).To(Equal(1))
				Expect(communicatorObserved.Metrics.Push.MessagesPushed).To(Equal(0))
			})

			It("should clear per-tick results after CollectObservedState", func() {
				// Use worker with state reader to test accumulation persistence
				tc := createWorkerWithStateReader()
				deps := tc.worker.GetDependencies()

				// Record a sync operation
				deps.RecordPullSuccess(100*time.Millisecond, 2)

				// First collection should have metrics
				observed1 := collectAndSave(tc)
				Expect(observed1.Metrics.Pull.TotalOps).To(Equal(1))

				// Second collection (without new sync) should maintain previous metrics
				// but NOT increment (syncTickCompleted is false)
				observed2 := collectAndSave(tc)
				Expect(observed2.Metrics.Pull.TotalOps).To(Equal(1)) // Still 1, no new tick
			})

			It("should calculate running average latency correctly", func() {
				// Use worker with state reader to test accumulation
				tc := createWorkerWithStateReader()
				deps := tc.worker.GetDependencies()

				// First pull: 100ms
				deps.RecordPullSuccess(100*time.Millisecond, 1)
				observed1 := collectAndSave(tc)
				Expect(observed1.Metrics.Pull.AvgLatency).To(Equal(100 * time.Millisecond))

				// Second pull: 200ms - average should be (100+200)/2 = 150ms
				deps.RecordPullSuccess(200*time.Millisecond, 1)
				observed2 := collectAndSave(tc)
				Expect(observed2.Metrics.Pull.TotalOps).To(Equal(2))
				Expect(observed2.Metrics.Pull.AvgLatency).To(Equal(150 * time.Millisecond))

				// Third pull: 300ms - average should be (100+200+300)/3 = 200ms
				deps.RecordPullSuccess(300*time.Millisecond, 1)
				observed3 := collectAndSave(tc)
				Expect(observed3.Metrics.Pull.TotalOps).To(Equal(3))
				Expect(observed3.Metrics.Pull.AvgLatency).To(Equal(200 * time.Millisecond))
			})

			It("should accumulate message counts across multiple pulls", func() {
				// Use worker with state reader to test accumulation
				tc := createWorkerWithStateReader()
				deps := tc.worker.GetDependencies()

				// First pull: 5 messages
				deps.RecordPullSuccess(50*time.Millisecond, 5)
				collectAndSave(tc)

				// Second pull: 3 messages
				deps.RecordPullSuccess(50*time.Millisecond, 3)
				observed2 := collectAndSave(tc)
				Expect(observed2.Metrics.Pull.MessagesPulled).To(Equal(8)) // 5 + 3

				// Third pull: 2 messages
				deps.RecordPullSuccess(50*time.Millisecond, 2)
				observed3 := collectAndSave(tc)
				Expect(observed3.Metrics.Pull.MessagesPulled).To(Equal(10)) // 5 + 3 + 2
			})

			It("should track both successful and failed ops separately", func() {
				// Use worker with state reader to test accumulation
				tc := createWorkerWithStateReader()
				deps := tc.worker.GetDependencies()

				// Success
				deps.RecordPullSuccess(50*time.Millisecond, 1)
				collectAndSave(tc)

				// Failure
				deps.RecordPullFailure(50 * time.Millisecond)
				collectAndSave(tc)

				// Success
				deps.RecordPullSuccess(50*time.Millisecond, 1)
				observed := collectAndSave(tc)

				Expect(observed.Metrics.Pull.TotalOps).To(Equal(3))
				Expect(observed.Metrics.Pull.SuccessfulOps).To(Equal(2))
				Expect(observed.Metrics.Pull.FailedOps).To(Equal(1))
			})
		})
	})
})
