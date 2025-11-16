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

package supervisor

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
	"go.uber.org/zap"
)

func setupTestStore(workerType string) *storage.TriangularStore {
	ctx := context.Background()
	basicStore := memory.NewInMemoryStore()

	_ = basicStore.CreateCollection(ctx, workerType+"_identity", nil)
	_ = basicStore.CreateCollection(ctx, workerType+"_desired", nil)
	_ = basicStore.CreateCollection(ctx, workerType+"_observed", nil)

	return storage.NewTriangularStore(basicStore)
}

var _ = Describe("Supervisor Configuration", func() {
	Context("when creating supervisor with default config", func() {
		It("should use default collector health thresholds", func() {
			cfg := Config{
				WorkerType: "container",
				Store:      setupTestStore("container"),
				Logger:     zap.NewNop().Sugar(),
			}

			s := NewSupervisor(cfg)

			Expect(s.getStaleThreshold()).To(Equal(10 * time.Second))
			Expect(s.getCollectorTimeout()).To(Equal(20 * time.Second))
			Expect(s.getMaxRestartAttempts()).To(Equal(3))
		})
	})

	Context("when creating supervisor with custom config", func() {
		It("should use custom collector health thresholds", func() {
			cfg := Config{
				WorkerType: "container",
				Store:      setupTestStore("container"),
				Logger:     zap.NewNop().Sugar(),
				CollectorHealth: CollectorHealthConfig{
					StaleThreshold:     5 * time.Second,
					Timeout:            15 * time.Second,
					MaxRestartAttempts: 5,
				},
			}

			s := NewSupervisor(cfg)

			Expect(s.getStaleThreshold()).To(Equal(5 * time.Second))
			Expect(s.getCollectorTimeout()).To(Equal(15 * time.Second))
			Expect(s.getMaxRestartAttempts()).To(Equal(5))
		})
	})

	Context("when creating supervisor with invalid config", func() {
		It("should panic when staleThreshold is negative", func() {
			Expect(func() {
				NewSupervisor(Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: CollectorHealthConfig{
						StaleThreshold:     -5 * time.Second,
						Timeout:            20 * time.Second,
						MaxRestartAttempts: 3,
					},
				})
			}).To(Panic())
		})

		It("should panic when timeout equals staleThreshold", func() {
			Expect(func() {
				NewSupervisor(Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: CollectorHealthConfig{
						StaleThreshold:     10 * time.Second,
						Timeout:            10 * time.Second,
						MaxRestartAttempts: 3,
					},
				})
			}).To(Panic())
		})

		It("should panic when timeout is less than staleThreshold", func() {
			Expect(func() {
				NewSupervisor(Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: CollectorHealthConfig{
						StaleThreshold:     20 * time.Second,
						Timeout:            10 * time.Second,
						MaxRestartAttempts: 3,
					},
				})
			}).To(Panic())
		})

		It("should panic when maxRestartAttempts is negative", func() {
			Expect(func() {
				NewSupervisor(Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: CollectorHealthConfig{
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
				NewSupervisor(Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: CollectorHealthConfig{
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
				NewSupervisor(Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: CollectorHealthConfig{
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
				NewSupervisor(Config{
					WorkerType: "container",
					Store:      setupTestStore("container"),
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: CollectorHealthConfig{
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
