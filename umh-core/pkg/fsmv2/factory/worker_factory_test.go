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

package factory_test

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
)

// mockWorker is a minimal Worker implementation for testing.
type mockWorker struct {
	identity deps.Identity
}

func (m *mockWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	return nil, nil
}

func (m *mockWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{}, nil
}

func (m *mockWorker) GetInitialState() fsmv2.State[any, any] {
	return nil
}

var _ = Describe("WorkerFactory", func() {
	BeforeEach(func() {
		factory.ResetRegistry()
	})

	Describe("RegisterWorkerType", func() {
		DescribeTable("should handle registration scenarios",
			func(workerType string, setupFunc func(), wantErr bool, errContains string) {
				if setupFunc != nil {
					setupFunc()
				}

				factoryFunc := func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &mockWorker{identity: id}
				}

				err := factory.RegisterFactoryByType(workerType, factoryFunc)

				if wantErr {
					Expect(err).To(HaveOccurred())
					if errContains != "" {
						Expect(err.Error()).To(ContainSubstring(errContains))
					}
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
			},
			Entry("register new worker type", "mqtt_client", nil, false, ""),
			Entry("register duplicate worker type", "mqtt_client", func() {
				_ = factory.RegisterFactoryByType("mqtt_client", func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
			}, true, "already registered"),
			Entry("register empty worker type", "", nil, true, "empty"),
		)
	})

	Describe("NewWorker", func() {
		BeforeEach(func() {
			err := factory.RegisterFactoryByType("test_worker", func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
				return &mockWorker{identity: id}
			})
			Expect(err).NotTo(HaveOccurred())
		})

		DescribeTable("should handle worker creation scenarios",
			func(workerType string, identity deps.Identity, wantErr bool, errContains string) {
				worker, err := factory.NewWorkerByType(workerType, identity, zap.NewNop().Sugar(), nil, nil)

				if wantErr {
					Expect(err).To(HaveOccurred())
					if errContains != "" {
						Expect(err.Error()).To(ContainSubstring(errContains))
					}
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(worker).NotTo(BeNil())
					mock, ok := worker.(*mockWorker)
					Expect(ok).To(BeTrue())
					Expect(mock.identity.ID).To(Equal(identity.ID))
				}
			},
			Entry("create registered worker type", "test_worker", deps.Identity{
				ID:         "test-123",
				Name:       "Test Worker",
				WorkerType: "test_worker",
			}, false, ""),
			Entry("create unknown worker type", "unknown_worker", deps.Identity{
				ID:         "test-456",
				Name:       "Unknown",
				WorkerType: "unknown_worker",
			}, true, "unknown worker type"),
			Entry("create with empty worker type", "", deps.Identity{
				ID:         "test-789",
				Name:       "Empty",
				WorkerType: "",
			}, true, "empty"),
		)
	})

	Describe("ConcurrentRegistration", func() {
		It("should handle concurrent registration correctly", func() {
			const numGoroutines = 10
			const numWorkerTypes = 5

			var wg sync.WaitGroup
			errors := make(chan error, numGoroutines*numWorkerTypes)

			for i := range numGoroutines {
				wg.Add(1)
				go func(goroutineID int) {
					defer wg.Done()

					for j := range numWorkerTypes {
						workerType := "worker_" + string(rune('A'+j))
						err := factory.RegisterFactoryByType(workerType, func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
							return &mockWorker{identity: id}
						})
						if err != nil {
							errors <- err
						}
					}
				}(i)
			}

			wg.Wait()
			close(errors)

			errorCount := 0
			for range errors {
				errorCount++
			}

			expectedErrors := (numGoroutines - 1) * numWorkerTypes
			Expect(errorCount).To(Equal(expectedErrors), "indicates missing mutex protection")
		})
	})

	Describe("ConcurrentCreation", func() {
		BeforeEach(func() {
			for i := range 3 {
				workerType := "concurrent_worker_" + string(rune('A'+i))
				err := factory.RegisterFactoryByType(workerType, func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("should handle concurrent worker creation", func() {
			const numGoroutines = 20

			var wg sync.WaitGroup
			errors := make(chan error, numGoroutines)
			workers := make(chan fsmv2.Worker, numGoroutines)

			for i := range numGoroutines {
				wg.Add(1)
				go func(goroutineID int) {
					defer wg.Done()

					workerType := fmt.Sprintf("concurrent_worker_%c", 'A'+(goroutineID%3))
					identity := deps.Identity{
						ID:         fmt.Sprintf("worker-%d", goroutineID),
						Name:       "Concurrent Worker",
						WorkerType: workerType,
					}

					worker, err := factory.NewWorkerByType(workerType, identity, zap.NewNop().Sugar(), nil, nil)
					if err != nil {
						errors <- err
					} else {
						workers <- worker
					}
				}(i)
			}

			wg.Wait()
			close(errors)
			close(workers)

			errorCount := 0
			for range errors {
				errorCount++
			}
			Expect(errorCount).To(Equal(0), "indicates race condition")

			workerCount := 0
			for range workers {
				workerCount++
			}
			Expect(workerCount).To(Equal(numGoroutines))
		})
	})

	Describe("ListRegisteredTypes", func() {
		It("should return empty slice for empty registry", func() {
			types := factory.ListRegisteredTypes()

			Expect(types).NotTo(BeNil())
			Expect(types).To(BeEmpty())
		})

		It("should return single type after one registration", func() {
			err := factory.RegisterFactoryByType("mqtt_client", func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
				return &mockWorker{identity: id}
			})
			Expect(err).NotTo(HaveOccurred())

			types := factory.ListRegisteredTypes()

			Expect(types).To(HaveLen(1))
			Expect(types[0]).To(Equal("mqtt_client"))
		})

		It("should return all types after multiple registrations", func() {
			workerTypes := []string{"mqtt_client", "modbus_server", "opcua_client"}

			for _, wt := range workerTypes {
				err := factory.RegisterFactoryByType(wt, func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
				Expect(err).NotTo(HaveOccurred())
			}

			types := factory.ListRegisteredTypes()

			Expect(types).To(HaveLen(len(workerTypes)))
			for _, wt := range workerTypes {
				Expect(types).To(ContainElement(wt))
			}
		})

		It("should return a copy of the slice", func() {
			err := factory.RegisterFactoryByType("test_worker", func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
				return &mockWorker{identity: id}
			})
			Expect(err).NotTo(HaveOccurred())

			types1 := factory.ListRegisteredTypes()
			if len(types1) > 0 {
				types1[0] = "modified"
			}

			types2 := factory.ListRegisteredTypes()

			Expect(types2[0]).NotTo(Equal("modified"))
			Expect(types2[0]).To(Equal("test_worker"))
		})

		It("should handle concurrent calls", func() {
			for i := range 5 {
				workerType := "worker_" + string(rune('A'+i))
				err := factory.RegisterFactoryByType(workerType, func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
				Expect(err).NotTo(HaveOccurred())
			}

			const numGoroutines = 20

			var wg sync.WaitGroup
			results := make(chan []string, numGoroutines)

			for i := range numGoroutines {
				wg.Add(1)
				go func(goroutineID int) {
					defer wg.Done()
					types := factory.ListRegisteredTypes()
					results <- types
				}(i)
			}

			wg.Wait()
			close(results)

			for result := range results {
				Expect(result).To(HaveLen(5))
			}
		})
	})

	Describe("RegisterWorkerAndSupervisorFactory", func() {
		DescribeTable("should handle combined registration scenarios",
			func(setupRegistry func(), wantErr bool, errContains string, verifyRollback bool) {
				if setupRegistry != nil {
					setupRegistry()
				}

				err := factory.RegisterWorkerAndSupervisorFactoryByType(
					"test_worker",
					func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
						return &mockWorker{identity: id}
					},
					func(cfg interface{}) interface{} {
						return nil
					},
				)

				if wantErr {
					Expect(err).To(HaveOccurred())
					if errContains != "" {
						Expect(err.Error()).To(ContainSubstring(errContains))
					}

					if verifyRollback {
						types := factory.ListRegisteredTypes()
						for _, typ := range types {
							Expect(typ).NotTo(Equal("test_worker"), "Worker factory was not rolled back")
						}
					}
				} else {
					Expect(err).NotTo(HaveOccurred())

					types := factory.ListRegisteredTypes()
					Expect(types).To(ContainElement("test_worker"))

					supervisorTypes := factory.ListSupervisorTypes()
					Expect(supervisorTypes).To(ContainElement("test_worker"))
				}
			},
			Entry("register both worker and supervisor successfully", nil, false, "", false),
			Entry("fail when worker already registered", func() {
				_ = factory.RegisterFactoryByType("test_worker", func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
			}, true, "failed to register worker factory", false),
			Entry("fail when supervisor already registered and rollback worker", func() {
				_ = factory.RegisterSupervisorFactoryByType("test_worker", func(cfg interface{}) interface{} {
					return nil
				})
			}, true, "failed to register supervisor factory", true),
		)
	})

	Describe("ValidateRegistryConsistency", func() {
		DescribeTable("should detect registry inconsistencies",
			func(setupRegistry func(), wantWorkerOnly []string, wantSupervisorOnly []string) {
				setupRegistry()

				workerOnly, supervisorOnly := factory.ValidateRegistryConsistency()

				Expect(workerOnly).To(HaveLen(len(wantWorkerOnly)))
				for _, want := range wantWorkerOnly {
					Expect(workerOnly).To(ContainElement(want))
				}

				Expect(supervisorOnly).To(HaveLen(len(wantSupervisorOnly)))
				for _, want := range wantSupervisorOnly {
					Expect(supervisorOnly).To(ContainElement(want))
				}
			},
			Entry("empty registries", func() {
				factory.ResetRegistry()
			}, []string{}, []string{}),
			Entry("consistent registries", func() {
				factory.ResetRegistry()
				_ = factory.RegisterFactoryByType("worker_a", func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
				_ = factory.RegisterSupervisorFactoryByType("worker_a", func(cfg interface{}) interface{} {
					return nil
				})
			}, []string{}, []string{}),
			Entry("worker registered but not supervisor", func() {
				factory.ResetRegistry()
				_ = factory.RegisterFactoryByType("orphan_worker", func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
			}, []string{"orphan_worker"}, []string{}),
			Entry("supervisor registered but not worker", func() {
				factory.ResetRegistry()
				_ = factory.RegisterSupervisorFactoryByType("orphan_supervisor", func(cfg interface{}) interface{} {
					return nil
				})
			}, []string{}, []string{"orphan_supervisor"}),
			Entry("mixed inconsistencies", func() {
				factory.ResetRegistry()
				_ = factory.RegisterFactoryByType("consistent", func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
				_ = factory.RegisterSupervisorFactoryByType("consistent", func(cfg interface{}) interface{} {
					return nil
				})
				_ = factory.RegisterFactoryByType("worker_only", func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &mockWorker{identity: id}
				})
				_ = factory.RegisterSupervisorFactoryByType("supervisor_only", func(cfg interface{}) interface{} {
					return nil
				})
			}, []string{"worker_only"}, []string{"supervisor_only"}),
		)
	})

	Describe("ListSupervisorTypes", func() {
		DescribeTable("should list supervisor types correctly",
			func(setupRegistry func(), wantTypes []string) {
				setupRegistry()

				types := factory.ListSupervisorTypes()

				Expect(types).To(HaveLen(len(wantTypes)))
				for _, want := range wantTypes {
					Expect(types).To(ContainElement(want))
				}
			},
			Entry("empty registry", func() {
				factory.ResetRegistry()
			}, []string{}),
			Entry("single supervisor", func() {
				factory.ResetRegistry()
				_ = factory.RegisterSupervisorFactoryByType("supervisor_a", func(cfg interface{}) interface{} {
					return nil
				})
			}, []string{"supervisor_a"}),
			Entry("multiple supervisors", func() {
				factory.ResetRegistry()
				_ = factory.RegisterSupervisorFactoryByType("supervisor_a", func(cfg interface{}) interface{} {
					return nil
				})
				_ = factory.RegisterSupervisorFactoryByType("supervisor_b", func(cfg interface{}) interface{} {
					return nil
				})
				_ = factory.RegisterSupervisorFactoryByType("supervisor_c", func(cfg interface{}) interface{} {
					return nil
				})
			}, []string{"supervisor_a", "supervisor_b", "supervisor_c"}),
		)
	})
})
