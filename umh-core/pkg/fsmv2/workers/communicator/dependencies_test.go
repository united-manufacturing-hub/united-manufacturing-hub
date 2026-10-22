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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// Test suite is registered in worker_test.go to avoid duplicate RunSpecs

type mockTransport struct{}

func (m *mockTransport) Authenticate(_ context.Context, _ types.AuthRequest) (types.AuthResponse, error) {
	return types.AuthResponse{}, nil
}
func (m *mockTransport) Pull(_ context.Context, _ string) ([]*types.UMHMessage, error) {
	return nil, nil
}
func (m *mockTransport) Push(_ context.Context, _ string, _ []*types.UMHMessage) error {
	return nil
}
func (m *mockTransport) Close() {}
func (m *mockTransport) Reset() {}

// mockChannelProvider implements communicator.ChannelProvider for testing.
type mockChannelProvider struct {
	inbound  chan<- *types.UMHMessage
	outbound <-chan *types.UMHMessage
}

func (m *mockChannelProvider) GetChannels(_ string) (
	inbound chan<- *types.UMHMessage,
	outbound <-chan *types.UMHMessage,
) {
	return m.inbound, m.outbound
}

func (m *mockChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	// Return reasonable defaults for dependency tests (not testing backpressure here)
	return 100, 0
}

// newTestChannelProvider creates a mock channel provider for test setup.
func newTestChannelProvider() *mockChannelProvider {
	// Create bidirectional channels, then extract send-only and receive-only
	inboundBi := make(chan *types.UMHMessage, 100)
	outboundBi := make(chan *types.UMHMessage, 100)

	return &mockChannelProvider{
		inbound:  inboundBi,
		outbound: outboundBi,
	}
}

