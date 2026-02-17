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

package transport_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	communicator_transport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// mockTransport implements the transport.Transport interface for testing.
type mockTransport struct {
	resetCallCount int
}

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

func (m *mockTransport) Reset() {
	m.resetCallCount++
}

func (m *mockTransport) ResetCallCount() int {
	return m.resetCallCount
}

// mockChannelProvider implements transport.ChannelProvider for testing.
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

func (m *mockChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	return 100, 0
}

// newTestChannelProvider creates a mock channel provider for test setup.
func newTestChannelProvider() *mockChannelProvider {
	inboundBi := make(chan *communicator_transport.UMHMessage, 100)
	outboundBi := make(chan *communicator_transport.UMHMessage, 100)

	return &mockChannelProvider{
		inbound:  inboundBi,
		outbound: outboundBi,
	}
}

var _ = Describe("TransportDependencies", func() {
	var (
		mt     communicator_transport.Transport
		logger depspkg.FSMLogger
	)

	BeforeEach(func() {
		mt = &mockTransport{}
		logger = depspkg.NewNopFSMLogger()
		// Set up singleton for ALL tests (except architecture tests)
		transport.SetChannelProvider(newTestChannelProvider())
	})

	AfterEach(func() {
		transport.ClearChannelProvider()
	})

	Describe("NewTransportDependencies", func() {
		Context("when creating new dependencies", func() {
			It("should return a non-nil dependencies", func() {
				identity := depspkg.Identity{ID: "test-id", WorkerType: "transport"}
				deps := transport.NewTransportDependencies(mt, logger, nil, identity)
				Expect(deps).NotTo(BeNil())
			})

			It("should store the transport", func() {
				identity := depspkg.Identity{ID: "test-id", WorkerType: "transport"}
				deps := transport.NewTransportDependencies(mt, logger, nil, identity)
				Expect(deps.GetTransport()).To(Equal(mt))
			})

			It("should store the logger", func() {
				identity := depspkg.Identity{ID: "test-id", WorkerType: "transport"}
				deps := transport.NewTransportDependencies(mt, logger, nil, identity)
				Expect(deps.GetLogger()).NotTo(BeNil())
			})
		})
	})

	Describe("JWT token management", func() {
		var deps *transport.TransportDependencies

		BeforeEach(func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "transport"}
			deps = transport.NewTransportDependencies(mt, logger, nil, identity)
		})

		Describe("SetJWT", func() {
			It("should store the JWT token and expiry", func() {
				expiry := time.Now().Add(1 * time.Hour)
				deps.SetJWT("test-jwt-token", expiry)

				Expect(deps.GetJWTToken()).To(Equal("test-jwt-token"))
				Expect(deps.GetJWTExpiry()).To(BeTemporally("~", expiry, time.Second))
			})

			It("should update JWT when called multiple times (re-authentication)", func() {
				firstExpiry := time.Now().Add(1 * time.Hour)
				deps.SetJWT("first-token", firstExpiry)

				secondExpiry := time.Now().Add(2 * time.Hour)
				deps.SetJWT("second-token", secondExpiry)

				Expect(deps.GetJWTToken()).To(Equal("second-token"))
				Expect(deps.GetJWTExpiry()).To(BeTemporally("~", secondExpiry, time.Second))
			})
		})

		Describe("GetJWTToken", func() {
			It("should return empty string when no token has been set", func() {
				Expect(deps.GetJWTToken()).To(BeEmpty())
			})
		})

		Describe("GetJWTExpiry", func() {
			It("should return zero time when no expiry has been set", func() {
				Expect(deps.GetJWTExpiry().IsZero()).To(BeTrue())
			})
		})

	})

	Describe("Transport management", func() {
		var deps *transport.TransportDependencies

		BeforeEach(func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "transport"}
			deps = transport.NewTransportDependencies(mt, logger, nil, identity)
		})

		Describe("SetTransport", func() {
			It("should update the transport instance", func() {
				newTransport := &mockTransport{}
				deps.SetTransport(newTransport)
				Expect(deps.GetTransport()).To(Equal(newTransport))
			})

			It("should allow setting transport to nil", func() {
				deps.SetTransport(nil)
				Expect(deps.GetTransport()).To(BeNil())
			})
		})

		Describe("GetTransport", func() {
			It("should return the transport passed to the constructor", func() {
				Expect(deps.GetTransport()).To(Equal(mt))
			})
		})
	})

	Describe("Channel management", func() {
		var deps *transport.TransportDependencies

		BeforeEach(func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "transport"}
			deps = transport.NewTransportDependencies(mt, logger, nil, identity)
		})

		Describe("GetInboundChan", func() {
			It("should return a non-nil inbound channel", func() {
				Expect(deps.GetInboundChan()).NotTo(BeNil())
			})
		})

		Describe("GetOutboundChan", func() {
			It("should return a non-nil outbound channel", func() {
				Expect(deps.GetOutboundChan()).NotTo(BeNil())
			})
		})
	})

	Describe("Error tracking", func() {
		var deps *transport.TransportDependencies

		BeforeEach(func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "transport"}
			deps = transport.NewTransportDependencies(mt, logger, nil, identity)
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

		Describe("Typed error recording", func() {
			It("should record error type via RecordTypedError", func() {
				deps.RecordTypedError(httpTransport.ErrorTypeBackendRateLimit, 30*time.Second)
				Expect(deps.GetLastErrorType()).To(Equal(httpTransport.ErrorTypeBackendRateLimit))
			})

			It("should record retry-after duration", func() {
				deps.RecordTypedError(httpTransport.ErrorTypeBackendRateLimit, 45*time.Second)
				Expect(deps.GetLastRetryAfter()).To(Equal(45 * time.Second))
			})

			It("should clear error type on RecordSuccess", func() {
				deps.RecordTypedError(httpTransport.ErrorTypeInvalidToken, 0)
				Expect(deps.GetLastErrorType()).To(Equal(httpTransport.ErrorTypeInvalidToken))

				deps.RecordSuccess()
				Expect(deps.GetLastErrorType()).To(Equal(httpTransport.ErrorType(0)))
			})

			It("should preserve transport instance after errors", func() {
				originalTransport := deps.GetTransport()
				deps.RecordTypedError(httpTransport.ErrorTypeBackendRateLimit, 0)
				deps.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
				Expect(deps.GetTransport()).To(Equal(originalTransport))
			})
		})

	})

	Describe("DegradedEnteredAt tracking", func() {
		var deps *transport.TransportDependencies

		BeforeEach(func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "transport"}
			deps = transport.NewTransportDependencies(mt, logger, nil, identity)
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
		})
	})

	Describe("RecordError does NOT reset transport (reset via FSM action only)", func() {
		var (
			mockTrans *mockTransport
			deps      *transport.TransportDependencies
		)

		BeforeEach(func() {
			mockTrans = &mockTransport{}
			identity := depspkg.Identity{ID: "test-id", WorkerType: "transport"}
			deps = transport.NewTransportDependencies(mockTrans, logger, nil, identity)
		})

		Context("when errors reach threshold", func() {
			It("should NOT call Reset() - reset happens via FSM action", func() {
				for range 10 {
					deps.RecordError()
				}

				// Transport reset is handled by ResetTransportAction from RecoveringState,
				// not by RecordError.
				Expect(mockTrans.ResetCallCount()).To(Equal(0))
			})
		})

		Context("when transport is nil", func() {
			It("should not panic when recording errors without transport", func() {
				identity := depspkg.Identity{ID: "test-id", WorkerType: "transport"}
				depsWithNilTransport := transport.NewTransportDependencies(nil, logger, nil, identity)

				Expect(func() {
					for range 10 {
						depsWithNilTransport.RecordError()
					}
				}).NotTo(Panic())
			})
		})
	})

	Describe("Dependencies interface implementation", func() {
		It("should implement deps.Dependencies interface", func() {
			identity := depspkg.Identity{ID: "test-id", WorkerType: "transport"}
			deps := transport.NewTransportDependencies(mt, logger, nil, identity)
			var _ depspkg.Dependencies = deps
			Expect(deps).To(Satisfy(func(d interface{}) bool {
				_, ok := d.(depspkg.Dependencies)

				return ok
			}))
		})
	})

	Describe("ChannelProvider Singleton Architecture", func() {
		BeforeEach(func() {
			transport.ClearChannelProvider()
		})

		AfterEach(func() {
			transport.ClearChannelProvider()
		})

		Describe("NewTransportDependencies", func() {
			Context("when ChannelProvider singleton is NOT set", func() {
				It("should panic with clear error message", func() {
					Expect(transport.GetChannelProvider()).To(BeNil())

					identity := depspkg.Identity{ID: "test-id", WorkerType: "transport"}
					Expect(func() {
						transport.NewTransportDependencies(mt, logger, nil, identity)
					}).To(PanicWith(ContainSubstring("ChannelProvider must be set")))
				})
			})

			Context("when ChannelProvider singleton IS set", func() {
				It("should NOT panic and create dependencies with channels from singleton", func() {
					inbound := make(chan<- *communicator_transport.UMHMessage, 10)
					outbound := make(<-chan *communicator_transport.UMHMessage, 10)
					mockProvider := &mockChannelProvider{
						inbound:  inbound,
						outbound: outbound,
					}
					transport.SetChannelProvider(mockProvider)

					identity := depspkg.Identity{ID: "test-singleton-id", WorkerType: "transport"}
					var deps *transport.TransportDependencies
					Expect(func() {
						deps = transport.NewTransportDependencies(mt, logger, nil, identity)
					}).NotTo(Panic())

					Expect(deps).NotTo(BeNil())
					Expect(deps.GetInboundChan()).To(Equal(inbound))
					Expect(deps.GetOutboundChan()).To(Equal(outbound))
				})
			})
		})
	})
})
