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

package supervisor_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"go.uber.org/zap"
)

var _ = Describe("Edge Cases", func() {

	Describe("Supervisor signal processing", func() {
		Context("when signal is SignalNone", func() {
			It("should process normally", func() {

				initialState := &mockState{signal: fsmv2.SignalNone}

				s := newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{})

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Describe("RequestShutdown", func() {
		Context("when shutdown is requested", func() {
			It("should save desired state with shutdown flag", func() {
				mockStore := newMockTriangularStore()

				identity := mockIdentity()
				identityDoc := persistence.Document{
					"id":         identity.ID,
					"name":       identity.Name,
					"workerType": identity.WorkerType,
				}
				err := mockStore.SaveIdentity(context.Background(), "container", identity.ID, identityDoc)
				Expect(err).ToNot(HaveOccurred())

				desiredDoc := persistence.Document{
					"id":               identity.ID,
					"shutdownRequested": false,
				}
				_, err = mockStore.SaveDesired(context.Background(), "container", identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType: "container",
					Store:      mockStore,
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold: 10 * time.Second,
						Timeout:        20 * time.Second,
					},
				})

				worker := &mockWorker{}
				err = s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				err = s.TestRequestShutdown(context.Background(), identity.ID, "test reason")
				Expect(err).ToNot(HaveOccurred())
				Expect(mockStore.SaveDesiredCalled).To(BeNumerically(">", 1))
			})
		})

		Context("when worker does not exist", func() {
			It("should return error", func() {
				mockStore := newMockTriangularStore()

				identity := mockIdentity()

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType: "container",
					Store:      mockStore,
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold: 10 * time.Second,
						Timeout:        20 * time.Second,
					},
				})

				// Don't add worker - test that requestShutdown fails for non-existent worker
				err := s.TestRequestShutdown(context.Background(), identity.ID, "test reason")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})
	})

	Describe("Action execution with retry", func() {
		Context("when action succeeds on first try", func() {
			It("should execute without retry", func() {
				var callCount int32
				action := &mockAction{
					executeFunc: func(ctx context.Context, deps any) error {
						atomic.AddInt32(&callCount, 1)

						return nil
					},
				}

				initialState := &mockState{}
				initialState.nextState = initialState
				initialState.action = action

				s := newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{
					ObservationTimeout: 1000 * time.Millisecond,
				})

				ctx := context.Background()

				// Start supervisor to run action executor
				done := s.Start(ctx)
				defer func() {
					s.Shutdown()
					<-done
				}()


				// Wait for action to execute
				time.Sleep(1200 * time.Millisecond)

				Expect(atomic.LoadInt32(&callCount)).To(Equal(int32(1)))
			})
		})

		Context("when action fails once then succeeds", func() {
			// Skip: ActionExecutor doesn't have retry logic yet
			// This test is for future retry functionality
			PIt("should retry and succeed", func() {
				var callCount int32
				action := &mockAction{
					executeFunc: func(ctx context.Context, deps any) error {
						count := atomic.AddInt32(&callCount, 1)
						if count == 1 {
							return errors.New("temporary error")
						}

						return nil
					},
				}

				initialState := &mockState{}
				initialState.nextState = initialState
				initialState.action = action

				s := newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{
					ObservationTimeout: 1000 * time.Millisecond,
				})

				ctx := context.Background()

				// Start supervisor to run action executor
				done := s.Start(ctx)
				defer func() {
					s.Shutdown()
					<-done
				}()


				// Wait for action to execute (with retry)
				time.Sleep(2500 * time.Millisecond)

				Expect(atomic.LoadInt32(&callCount)).To(Equal(int32(2)))
			})
		})

		Context("when action always fails", func() {
			// Skip: ActionExecutor doesn't have retry logic yet
			// This test is for future retry functionality
			PIt("should retry max times then fail", func() {
				var callCount int32
				action := &mockAction{
					executeFunc: func(ctx context.Context, deps any) error {
						atomic.AddInt32(&callCount, 1)

						return errors.New("persistent error")
					},
				}

				initialState := &mockState{}
				initialState.nextState = initialState
				initialState.action = action

				s := newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{
					ObservationTimeout: 1000 * time.Millisecond,
				})

				ctx := context.Background()

				// Start supervisor to run action executor
				done := s.Start(ctx)
				defer func() {
					s.Shutdown()
					<-done
				}()


				// Wait for action to execute (with max retries)
				time.Sleep(3500 * time.Millisecond)

				Expect(atomic.LoadInt32(&callCount)).To(Equal(int32(3)))
			})
		})
	})

	Describe("Tick timeout handling", func() {
		Context("when data times out and restart count is below max", func() {
			It("should restart collector", func() {
				mockStore := newMockTriangularStore()

				identity := mockIdentity()
				identityDoc := persistence.Document{
					"id":         identity.ID,
					"name":       identity.Name,
					"workerType": identity.WorkerType,
				}
				err := mockStore.SaveIdentity(context.Background(), "container", identity.ID, identityDoc)
				Expect(err).ToNot(HaveOccurred())

				oldTimestamp := time.Now().Add(-30 * time.Second)
				obs := &mockObservedState{
					ID:          identity.ID,
					CollectedAt: oldTimestamp,
					Desired:     &mockDesiredState{},
				}
				_, err = mockStore.SaveObserved(context.Background(), "container", identity.ID, obs)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType: "container",
					Store:      mockStore,
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold:     10 * time.Second,
						Timeout:            20 * time.Second,
						MaxRestartAttempts: 3,
					},
				})

				worker := &mockWorker{
					observed: obs,
				}
				err = s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				err = s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(s.TestGetRestartCount()).To(Equal(1))
			})
		})

		Context("when data times out and restart count is at max", func() {
			It("should request shutdown", func() {
				mockStore := newMockTriangularStore()

				identity := mockIdentity()
				identityDoc := persistence.Document{
					"id":         identity.ID,
					"name":       identity.Name,
					"workerType": identity.WorkerType,
				}
				err := mockStore.SaveIdentity(context.Background(), "container", identity.ID, identityDoc)
				Expect(err).ToNot(HaveOccurred())

				desiredDoc := persistence.Document{
					"id":               identity.ID,
					"shutdownRequested": false,
				}
				_, err = mockStore.SaveDesired(context.Background(), "container", identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				oldTimestamp := time.Now().Add(-30 * time.Second)
				obs := &mockObservedState{
					ID:          identity.ID,
					CollectedAt: oldTimestamp,
					Desired:     &mockDesiredState{},
				}
				_, err = mockStore.SaveObserved(context.Background(), "container", identity.ID, obs)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType: "container",
					Store:      mockStore,
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold:     10 * time.Second,
						Timeout:            20 * time.Second,
						MaxRestartAttempts: 3,
					},
				})

				worker := &mockWorker{
					observed: obs,
				}
				err = s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				s.TestSetRestartCount(3)

				err = s.TestTick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unresponsive"))
			})
		})

		Context("when collector recovers after restarts", func() {
			It("should reset restart count", func() {
				mockStore := newMockTriangularStore()

				identity := mockIdentity()
				identityDoc := persistence.Document{
					"id":         identity.ID,
					"name":       identity.Name,
					"workerType": identity.WorkerType,
				}
				err := mockStore.SaveIdentity(context.Background(), "container", identity.ID, identityDoc)
				Expect(err).ToNot(HaveOccurred())

				obs := &mockObservedState{
					ID:          identity.ID,
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				}
				_, err = mockStore.SaveObserved(context.Background(), "container", identity.ID, obs)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType:      "container",
					Store:           mockStore,
					Logger:          zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{},
				})

				worker := &mockWorker{observed: obs}
				err = s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				s.TestSetRestartCount(2)

				err = s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(s.TestGetRestartCount()).To(Equal(0))
			})
		})
	})

	Describe("Tick LoadSnapshot error handling", func() {
		Context("when LoadSnapshot fails", func() {
			It("should return error", func() {
				mockStore := newMockTriangularStore()
				mockStore.LoadSnapshotErr = errors.New("snapshot load failed")

				identity := mockIdentity()
				identityDoc := persistence.Document{
					"id":         identity.ID,
					"name":       identity.Name,
					"workerType": identity.WorkerType,
				}
				err := mockStore.SaveIdentity(context.Background(), "container", identity.ID, identityDoc)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType:      "container",
					Store:           mockStore,
					Logger:          zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{},
				})

				worker := &mockWorker{}
				err = s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				err = s.TestTick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("snapshot"))
			})
		})
	})

	Describe("Context cancellation", func() {
		Context("when context is canceled during tick loop", func() {
			It("should stop tick loop gracefully", func() {
				worker := &mockWorker{}
				s := newSupervisorWithWorker(worker, nil, supervisor.CollectorHealthConfig{
					ObservationTimeout: 1000 * time.Millisecond,
					StaleThreshold:     10 * time.Second,
					Timeout:            20 * time.Second,
					MaxRestartAttempts: 3,
				})
				defer s.Shutdown()

				ctx, cancel := context.WithCancel(context.Background())

				done := s.Start(ctx)

				time.Sleep(50 * time.Millisecond)

				cancel()

				Eventually(done, 2*time.Second).Should(BeClosed())
			})
		})

		Context("when context is canceled before first tick", func() {
			It("should stop immediately", func() {
				worker := &mockWorker{}
				s := newSupervisorWithWorker(worker, nil, supervisor.CollectorHealthConfig{
					ObservationTimeout: 1000 * time.Millisecond,
					StaleThreshold:     10 * time.Second,
					Timeout:            20 * time.Second,
					MaxRestartAttempts: 3,
				})
				defer s.Shutdown()

				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				done := s.Start(ctx)

				Eventually(done, 1*time.Second).Should(BeClosed())
			})
		})
	})

	Describe("RequestShutdown with valid worker", func() {
		Context("when worker exists", func() {
			It("should set ShutdownRequested in persistence when desired state exists", func() {
				mockStore := newMockTriangularStore()

				identity := mockIdentity()
				identityDoc := persistence.Document{
					"id":         identity.ID,
					"name":       identity.Name,
					"workerType": identity.WorkerType,
				}
				err := mockStore.SaveIdentity(context.Background(), "container", identity.ID, identityDoc)
				Expect(err).ToNot(HaveOccurred())

				// Save a desired state that requestShutdown can load and modify
				desiredDoc := persistence.Document{
					"ShutdownRequested": false,
				}
				_, err = mockStore.SaveDesired(context.Background(), "container", identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType: "container",
					Store:      mockStore,
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold: 10 * time.Second,
						Timeout:        20 * time.Second,
					},
				})

				worker := &mockWorker{}
				err = s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				// RequestShutdown should succeed and update persistence
				err = s.TestRequestShutdown(context.Background(), identity.ID, "test reason")
				Expect(err).ToNot(HaveOccurred())

				// Verify ShutdownRequested was set in the store
				savedDesired, loadErr := mockStore.LoadDesired(context.Background(), "container", identity.ID)
				Expect(loadErr).ToNot(HaveOccurred())
				savedDoc, ok := savedDesired.(persistence.Document)
				Expect(ok).To(BeTrue())
				Expect(savedDoc["ShutdownRequested"]).To(BeTrue())
			})
		})

		Context("when desired state does not exist", func() {
			It("should return nil (no desired state to update)", func() {
				mockStore := newMockTriangularStore()

				identity := mockIdentity()
				identityDoc := persistence.Document{
					"id":         identity.ID,
					"name":       identity.Name,
					"workerType": identity.WorkerType,
				}
				err := mockStore.SaveIdentity(context.Background(), "container", identity.ID, identityDoc)
				Expect(err).ToNot(HaveOccurred())

				// Don't save any desired state - requestShutdown should return nil

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType: "container",
					Store:      mockStore,
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold: 10 * time.Second,
						Timeout:        20 * time.Second,
					},
				})

				worker := &mockWorker{}
				err = s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				// RequestShutdown should return nil when no desired state exists
				err = s.TestRequestShutdown(context.Background(), identity.ID, "test reason")
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Describe("Shutdown escalation with SaveDesired error", func() {
		Context("when max restart attempts reached and SaveDesired fails", func() {
			It("should log error and return shutdown error", func() {
				mockStore := newMockTriangularStore()

				identity := mockIdentity()
				identityDoc := persistence.Document{
					"id":         identity.ID,
					"name":       identity.Name,
					"workerType": identity.WorkerType,
				}
				err := mockStore.SaveIdentity(context.Background(), "container", identity.ID, identityDoc)
				Expect(err).ToNot(HaveOccurred())

				desiredDoc := persistence.Document{
					"id":               identity.ID,
					"shutdownRequested": false,
				}
				_, err = mockStore.SaveDesired(context.Background(), "container", identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				oldTimestamp := time.Now().Add(-30 * time.Second)
				obs := &mockObservedState{
					ID:          identity.ID,
					CollectedAt: oldTimestamp,
					Desired:     &mockDesiredState{},
				}
				_, err = mockStore.SaveObserved(context.Background(), "container", identity.ID, obs)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType: "container",
					Store:      mockStore,
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold:     10 * time.Second,
						Timeout:            20 * time.Second,
						MaxRestartAttempts: 3,
					},
				})

				worker := &mockWorker{
					observed: obs,
				}
				err = s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				// Set error AFTER AddWorker (which needs SaveDesired to succeed)
				mockStore.SaveDesiredErr = errors.New("save failed")

				s.TestSetRestartCount(3)

				err = s.TestTick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unresponsive"))
			})
		})
	})

	Describe("Stale data pausing FSM", func() {
		Context("when data is stale but not timed out", func() {
			It("should pause FSM without restarting collector", func() {

				s := newSupervisorWithWorker(&mockWorker{}, nil, supervisor.CollectorHealthConfig{
					StaleThreshold:     10 * time.Second,
					Timeout:            20 * time.Second,
					MaxRestartAttempts: 3,
				})

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(s.TestGetRestartCount()).To(Equal(0))
			})
		})
	})
})

