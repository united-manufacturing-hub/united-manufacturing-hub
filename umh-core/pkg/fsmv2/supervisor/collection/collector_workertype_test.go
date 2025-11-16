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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"go.uber.org/zap"
)

var _ = Describe("Collector WorkerType", func() {
	Context("when collector is configured with workerType", func() {
		It("should use configured workerType when saving", func() {
			ctx := context.Background()

			workerType := "s6"
			identity := fsmv2.Identity{
				ID:         "test-s6-worker",
				Name:       "S6 Worker",
				WorkerType: workerType,
			}

			triangularStore := supervisor.CreateTestTriangularStoreForWorkerType(workerType)
			Expect(triangularStore).ToNot(BeNil())

			s := supervisor.NewSupervisor(supervisor.Config{
				WorkerType:      workerType,
				Store:           triangularStore,
				Logger:          zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{},
			})

			worker := &supervisor.TestWorkerWithType{
				TestWorker: supervisor.TestWorker{
					Observed: supervisor.CreateTestObservedStateWithID(identity.ID),
				},
				WorkerType: workerType,
			}

			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			observedDoc := persistence.Document{
				"id":                identity.ID,
				"shutdownRequested": false,
			}
			_, err = triangularStore.SaveObserved(ctx, workerType, identity.ID, observedDoc)
			Expect(err).ToNot(HaveOccurred())

			// Verify the correct workerType collection is used by loading the data back
			doc, err := triangularStore.LoadObserved(ctx, "s6", identity.ID)
			Expect(err).ToNot(HaveOccurred(), "should load observed state from s6_observed collection")
			Expect(doc).ToNot(BeNil())
			docMap, ok := doc.(persistence.Document)
			Expect(ok).To(BeTrue(), "LoadObserved should return persistence.Document")
			Expect(docMap["id"]).To(Equal(identity.ID))
			Expect(docMap["shutdownRequested"]).To(BeFalse())

			// Verify container_observed collection is not used (should fail to load)
			_, err = triangularStore.LoadObserved(ctx, "container", identity.ID)
			Expect(err).To(HaveOccurred(), "should not find data in container_observed collection")
		})
	})

	Context("when multiple collectors use different workerTypes", func() {
		It("should save to different collections for different workerTypes", func() {
			ctx := context.Background()

			workerTypes := []string{"s6", "benthos"}
			stores := make(map[string]*storage.TriangularStore)
			supervisors := make([]*supervisor.Supervisor, 0, len(workerTypes))

			for _, wt := range workerTypes {
				stores[wt] = supervisor.CreateTestTriangularStoreForWorkerType(wt)
			}

			for _, wt := range workerTypes {
				triangularStore := stores[wt]

				identity := fsmv2.Identity{
					ID:         wt + "-worker",
					Name:       wt + " Worker",
					WorkerType: wt,
				}

				s := supervisor.NewSupervisor(supervisor.Config{
					WorkerType:      wt,
					Store:           triangularStore,
					Logger:          zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{},
				})

				worker := &supervisor.TestWorkerWithType{
					TestWorker: supervisor.TestWorker{
						Observed: supervisor.CreateTestObservedStateWithID(identity.ID),
					},
					WorkerType: wt,
				}

				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				observedDoc := persistence.Document{
					"id":                identity.ID,
					"shutdownRequested": false,
				}
				_, err = triangularStore.SaveObserved(ctx, wt, identity.ID, observedDoc)
				Expect(err).ToNot(HaveOccurred())

				supervisors = append(supervisors, s)
			}

			Expect(supervisors).To(HaveLen(len(workerTypes)))

			for _, wt := range workerTypes {
				triangularStore := stores[wt]
				identity := fsmv2.Identity{
					ID:         wt + "-worker",
					Name:       wt + " Worker",
					WorkerType: wt,
				}

				doc, err := triangularStore.LoadObserved(ctx, wt, identity.ID)
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should load observed state from %s collection", wt))
				Expect(doc).ToNot(BeNil())
				docMap, ok := doc.(persistence.Document)
				Expect(ok).To(BeTrue(), "LoadObserved should return persistence.Document")
				Expect(docMap["id"]).To(Equal(identity.ID))
				Expect(docMap["shutdownRequested"]).To(BeFalse())

				otherWorkerType := "benthos"
				if wt == "benthos" {
					otherWorkerType = "s6"
				}
				_, err = triangularStore.LoadObserved(ctx, otherWorkerType, identity.ID)
				Expect(err).To(HaveOccurred(), "should not find data from wrong workerType collection")
			}
		})
	})
})
