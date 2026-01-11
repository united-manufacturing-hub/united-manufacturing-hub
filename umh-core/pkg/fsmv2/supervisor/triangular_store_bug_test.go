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
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("Triangular Store Bug", func() {
	It("should use supervisor's workerType when saving to triangular store", func() {
		ctx := context.Background()
		store := newMockStore()

		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: "test",
			Store:      store,
			Logger:     zap.NewNop().Sugar(),
		})

		identity := fsmv2.Identity{
			ID:         "worker1",
			Name:       "Test Worker 1",
			WorkerType: "",
		}

		worker := &mockWorker{
			observed: &mockObservedState{
				ID:          "worker1",
				CollectedAt: time.Now(),
				Desired:     &mockDesiredState{},
			},
		}

		err := s.AddWorker(identity, worker)
		Expect(err).NotTo(HaveOccurred())

		identityDoc, err := store.LoadIdentity(ctx, "test", "worker1")
		Expect(err).NotTo(HaveOccurred(), "Identity should be saved with workerType='test'")
		Expect(identityDoc).NotTo(BeNil())

		observed, err := store.LoadObserved(ctx, "test", "worker1")
		Expect(err).NotTo(HaveOccurred(), "Observed should be saved with workerType='test'")
		Expect(observed).NotTo(BeNil())

		desired, err := store.LoadDesired(ctx, "test", "worker1")
		Expect(err).NotTo(HaveOccurred(), "Desired should be saved with workerType='test'")
		Expect(desired).NotTo(BeNil())

		_, err = store.LoadIdentity(ctx, "", "worker1")
		Expect(err).To(HaveOccurred(), "Identity should NOT be saved with empty workerType")

		_, err = store.LoadObserved(ctx, "", "worker1")
		Expect(err).To(HaveOccurred(), "Observed should NOT be saved with empty workerType")

		_, err = store.LoadDesired(ctx, "", "worker1")
		Expect(err).To(HaveOccurred(), "Desired should NOT be saved with empty workerType")
	})
})
