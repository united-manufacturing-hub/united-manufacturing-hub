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

package collection_test

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor/internal/collection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor/metrics"
)

const observationSaveMetric = "umh_fsmv2_observation_save_total"

func observationSaveSeries(hierarchyPath string) int {
	families, err := prometheus.DefaultGatherer.Gather()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	count := 0

	for _, family := range families {
		if family.GetName() != observationSaveMetric {
			continue
		}

		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "hierarchy_path" && label.GetValue() == hierarchyPath {
					count++
				}
			}
		}
	}

	return count
}

var _ = Describe("A collector whose worker was removed", func() {
	It("does not record the observation save", func() {
		identity := supervisor.TestIdentity()
		identity.HierarchyPath = "collector-removed-claim(" + identity.WorkerType + ")"

		removedClaim := metrics.ClaimWorkerMetrics(identity.HierarchyPath)
		metrics.CleanupWorkerMetrics(identity.HierarchyPath, removedClaim)

		collected := make(chan struct{})

		var collectedOnce sync.Once

		worker := &supervisor.TestWorker{
			CollectFunc: func(_ context.Context) (fsmv2.ObservedState, error) {
				collectedOnce.Do(func() { close(collected) })

				return supervisor.CreateTestObservedStateWithID(identity.ID), nil
			},
		}
		collector := collection.NewCollector[*supervisor.TestObservedState](collection.CollectorConfig[*supervisor.TestObservedState]{
			Worker:              worker,
			Identity:            identity,
			Store:               supervisor.CreateTestTriangularStore(),
			Logger:              deps.NewNopFSMLogger(),
			ObservationInterval: 10 * time.Millisecond,
			ObservationTimeout:  5 * time.Second,
			DesiredStateProvider: func() (fsmv2.DesiredState, error) {
				return &supervisor.TestDesiredState{}, nil
			},
			MetricsClaim: func() metrics.WorkerMetricsClaim { return removedClaim },
		})

		Expect(collector.Start(context.Background())).To(Succeed())
		DeferCleanup(collector.Stop, context.Background())

		Eventually(collected).Should(BeClosed())
		Consistently(func() int { return observationSaveSeries(identity.HierarchyPath) }, 300*time.Millisecond).Should(BeZero())
	})
})