var _ = Describe("CommunicatorDependencies", func() {
	var (
		mt     types.Transport
		logger depspkg.FSMLogger
	)

	BeforeEach(func() {
		mt = &mockTransport{}
		logger = depspkg.NewNopFSMLogger()
	})

	Describe("NewCommunicatorDependencies", func() {
		Context("when creating a new dependencies", func() {
			It("should return a non-nil dependencies", func() {
				identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
				deps := communicator.NewCommunicatorDependencies(mt, depspkg.NewBaseDependencies(logger, nil, identity), newTestChannelProvider())
				Expect(deps).NotTo(BeNil())
			})

			It("should store the transport", func() {
				identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
				deps := communicator.NewCommunicatorDependencies(mt, depspkg.NewBaseDependencies(logger, nil, identity), newTestChannelProvider())
				Expect(deps.GetTransport()).To(Equal(mt))
			})

			It("should store the logger", func() {
				identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
				deps := communicator.NewCommunicatorDependencies(mt, depspkg.NewBaseDependencies(logger, nil, identity), newTestChannelProvider())
				Expect(deps.GetLogger()).NotTo(BeNil())
			})
		})
	})

	Describe("GetTransport", func() {
		It("should return the transport passed to the constructor", func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps := communicator.NewCommunicatorDependencies(mt, depspkg.NewBaseDependencies(logger, nil, identity), newTestChannelProvider())
			Expect(deps.GetTransport()).To(Equal(mt))
		})
	})

	Describe("GetLogger", func() {
		It("should return the logger inherited from BaseDependencies", func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps := communicator.NewCommunicatorDependencies(mt, depspkg.NewBaseDependencies(logger, nil, identity), newTestChannelProvider())
			// Logger is enriched with worker context
			Expect(deps.GetLogger()).NotTo(BeNil())
		})
	})

	Describe("Dependencies interface implementation", func() {
		It("should implement deps.Dependencies interface", func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps := communicator.NewCommunicatorDependencies(mt, depspkg.NewBaseDependencies(logger, nil, identity), newTestChannelProvider())
			var _ depspkg.Dependencies = deps
			Expect(deps).To(Satisfy(func(d interface{}) bool {
				_, ok := d.(depspkg.Dependencies)

				return ok
			}))
		})
	})

	Describe("Consecutive error tracking", func() {
		var deps *communicator.CommunicatorDependencies

		BeforeEach(func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps = communicator.NewCommunicatorDependencies(mt, depspkg.NewBaseDependencies(logger, nil, identity), newTestChannelProvider())
		})

		Describe("GetConsecutiveErrors", func() {
			Context("when no errors have been recorded", func() {
				It("should return 0", func() {
					Expect(deps.GetConsecutiveErrors()).To(Equal(0))
				})
			})
		})

		Describe("RecordError", func() {
			Context("when recording a single error", func() {
				It("should increment the counter to 1", func() {
					deps.RecordError()
					Expect(deps.GetConsecutiveErrors()).To(Equal(1))
				})
			})

			Context("when recording multiple consecutive errors", func() {
				It("should accumulate the count", func() {
					deps.RecordError()
					deps.RecordError()
					deps.RecordError()
					Expect(deps.GetConsecutiveErrors()).To(Equal(3))
				})
			})
		})

		Describe("RecordSuccess", func() {
			Context("when recording success after no errors", func() {
				It("should keep the counter at 0", func() {
					deps.RecordSuccess()
					Expect(deps.GetConsecutiveErrors()).To(Equal(0))
				})
			})

			Context("when recording success after errors", func() {
				It("should reset the counter to 0", func() {
					deps.RecordError()
					deps.RecordError()
					Expect(deps.GetConsecutiveErrors()).To(Equal(2))

					deps.RecordSuccess()
					Expect(deps.GetConsecutiveErrors()).To(Equal(0))
				})
			})
		})

		Describe("Thread safety", func() {
			It("should handle concurrent RecordError and RecordSuccess calls", func() {
				done := make(chan bool, 20)

				for range 10 {
					go func() {
						deps.RecordError()
						done <- true
					}()
				}

				for range 10 {
					go func() {
						deps.RecordSuccess()
						done <- true
					}()
				}

				for range 20 {
					<-done
				}

				Expect(deps.GetConsecutiveErrors()).To(BeNumerically(">=", 0))
			})
		})
	})

	// Transport reset responsibility belongs to ResetTransportAction, NOT RecordError.
	// RecordError only tracks error counts; transport reset is triggered by
	// TransportWorker's DegradedState dispatching ResetTransportAction at threshold multiples.
	// This separation ensures single responsibility and avoids double resets.
	Describe("RecordError does NOT reset transport (reset via FSM action only)", func() {
		var (
			mockTrans *MockTransport
			deps      *communicator.CommunicatorDependencies
		)

		BeforeEach(func() {
			mockTrans = NewMockTransport()
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps = communicator.NewCommunicatorDependencies(mockTrans, depspkg.NewBaseDependencies(logger, nil, identity), newTestChannelProvider())
		})

		Context("when errors are below threshold", func() {
			It("should NOT call Reset() for 4 consecutive errors", func() {
				for range 4 {
					deps.RecordError()
				}

				Expect(mockTrans.ResetCallCount()).To(Equal(0))
			})
		})

		Context("when errors reach threshold", func() {
			It("should NOT call Reset() even at TransportResetThreshold (5) - reset happens via FSM action", func() {
				for range 5 {
					deps.RecordError()
				}

				// Transport reset is handled by ResetTransportAction from TransportWorker's DegradedState,
				// not by RecordError. This avoids double resets.
				Expect(mockTrans.ResetCallCount()).To(Equal(0))
			})

			It("should NOT call Reset() even at threshold multiples (10) - reset happens via FSM action", func() {
				for range 10 {
					deps.RecordError()
				}

				// Transport reset is handled by ResetTransportAction from TransportWorker's DegradedState,
				// not by RecordError. This avoids double resets.
				Expect(mockTrans.ResetCallCount()).To(Equal(0))
			})
		})

		Context("when transport is nil", func() {
			It("should not panic when recording errors without transport", func() {
				identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
				depsWithNilTransport := communicator.NewCommunicatorDependencies(nil, depspkg.NewBaseDependencies(logger, nil, identity), newTestChannelProvider())

				Expect(func() {
					for range 10 {
						depsWithNilTransport.RecordError()
					}
				}).NotTo(Panic())
			})
		})

		Context("when success resets error count", func() {
			It("should track consecutive errors correctly without calling Reset()", func() {
				for range 3 {
					deps.RecordError()
				}
				Expect(deps.GetConsecutiveErrors()).To(Equal(3))
				Expect(mockTrans.ResetCallCount()).To(Equal(0))

				deps.RecordSuccess()
				Expect(deps.GetConsecutiveErrors()).To(Equal(0))

				for range 5 {
					deps.RecordError()
				}
				Expect(deps.GetConsecutiveErrors()).To(Equal(5))
				// Still no reset - that's TransportWorker's DegradedState job via ResetTransportAction
				Expect(mockTrans.ResetCallCount()).To(Equal(0))
			})
		})
	})

	Describe("DegradedEnteredAt tracking", func() {
		var deps *communicator.CommunicatorDependencies

		BeforeEach(func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps = communicator.NewCommunicatorDependencies(mt, depspkg.NewBaseDependencies(logger, nil, identity), newTestChannelProvider())
		})

		Describe("GetDegradedEnteredAt", func() {
			Context("when no errors have been recorded", func() {
				It("should return zero time", func() {
					Expect(deps.GetDegradedEnteredAt().IsZero()).To(BeTrue())
				})
			})
		})

		Describe("RecordError sets DegradedEnteredAt", func() {
			Context("when first error is recorded", func() {
				It("should set DegradedEnteredAt to current time", func() {
					Expect(deps.GetDegradedEnteredAt().IsZero()).To(BeTrue())

					deps.RecordError()

					enteredAt := deps.GetDegradedEnteredAt()
					Expect(enteredAt.IsZero()).To(BeFalse())
					Expect(enteredAt).To(BeTemporally("~", time.Now(), time.Second))
				})
			})

			Context("when subsequent errors are recorded", func() {
				It("should NOT update DegradedEnteredAt", func() {
					deps.RecordError()
					firstEnteredAt := deps.GetDegradedEnteredAt()

					deps.RecordError()
					deps.RecordError()

					Expect(deps.GetDegradedEnteredAt()).To(Equal(firstEnteredAt))
				})
			})
		})

		Describe("RecordSuccess clears DegradedEnteredAt", func() {
			Context("when success is recorded after errors", func() {
				It("should clear DegradedEnteredAt", func() {
					deps.RecordError()
					Expect(deps.GetDegradedEnteredAt().IsZero()).To(BeFalse())

					deps.RecordSuccess()

					Expect(deps.GetDegradedEnteredAt().IsZero()).To(BeTrue())
				})
			})

			Context("when success is recorded without prior errors", func() {
				It("should keep DegradedEnteredAt as zero", func() {
					Expect(deps.GetDegradedEnteredAt().IsZero()).To(BeTrue())

					deps.RecordSuccess()

					Expect(deps.GetDegradedEnteredAt().IsZero()).To(BeTrue())
				})
			})
		})

		Describe("DegradedEnteredAt preserved through error sequence", func() {
			It("should track the original entry time through multiple errors and reset on success", func() {
				Expect(deps.GetDegradedEnteredAt().IsZero()).To(BeTrue())
				Expect(deps.GetConsecutiveErrors()).To(Equal(0))

				deps.RecordError()
				firstEnteredAt := deps.GetDegradedEnteredAt()
				Expect(firstEnteredAt.IsZero()).To(BeFalse())

				deps.RecordError()
				deps.RecordError()
				Expect(deps.GetDegradedEnteredAt()).To(Equal(firstEnteredAt))
				Expect(deps.GetConsecutiveErrors()).To(Equal(3))

				deps.RecordSuccess()
				Expect(deps.GetDegradedEnteredAt().IsZero()).To(BeTrue())
				Expect(deps.GetConsecutiveErrors()).To(Equal(0))

				deps.RecordError()
				newEnteredAt := deps.GetDegradedEnteredAt()
				Expect(newEnteredAt.IsZero()).To(BeFalse())
				Expect(newEnteredAt).To(BeTemporally(">=", firstEnteredAt))
			})
		})
	})

	Describe("NewCommunicatorDependencies", func() {
		Context("when the provider is given", func() {
			It("should create dependencies with the provider's channels", func() {
				inbound := make(chan<- *types.UMHMessage, 10)
				outbound := make(<-chan *types.UMHMessage, 10)
				mockProvider := &mockChannelProvider{
					inbound:  inbound,
					outbound: outbound,
				}

				identity := depspkg.Identity{ID: "test-provider-id", WorkerType: "communicator"}
				deps := communicator.NewCommunicatorDependencies(mt, depspkg.NewBaseDependencies(logger, nil, identity), mockProvider)

				Expect(deps).NotTo(BeNil())
				Expect(deps.GetInboundChan()).To(Equal(inbound))
				Expect(deps.GetOutboundChan()).To(Equal(outbound))
			})
		})
	})
})
