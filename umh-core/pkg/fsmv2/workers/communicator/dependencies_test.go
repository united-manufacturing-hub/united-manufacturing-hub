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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// Test suite is registered in worker_test.go to avoid duplicate RunSpecs

type mockTransport struct{}

func (m *mockTransport) Authenticate(_ context.Context, _ transport.AuthRequest) (transport.AuthResponse, error) {
	return transport.AuthResponse{}, nil
}
func (m *mockTransport) Pull(_ context.Context, _ string) ([]*transport.UMHMessage, error) {
	return nil, nil
}
func (m *mockTransport) Push(_ context.Context, _ string, _ []*transport.UMHMessage) error {
	return nil
}
func (m *mockTransport) Close() {}
func (m *mockTransport) Reset() {}

var _ = Describe("CommunicatorDependencies", func() {
	var (
		mt     transport.Transport
		logger *zap.SugaredLogger
	)

	BeforeEach(func() {
		mt = &mockTransport{}
		logger = zap.NewNop().Sugar()
	})

	Describe("NewCommunicatorDependencies", func() {
		Context("when creating a new dependencies", func() {
			It("should return a non-nil dependencies", func() {
				identity := fsmv2.Identity{ID: "test-id", WorkerType: "communicator"}
				deps := communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
				Expect(deps).NotTo(BeNil())
			})

			It("should store the transport", func() {
				identity := fsmv2.Identity{ID: "test-id", WorkerType: "communicator"}
				deps := communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
				Expect(deps.GetTransport()).To(Equal(mt))
			})

			It("should store the logger", func() {
				identity := fsmv2.Identity{ID: "test-id", WorkerType: "communicator"}
				deps := communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
				// Logger is enriched with worker context, so it won't equal original
				Expect(deps.GetLogger()).NotTo(BeNil())
			})
		})
	})

	Describe("GetTransport", func() {
		It("should return the transport passed to the constructor", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "communicator"}
			deps := communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
			Expect(deps.GetTransport()).To(Equal(mt))
		})
	})

	Describe("GetLogger", func() {
		It("should return the logger inherited from BaseDependencies", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "communicator"}
			deps := communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
			// Logger is enriched with worker context
			Expect(deps.GetLogger()).NotTo(BeNil())
		})
	})

	Describe("Dependencies interface implementation", func() {
		It("should implement fsmv2.Dependencies interface", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "communicator"}
			deps := communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
			var _ fsmv2.Dependencies = deps
			Expect(deps).To(Satisfy(func(d interface{}) bool {
				_, ok := d.(fsmv2.Dependencies)

				return ok
			}))
		})
	})

	Describe("Bug #6: Instance info and auth success callback", func() {
		var deps *communicator.CommunicatorDependencies

		BeforeEach(func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "communicator"}
			deps = communicator.NewCommunicatorDependencies(mt, logger, nil, identity)
		})

		Describe("SetInstanceInfo", func() {
			It("should store instance UUID and name", func() {
				deps.SetInstanceInfo("backend-real-uuid-12345", "My Instance")
				uuid, name := deps.GetInstanceInfo()
				Expect(uuid).To(Equal("backend-real-uuid-12345"))
				Expect(name).To(Equal("My Instance"))
			})
		})

		Describe("SetOnAuthSuccessCallback", func() {
			It("should invoke callback when SetInstanceInfo is called", func() {
				var callbackUUID, callbackName string
				callbackCalled := false

				deps.SetOnAuthSuccessCallback(func(uuid, name string) {
					callbackCalled = true
					callbackUUID = uuid
					callbackName = name
				})

				deps.SetInstanceInfo("backend-uuid-from-auth", "Backend Instance Name")

				Expect(callbackCalled).To(BeTrue(), "Expected callback to be invoked")
				Expect(callbackUUID).To(Equal("backend-uuid-from-auth"))
				Expect(callbackName).To(Equal("Backend Instance Name"))
			})

			It("should not panic if no callback is set", func() {
				Expect(func() {
					deps.SetInstanceInfo("uuid", "name")
				}).NotTo(Panic())
			})

			It("should handle callback being set after SetInstanceInfo was already called", func() {
				// First call without callback
				deps.SetInstanceInfo("first-uuid", "First Name")

				// Now set callback
				var callbackUUID string
				deps.SetOnAuthSuccessCallback(func(uuid, name string) {
					callbackUUID = uuid
				})

				// Second call should trigger callback
				deps.SetInstanceInfo("second-uuid", "Second Name")
				Expect(callbackUUID).To(Equal("second-uuid"))
			})
		})

		Describe("Thread safety for instance info", func() {
			It("should handle concurrent SetInstanceInfo calls", func() {
				done := make(chan bool, 10)

				for i := range 10 {
					go func(idx int) {
						deps.SetInstanceInfo(
							"uuid-"+string(rune('0'+idx)),
							"name-"+string(rune('0'+idx)),
						)
						done <- true
					}(i)
				}

				for range 10 {
					<-done
				}

				// Should not panic and should have some value
				uuid, name := deps.GetInstanceInfo()
				Expect(uuid).NotTo(BeEmpty())
				Expect(name).NotTo(BeEmpty())
			})
		})
	})

	Describe("Consecutive error tracking", func() {
		var deps *communicator.CommunicatorDependencies

		BeforeEach(func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "communicator"}
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
})
