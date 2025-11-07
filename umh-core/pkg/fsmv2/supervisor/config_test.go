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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
	"go.uber.org/zap"
)

func setupTestStore(workerType string) *storage.TriangularStore {
	ctx := context.Background()
	basicStore := memory.NewInMemoryStore()

	registry := storage.NewRegistry()
	registry.Register(&storage.CollectionMetadata{
		Name:          workerType + "_identity",
		WorkerType:    workerType,
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	registry.Register(&storage.CollectionMetadata{
		Name:          workerType + "_desired",
		WorkerType:    workerType,
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	registry.Register(&storage.CollectionMetadata{
		Name:          workerType + "_observed",
		WorkerType:    workerType,
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, workerType+"_identity", nil)
	_ = basicStore.CreateCollection(ctx, workerType+"_desired", nil)
	_ = basicStore.CreateCollection(ctx, workerType+"_observed", nil)

	return storage.NewTriangularStore(basicStore, registry)
}

var _ = Describe("Supervisor Configuration", func() {
	Context("when creating supervisor with default config", func() {
		It("should use default collector health thresholds", func() {
			cfg := supervisor.Config{
				WorkerType: "container",
				Store:      setupTestStore("container"),
				Logger:     zap.NewNop().Sugar(),
			}

			s := supervisor.NewSupervisor(cfg)

			Expect(s.GetStaleThreshold()).To(Equal(10 * time.Second))
			Expect(s.GetCollectorTimeout()).To(Equal(20 * time.Second))
			Expect(s.GetMaxRestartAttempts()).To(Equal(3))
		})
	})

	Context("when creating supervisor with custom config", func() {
		It("should use custom collector health thresholds", func() {
			cfg := supervisor.Config{
				WorkerType: "container",
				Store:      setupTestStore("container"),
				Logger:     zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{
					StaleThreshold:     5 * time.Second,
					Timeout:            15 * time.Second,
					MaxRestartAttempts: 5,
				},
			}

			s := supervisor.NewSupervisor(cfg)

			Expect(s.GetStaleThreshold()).To(Equal(5 * time.Second))
			Expect(s.GetCollectorTimeout()).To(Equal(15 * time.Second))
			Expect(s.GetMaxRestartAttempts()).To(Equal(5))
		})
	})

	Context("when creating supervisor with invalid config", func() {
		It("should panic when staleThreshold is negative", func() {
			Expect(func() {
				supervisor.NewSupervisor(supervisor.Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold:     -5 * time.Second,
						Timeout:            20 * time.Second,
						MaxRestartAttempts: 3,
					},
				})
			}).To(Panic())
		})

		It("should panic when timeout equals staleThreshold", func() {
			Expect(func() {
				supervisor.NewSupervisor(supervisor.Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold:     10 * time.Second,
						Timeout:            10 * time.Second,
						MaxRestartAttempts: 3,
					},
				})
			}).To(Panic())
		})

		It("should panic when timeout is less than staleThreshold", func() {
			Expect(func() {
				supervisor.NewSupervisor(supervisor.Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold:     20 * time.Second,
						Timeout:            10 * time.Second,
						MaxRestartAttempts: 3,
					},
				})
			}).To(Panic())
		})

		It("should panic when maxRestartAttempts is negative", func() {
			Expect(func() {
				supervisor.NewSupervisor(supervisor.Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold:     10 * time.Second,
						Timeout:            20 * time.Second,
						MaxRestartAttempts: -1,
					},
				})
			}).To(Panic())
		})
	})

	Context("timeout ordering validation (I7)", func() {
		It("should panic when ObservationTimeout >= StaleThreshold", func() {
			Expect(func() {
				supervisor.NewSupervisor(supervisor.Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						ObservationTimeout: 10 * time.Second,
						StaleThreshold:     10 * time.Second,
						Timeout:            20 * time.Second,
						MaxRestartAttempts: 3,
					},
				})
			}).To(Panic())
		})

		It("should panic when StaleThreshold >= CollectorTimeout", func() {
			Expect(func() {
				supervisor.NewSupervisor(supervisor.Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						ObservationTimeout: 1 * time.Second,
						StaleThreshold:     15 * time.Second,
						Timeout:            15 * time.Second,
						MaxRestartAttempts: 3,
					},
				})
			}).To(Panic())
		})

		It("should accept valid timeout ordering", func() {
			Expect(func() {
				supervisor.NewSupervisor(supervisor.Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						ObservationTimeout: 1 * time.Second,
						StaleThreshold:     5 * time.Second,
						Timeout:            10 * time.Second,
						MaxRestartAttempts: 3,
					},
				})
			}).NotTo(Panic())
		})
	})
})
