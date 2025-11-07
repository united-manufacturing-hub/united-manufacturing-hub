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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/collection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/health"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"go.uber.org/zap"
)

var _ = Describe("Edge Cases", func() {
	Describe("FreshnessChecker edge cases", func() {
		Context("when snapshot has nil observed state", func() {
			It("should return false for Check", func() {
				checker := health.NewFreshnessChecker(
					10*time.Second,
					20*time.Second,
					zap.NewNop().Sugar(),
				)

				snapshot := &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Observed: nil,
					Desired:  &mockDesiredState{},
				}

				Expect(checker.Check(snapshot)).To(BeFalse())
			})

			It("should return false for IsTimeout", func() {
				checker := health.NewFreshnessChecker(
					10*time.Second,
					20*time.Second,
					zap.NewNop().Sugar(),
				)

				snapshot := &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Observed: nil,
					Desired:  &mockDesiredState{},
				}

				Expect(checker.IsTimeout(snapshot)).To(BeFalse())
			})
		})
	})

	Describe("Collector error handling", func() {
		Context("when CollectObservedState fails", func() {
			It("should continue observation loop", func() {
				var callCountMutex sync.Mutex
				callCount := 0
				collector := collection.NewCollector(collection.CollectorConfig{
					Worker: &mockWorker{
						collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
							callCountMutex.Lock()
							callCount++
							count := callCount
							callCountMutex.Unlock()

							if count == 1 {
								return nil, errors.New("collection error")
							}

							return &mockObservedState{ID: "test-worker", CollectedAt: time.Now()}, nil
						},
					},
					Identity:            mockIdentity(),
					Store:               createTestTriangularStore(),
					Logger:              zap.NewNop().Sugar(),
					ObservationInterval: 50 * time.Millisecond,
					ObservationTimeout:  1 * time.Second,
				})

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				err := collector.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(200 * time.Millisecond)

				callCountMutex.Lock()
				finalCount := callCount
				callCountMutex.Unlock()
				Expect(finalCount).To(BeNumerically(">=", 2))

				cancel()
				time.Sleep(100 * time.Millisecond)
			})
		})

		Context("when SaveObserved fails", func() {
			It("should continue observation loop", func() {
				var saveCallCountMutex sync.Mutex
				saveCallCount := 0

				collector := collection.NewCollector(collection.CollectorConfig{
					Worker: &mockWorker{
						collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
							saveCallCountMutex.Lock()
							saveCallCount++
							saveCallCountMutex.Unlock()

							return &mockObservedState{ID: "test-worker", CollectedAt: time.Now()}, nil
						},
					},
					Identity:            mockIdentity(),
					Store:               createTestTriangularStore(),
					Logger:              zap.NewNop().Sugar(),
					ObservationInterval: 50 * time.Millisecond,
					ObservationTimeout:  1 * time.Second,
				})

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				err := collector.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(200 * time.Millisecond)

				saveCallCountMutex.Lock()
				finalSaveCount := saveCallCount
				saveCallCountMutex.Unlock()
				Expect(finalSaveCount).To(BeNumerically(">=", 2))

				cancel()
				time.Sleep(100 * time.Millisecond)
			})
		})

		Context("when context is canceled during collection", func() {
			It("should stop observation loop", func() {
				collector := collection.NewCollector(collection.CollectorConfig{
					Worker:              &mockWorker{},
					Identity:            mockIdentity(),
					Store:               createTestTriangularStore(),
					Logger:              zap.NewNop().Sugar(),
					ObservationInterval: 1 * time.Second,
					ObservationTimeout:  3 * time.Second,
				})

				ctx, cancel := context.WithCancel(context.Background())

				err := collector.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(50 * time.Millisecond)
				Expect(collector.IsRunning()).To(BeTrue())

				cancel()

				Eventually(func() bool {
					return collector.IsRunning()
				}, 2*time.Second).Should(BeFalse())
			})
		})

		Context("when Restart is called with pending restart", func() {
			It("should not block", func() {
				collector := collection.NewCollector(collection.CollectorConfig{
					Worker:              &mockWorker{},
					Identity:            mockIdentity(),
					Store:               createTestTriangularStore(),
					Logger:              zap.NewNop().Sugar(),
					ObservationInterval: 1 * time.Second,
					ObservationTimeout:  3 * time.Second,
				})

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				err := collector.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				collector.Restart()
				collector.Restart()

				time.Sleep(100 * time.Millisecond)

				cancel()
			})
		})
	})

	Describe("Supervisor signal processing", func() {
		Context("when signal is SignalNone", func() {
			It("should process normally", func() {

				initialState := &mockState{signal: fsmv2.SignalNone}

				s := newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{})

				err := s.Tick(context.Background())
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
				err = mockStore.SaveDesired(context.Background(), "container", identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor(supervisor.Config{
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

				err = s.RequestShutdown(context.Background(), identity.ID, "test reason")
				Expect(err).ToNot(HaveOccurred())
				Expect(mockStore.SaveDesiredCalled).To(BeNumerically(">", 1))
			})
		})

		Context("when SaveDesired fails", func() {
			It("should return error", func() {
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
				err = mockStore.SaveDesired(context.Background(), "container", identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				mockStore.SaveDesiredErr = errors.New("save failed")

				s := supervisor.NewSupervisor(supervisor.Config{
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

				err = s.RequestShutdown(context.Background(), identity.ID, "test reason")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("save"))
			})
		})
	})

	Describe("Action execution with retry", func() {
		Context("when action succeeds on first try", func() {
			It("should execute without retry", func() {
				callCount := 0
				action := &mockAction{
					executeFunc: func(ctx context.Context) error {
						callCount++

						return nil
					},
				}


				initialState := &mockState{}
				initialState.nextState = initialState
				initialState.action = action

				s := newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{})

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(callCount).To(Equal(1))
			})
		})

		Context("when action fails once then succeeds", func() {
			It("should retry and succeed", func() {
				callCount := 0
				action := &mockAction{
					executeFunc: func(ctx context.Context) error {
						callCount++
						if callCount == 1 {
							return errors.New("temporary error")
						}

						return nil
					},
				}


				initialState := &mockState{}
				initialState.nextState = initialState
				initialState.action = action

				s := newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{})

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(callCount).To(Equal(2))
			})
		})

		Context("when action always fails", func() {
			It("should retry max times then fail", func() {
				callCount := 0
				action := &mockAction{
					executeFunc: func(ctx context.Context) error {
						callCount++

						return errors.New("persistent error")
					},
				}


				initialState := &mockState{}
				initialState.nextState = initialState
				initialState.action = action

				s := newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{})

				err := s.Tick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed after"))
				Expect(callCount).To(Equal(3))
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
				err = mockStore.SaveObserved(context.Background(), "container", identity.ID, obs)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor(supervisor.Config{
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

				err = s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(s.GetRestartCount()).To(Equal(1))
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
				err = mockStore.SaveDesired(context.Background(), "container", identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				oldTimestamp := time.Now().Add(-30 * time.Second)
				obs := &mockObservedState{
					ID:          identity.ID,
					CollectedAt: oldTimestamp,
					Desired:     &mockDesiredState{},
				}
				err = mockStore.SaveObserved(context.Background(), "container", identity.ID, obs)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor(supervisor.Config{
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

				s.SetRestartCount(3)

				err = s.Tick(context.Background())
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
				err = mockStore.SaveObserved(context.Background(), "container", identity.ID, obs)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor(supervisor.Config{
					WorkerType:      "container",
					Store:           mockStore,
					Logger:          zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{},
				})

				worker := &mockWorker{observed: obs}
				err = s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				s.SetRestartCount(2)

				err = s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(s.GetRestartCount()).To(Equal(0))
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

				s := supervisor.NewSupervisor(supervisor.Config{
					WorkerType:      "container",
					Store:           mockStore,
					Logger:          zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{},
				})

				worker := &mockWorker{}
				err = s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				err = s.Tick(context.Background())
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

				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				done := s.Start(ctx)

				Eventually(done, 1*time.Second).Should(BeClosed())
			})
		})
	})

	Describe("RequestShutdown LoadDesired error", func() {
		Context("when LoadDesired fails", func() {
			It("should return error", func() {
				mockStore := newMockTriangularStore()
				mockStore.LoadDesiredErr = errors.New("load desired failed")

				identity := mockIdentity()
				identityDoc := persistence.Document{
					"id":         identity.ID,
					"name":       identity.Name,
					"workerType": identity.WorkerType,
				}
				err := mockStore.SaveIdentity(context.Background(), "container", identity.ID, identityDoc)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor(supervisor.Config{
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

				err = s.RequestShutdown(context.Background(), identity.ID, "test reason")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("load"))
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
				err = mockStore.SaveDesired(context.Background(), "container", identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				oldTimestamp := time.Now().Add(-30 * time.Second)
				obs := &mockObservedState{
					ID:          identity.ID,
					CollectedAt: oldTimestamp,
					Desired:     &mockDesiredState{},
				}
				err = mockStore.SaveObserved(context.Background(), "container", identity.ID, obs)
				Expect(err).ToNot(HaveOccurred())

				mockStore.SaveDesiredErr = errors.New("save failed")

				s := supervisor.NewSupervisor(supervisor.Config{
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

				s.SetRestartCount(3)

				err = s.Tick(context.Background())
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

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(s.GetRestartCount()).To(Equal(0))
			})
		})
	})
})

type mockAction struct {
	executeFunc func(ctx context.Context) error
	name        string
}

func (m *mockAction) Execute(ctx context.Context) error {
	if m.executeFunc != nil {
		return m.executeFunc(ctx)
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
				err = mockStore.SaveDesired(context.Background(), "container", identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor(supervisor.Config{
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
					_ = s.Tick(context.Background())
				}()

				Expect(panicMessage).To(ContainSubstring("Invariant I16 violated"))
				Expect(panicMessage).To(ContainSubstring("test-worker"))
				Expect(panicMessage).To(ContainSubstring("alternateObservedState"))
				Expect(panicMessage).To(ContainSubstring("mockObservedState"))
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
				err = mockStore.SaveDesired(context.Background(), "container", identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				s := supervisor.NewSupervisor(supervisor.Config{
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
					_ = s.Tick(context.Background())
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

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when worker returns pointer type consistently (I16 normalization test)", func() {
			It("should not panic due to pointer vs struct type mismatch", func() {
				worker := &mockWorker{
					observed: &mockObservedState{ID: "test-worker", CollectedAt: time.Now()},
				}

				s := newSupervisorWithWorker(worker, nil, supervisor.CollectorHealthConfig{})

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
