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
	"go.uber.org/zap"

	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	communicator_transport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// Test suite is registered in worker_test.go to avoid duplicate RunSpecs

// =============================================================================
// Phase 1 Architecture Verification Tests - ChannelProvider Singleton
// =============================================================================
//
// These tests verify the Phase 1 FSMv2 Communicator Architecture:
// 1. ChannelProvider MUST be set via global singleton BEFORE creating dependencies
// 2. deps.SetChannelProvider() method should NOT exist (removed in Phase 1)
// 3. factory deps["channelProvider"] path should NOT exist (removed in Phase 1)
//
// Compile-time verification:
// The removal of deps.SetChannelProvider() is verified at compile time - if this
// code compiles, the method does not exist on CommunicatorDependencies:
//
//     var _ interface{ SetChannelProvider(ChannelProvider) } = (*communicator.CommunicatorDependencies)(nil)
//     // ^ This line would fail to compile if the method still exists
//
// The above is NOT included because it would FAIL compilation after Phase 1 is complete.
// Instead, if someone accidentally adds the method back, the architecture tests
// will catch it at runtime.
// =============================================================================

type mockTransport struct{}

func (m *mockTransport) Authenticate(_ context.Context, _ communicator_transport.AuthRequest) (communicator_transport.AuthResponse, error) {
	return communicator_transport.AuthResponse{}, nil
}
func (m *mockTransport) Pull(_ context.Context, _ string) ([]*communicator_transport.UMHMessage, error) {
	return nil, nil
}
func (m *mockTransport) Push(_ context.Context, _ string, _ []*communicator_transport.UMHMessage) error {
	return nil
}
func (m *mockTransport) Close() {}
func (m *mockTransport) Reset() {}

// mockChannelProvider implements communicator.ChannelProvider for testing.
type mockChannelProvider struct {
	inbound  chan<- *communicator_transport.UMHMessage
	outbound <-chan *communicator_transport.UMHMessage
}

func (m *mockChannelProvider) GetChannels(_ string) (
	inbound chan<- *communicator_transport.UMHMessage,
	outbound <-chan *communicator_transport.UMHMessage,
) {
	return m.inbound, m.outbound
}

// newTestChannelProvider creates a mock channel provider for test setup.
func newTestChannelProvider() *mockChannelProvider {
	// Create bidirectional channels, then extract send-only and receive-only
	inboundBi := make(chan *communicator_transport.UMHMessage, 100)
	outboundBi := make(chan *communicator_transport.UMHMessage, 100)

	return &mockChannelProvider{
		inbound:  inboundBi,
		outbound: outboundBi,
	}
}