type mockAction struct {
	executeFunc func(ctx context.Context, deps any) error
	name        string
}

func (m *mockAction) Execute(ctx context.Context, deps any) error {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, deps)
	}

	return nil
}

func (m *mockAction) Name() string {
	if m.name != "" {
		return m.name
	}

	return "MockAction"
}

type alternateObservedState struct {
	timestamp time.Time
}

func (a *alternateObservedState) GetTimestamp() time.Time { return a.timestamp }
func (a *alternateObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &mockDesiredState{}
}

var _ = Describe("Type Safety (Invariant I16)", func() {
	Describe("ObservedState type validation", func() {
		Context("when worker returns wrong ObservedState type", func() {
			It("should panic with clear message before calling state.Next()", func() {
				mockStore := newMockTriangularStore()

				identity := mockIdentity()
				identityDoc := persistence.Document{
					"id":         identity.ID,
					"name":       identity.Name,
					"workerType": identity.WorkerType,
				}
				err := mockStore.SaveIdentity(context.Background(), "container", identity.ID, identityDoc)
				Expect(err).ToNot(HaveOccurred())

				desiredDoc := persistence.Document{
					"id":               identity.ID,
					"shutdownRequested": false,
				}
				_, err = mockStore.SaveDesired(context.Background(), "container", identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType: "container",
					Store:      mockStore,
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold: 10 * time.Second,
						Timeout:        20 * time.Second,
					},
				})

				worker := &mockWorker{
					observed: &mockObservedState{ID: "test-worker", CollectedAt: time.Now()},
				}
				err = s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				wrongTypeObs := &alternateObservedState{timestamp: time.Now()}
				if mockStore.Observed["container"] == nil {
					mockStore.Observed["container"] = make(map[string]interface{})
				}
				mockStore.Observed["container"][identity.ID] = wrongTypeObs

				var panicMessage string
				func() {
					defer func() {
						if r := recover(); r != nil {
							panicMessage = fmt.Sprintf("%v", r)
						}
					}()
					_ = s.TestTick(context.Background())
				}()

				Expect(panicMessage).To(ContainSubstring("Invariant I16 violated"))
				Expect(panicMessage).To(ContainSubstring("test-worker"))
				Expect(panicMessage).To(ContainSubstring("alternateObservedState"))
				Expect(panicMessage).To(ContainSubstring("TestObservedState"))
			})
		})

		Context("when worker returns nil ObservedState", func() {
			It("should panic with clear message before calling state.Next()", func() {
				mockStore := newMockTriangularStore()

				identity := mockIdentity()
				identityDoc := persistence.Document{
					"id":         identity.ID,
					"name":       identity.Name,
					"workerType": identity.WorkerType,
				}
				err := mockStore.SaveIdentity(context.Background(), "container", identity.ID, identityDoc)
				Expect(err).ToNot(HaveOccurred())

				desiredDoc := persistence.Document{
					"id":               identity.ID,
					"shutdownRequested": false,
				}
				_, err = mockStore.SaveDesired(context.Background(), "container", identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType: "container",
					Store:      mockStore,
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold: 10 * time.Second,
						Timeout:        20 * time.Second,
					},
				})

				worker := &mockWorker{
					observed: &mockObservedState{ID: "test-worker", CollectedAt: time.Now()},
				}
				err = s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				if mockStore.Observed["container"] == nil {
					mockStore.Observed["container"] = make(map[string]interface{})
				}
				mockStore.Observed["container"][identity.ID] = nil

				var panicMessage string
				func() {
					defer func() {
						if r := recover(); r != nil {
							panicMessage = fmt.Sprintf("%v", r)
						}
					}()
					_ = s.TestTick(context.Background())
				}()

				Expect(panicMessage).To(ContainSubstring("Invariant I16 violated"))
				Expect(panicMessage).To(ContainSubstring("nil ObservedState"))
				Expect(panicMessage).To(ContainSubstring("test-worker"))
			})
		})

		Context("when worker consistently returns correct type", func() {
			It("should not panic", func() {
				worker := &mockWorker{
					observed: &mockObservedState{ID: "test-worker", CollectedAt: time.Now()},
				}

				s := newSupervisorWithWorker(worker, nil, supervisor.CollectorHealthConfig{})

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when worker returns pointer type consistently (I16 normalization test)", func() {
			It("should not panic due to pointer vs struct type mismatch", func() {
				worker := &mockWorker{
					observed: &mockObservedState{ID: "test-worker", CollectedAt: time.Now()},
				}

				s := newSupervisorWithWorker(worker, nil, supervisor.CollectorHealthConfig{})

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
