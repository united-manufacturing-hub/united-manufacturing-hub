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

package action_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/action"
)

var _ = Describe("CompactDeltasAction", func() {
	var (
		ctx       context.Context
		mockStore *mockTriangularStore
		d         *persistence.PersistenceDependencies
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &mockTriangularStore{}
		identity := deps.Identity{ID: "test-id", WorkerType: "persistence"}
		d = persistence.NewPersistenceDependencies(mockStore, deps.DefaultScheduler{}, deps.NewNopFSMLogger(), nil, identity)
	})

	Describe("Execute", func() {
		Context("when compaction succeeds", func() {
			It("should call CompactDeltas, set timestamp, record metrics, and return nil", func() {
				mockStore.compactDeltasResult = 42
				a := action.NewCompactDeltasAction(24 * time.Hour)

				err := a.Execute(ctx, d)
				Expect(err).NotTo(HaveOccurred())
				Expect(mockStore.compactDeltasCalled).To(BeTrue())
				Expect(mockStore.compactDeltasRetention).To(Equal(24 * time.Hour))
				Expect(d.GetLastCompactionAt()).NotTo(BeZero())

				drained := d.MetricsRecorder().Drain()
				Expect(drained.Counters).To(HaveKeyWithValue(
					string(deps.CounterCompactionDeltasDeletedTotal), int64(42)))
				Expect(drained.Counters).To(HaveKeyWithValue(
					string(deps.CounterCompactionCyclesTotal), int64(1)))
				Expect(drained.Gauges).To(HaveKey(
					string(deps.GaugeLastCompactionDurationMs)))
			})
		})

		Context("when compaction fails", func() {
			It("should return error and NOT set timestamp", func() {
				mockStore.compactDeltasErr = errors.New("disk full")
				a := action.NewCompactDeltasAction(24 * time.Hour)

				err := a.Execute(ctx, d)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("compaction failed"))
				Expect(d.GetLastCompactionAt()).To(BeZero())
			})
		})

		Context("when context is cancelled", func() {
			It("should return context error", func() {
				cancelledCtx, cancel := context.WithCancel(ctx)
				cancel()

				a := action.NewCompactDeltasAction(24 * time.Hour)
				err := a.Execute(cancelledCtx, d)
				Expect(err).To(Equal(context.Canceled))
				Expect(mockStore.compactDeltasCalled).To(BeFalse())
			})
		})
	})

	Describe("Name", func() {
		It("should return CompactDeltas", func() {
			a := action.NewCompactDeltasAction(time.Hour)
			Expect(a.Name()).To(Equal("CompactDeltas"))
		})
	})
})