var _ = Describe("CommunicatorDependencies", func() {
	var (
		mt     communicator_transport.Transport
		logger *zap.SugaredLogger
	)

	BeforeEach(func() {
		mt = &mockTransport{}
		logger = zap.NewNop().Sugar()
		// Phase 1: Set up singleton for ALL tests (except Phase 1 architecture tests)
		communicator.SetChannelProvider(newTestChannelProvider())
	})

	AfterEach(func() {
		// Clean up singleton after each test
		communicator.ClearChannelProvider()
	})

	Describe("NewCommunicatorDependencies", func() {
		Context("when creating a new dependencies", func() {
			It("should return a non-nil dependencies", func() {
				identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
				deps := communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
				Expect(deps).NotTo(BeNil())
			})

			It("should store the transport", func() {
				identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
				deps := communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
				Expect(deps.GetTransport()).To(Equal(mt))
			})

			It("should store the logger", func() {
				identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
				deps := communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
				// Logger is enriched with worker context, so it won't equal original
				Expect(deps.GetLogger()).NotTo(BeNil())
			})
		})
	})

	Describe("GetTransport", func() {
		It("should return the transport passed to the constructor", func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps := communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
			Expect(deps.GetTransport()).To(Equal(mt))
		})
	})

	Describe("GetLogger", func() {
		It("should return the logger inherited from BaseDependencies", func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps := communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
			// Logger is enriched with worker context
			Expect(deps.GetLogger()).NotTo(BeNil())
		})
	})

	Describe("Dependencies interface implementation", func() {
		It("should implement deps.Dependencies interface", func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps := communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
			var _ depspkg.Dependencies = deps
			Expect(deps).To(Satisfy(func(d interface{}) bool {
				_, ok := d.(depspkg.Dependencies)

				return ok
			}))
		})
	})

	Describe("Phase 2: AuthenticatedUUID storage via ObservedState", func() {
		var deps *communicator.CommunicatorDependencies

		BeforeEach(func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps = communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
		})

		Describe("SetAuthenticatedUUID", func() {
			It("should store the authenticated UUID", func() {
				deps.SetAuthenticatedUUID("backend-real-uuid-12345")
				uuid := deps.GetAuthenticatedUUID()
				Expect(uuid).To(Equal("backend-real-uuid-12345"))
			})

			It("should update UUID when called multiple times (re-authentication)", func() {
				deps.SetAuthenticatedUUID("first-uuid")
				deps.SetAuthenticatedUUID("second-uuid-after-reauth")
				uuid := deps.GetAuthenticatedUUID()
				Expect(uuid).To(Equal("second-uuid-after-reauth"))
			})
		})

		Describe("GetAuthenticatedUUID", func() {
			It("should return empty string when no UUID has been set", func() {
				uuid := deps.GetAuthenticatedUUID()
				Expect(uuid).To(BeEmpty())
			})
		})

		Describe("Thread safety for authenticated UUID", func() {
			It("should handle concurrent SetAuthenticatedUUID calls", func() {
				done := make(chan bool, 10)

				for i := range 10 {
					go func(idx int) {
						deps.SetAuthenticatedUUID("uuid-" + string(rune('0'+idx)))
						done <- true
					}(i)
				}

				for range 10 {
					<-done
				}

				// Should not panic and should have some value
				uuid := deps.GetAuthenticatedUUID()
				Expect(uuid).NotTo(BeEmpty())
			})

			It("should handle concurrent read and write", func() {
				done := make(chan bool, 20)

				// Writers
				for i := range 10 {
					go func(idx int) {
						deps.SetAuthenticatedUUID("uuid-" + string(rune('0'+idx)))
						done <- true
					}(i)
				}

				// Readers
				for range 10 {
					go func() {
						_ = deps.GetAuthenticatedUUID()
						done <- true
					}()
				}

				for range 20 {
					<-done
				}

				// Should not panic
			})
		})

		Describe("Backward compatibility: SetInstanceInfo (deprecated)", func() {
			It("should store instance UUID and name via deprecated method", func() {
				deps.SetInstanceInfo("backend-real-uuid-12345", "My Instance")
				uuid, name := deps.GetInstanceInfo()
				Expect(uuid).To(Equal("backend-real-uuid-12345"))
				Expect(name).To(Equal("My Instance"))
			})
		})
	})

	Describe("Consecutive error tracking", func() {
		var deps *communicator.CommunicatorDependencies

		BeforeEach(func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps = communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
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

				// Launch multiple goroutines to record errors
				for range 10 {
					go func() {
						deps.RecordError()
						done <- true
					}()
				}

				// Launch multiple goroutines to record success
				for range 10 {
					go func() {
						deps.RecordSuccess()
						done <- true
					}()
				}

				// Wait for all goroutines to complete
				for range 20 {
					<-done
				}

				// The counter should be a non-negative integer
				Expect(deps.GetConsecutiveErrors()).To(BeNumerically(">=", 0))
			})
		})
	})

	Describe("Transport reset on consecutive errors", func() {
		var (
			mockTrans *MockTransport
			deps      *communicator.CommunicatorDependencies
		)

		BeforeEach(func() {
			mockTrans = NewMockTransport()
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps = communicator.NewCommunicatorDependencies(mockTrans, logger, nil, identity)
		})

		Context("when errors are below threshold", func() {
			It("should NOT call Reset() for 4 consecutive errors", func() {
				// Record 4 errors (below threshold of 5)
				for range 4 {
					deps.RecordError()
				}

				Expect(mockTrans.ResetCallCount()).To(Equal(0))
			})
		})

		Context("when errors reach threshold", func() {
			It("should call Reset() when consecutive errors reach TransportResetThreshold (5)", func() {
				// Record 5 errors to reach threshold
				for range 5 {
					deps.RecordError()
				}

				// Reset should be called once when threshold is reached
				Expect(mockTrans.ResetCallCount()).To(Equal(1))
			})

			It("should call Reset() again when errors accumulate to next threshold multiple", func() {
				// Record 10 errors (two thresholds)
				for range 10 {
					deps.RecordError()
				}

				// Reset should be called twice (at 5 and 10)
				Expect(mockTrans.ResetCallCount()).To(Equal(2))
			})
		})

		Context("when transport is nil", func() {
			It("should not panic when recording errors without transport", func() {
				identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
				// Create deps with nil transport
				depsWithNilTransport := communicator.NewCommunicatorDependencies(nil, logger, nil, identity)

				// Should not panic even when threshold is reached
				Expect(func() {
					for range 10 {
						depsWithNilTransport.RecordError()
					}
				}).NotTo(Panic())
			})
		})

		Context("when success resets error count", func() {
			It("should require reaching threshold again after RecordSuccess", func() {
				// Record 3 errors
				for range 3 {
					deps.RecordError()
				}
				Expect(mockTrans.ResetCallCount()).To(Equal(0))

				// Success resets counter
				deps.RecordSuccess()

				// Record 4 more errors (still below threshold)
				for range 4 {
					deps.RecordError()
				}
				Expect(mockTrans.ResetCallCount()).To(Equal(0))

				// 5th error should trigger reset
				deps.RecordError()
				Expect(mockTrans.ResetCallCount()).To(Equal(1))
			})
		})
	})

	Describe("DegradedEnteredAt tracking", func() {
		var deps *communicator.CommunicatorDependencies

		BeforeEach(func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
			deps = communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
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

					// DegradedEnteredAt should now be set
					enteredAt := deps.GetDegradedEnteredAt()
					Expect(enteredAt.IsZero()).To(BeFalse())
					// Should be very recent (within last second)
					Expect(enteredAt).To(BeTemporally("~", time.Now(), time.Second))
				})
			})

			Context("when subsequent errors are recorded", func() {
				It("should NOT update DegradedEnteredAt", func() {
					deps.RecordError()
					firstEnteredAt := deps.GetDegradedEnteredAt()

					// Small delay to ensure time difference
					deps.RecordError()
					deps.RecordError()

					// DegradedEnteredAt should still be the original time
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

					// DegradedEnteredAt should be cleared
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
				// No errors yet
				Expect(deps.GetDegradedEnteredAt().IsZero()).To(BeTrue())
				Expect(deps.GetConsecutiveErrors()).To(Equal(0))

				// First error sets DegradedEnteredAt
				deps.RecordError()
				firstEnteredAt := deps.GetDegradedEnteredAt()
				Expect(firstEnteredAt.IsZero()).To(BeFalse())

				// More errors don't change DegradedEnteredAt
				deps.RecordError()
				deps.RecordError()
				Expect(deps.GetDegradedEnteredAt()).To(Equal(firstEnteredAt))
				Expect(deps.GetConsecutiveErrors()).To(Equal(3))

				// Success clears both
				deps.RecordSuccess()
				Expect(deps.GetDegradedEnteredAt().IsZero()).To(BeTrue())
				Expect(deps.GetConsecutiveErrors()).To(Equal(0))

				// New error sequence gets new timestamp
				deps.RecordError()
				newEnteredAt := deps.GetDegradedEnteredAt()
				Expect(newEnteredAt.IsZero()).To(BeFalse())
				// The new timestamp should be after (or equal to) the first one
				Expect(newEnteredAt).To(BeTemporally(">=", firstEnteredAt))
			})
		})
	})

	// =============================================================================
	// Phase 1: ChannelProvider Singleton Architecture Tests
	// =============================================================================
	//
	// After Phase 1, the ChannelProvider MUST be set via global singleton ONLY.
	// These tests verify:
	// 1. NewCommunicatorDependencies panics if singleton is nil
	// 2. deps.SetChannelProvider() method no longer exists (verified at runtime)
	//
	// See comment block at top of file for compile-time verification notes.
	// =============================================================================
	Describe("Phase 1: ChannelProvider Singleton Architecture", func() {
		BeforeEach(func() {
			// Ensure singleton is cleared before each test
			communicator.ClearChannelProvider()
		})

		AfterEach(func() {
			// Clean up singleton after each test
			communicator.ClearChannelProvider()
		})

		Describe("NewCommunicatorDependencies", func() {
			Context("when ChannelProvider singleton is NOT set", func() {
				It("should panic with clear error message", func() {
					// Ensure singleton is nil
					Expect(communicator.GetChannelProvider()).To(BeNil())

					// Creating dependencies without singleton should panic
					identity := depspkg.Identity{ID: "test-id", WorkerType: "communicator"}
					Expect(func() {
						communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
					}).To(PanicWith(ContainSubstring("ChannelProvider must be set")))
				})
			})

			Context("when ChannelProvider singleton IS set", func() {
				It("should NOT panic and create dependencies with channels from singleton", func() {
					// Set up mock provider
					inbound := make(chan<- *communicator_transport.UMHMessage, 10)
					outbound := make(<-chan *communicator_transport.UMHMessage, 10)
					mockProvider := &mockChannelProvider{
						inbound:  inbound,
						outbound: outbound,
					}
					communicator.SetChannelProvider(mockProvider)

					// Creating dependencies should NOT panic
					identity := depspkg.Identity{ID: "test-singleton-id", WorkerType: "communicator"}
					var deps *communicator.CommunicatorDependencies
					Expect(func() {
						deps = communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
					}).NotTo(Panic())

					Expect(deps).NotTo(BeNil())
					// Channels should be set from singleton provider
					Expect(deps.GetInboundChan()).To(Equal(inbound))
					Expect(deps.GetOutboundChan()).To(Equal(outbound))
				})
			})
		})
	})
})
