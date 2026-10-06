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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var errStoreFailedOnce = errors.New("store failed once")

// failOnceStore fails the first SaveDesired and passes every other call
// through.
type failOnceStore struct {
	storage.TriangularStoreInterface

	failed bool
}

func (f *failOnceStore) SaveDesired(ctx context.Context, workerType string, id string, desired persistence.Document) (bool, error) {
	if !f.failed {
		f.failed = true

		return false, errStoreFailedOnce
	}

	return f.TriangularStoreInterface.SaveDesired(ctx, workerType, id, desired)
}

var _ = Describe("StartupCount after a failed AddWorker", func() {
	It("stays at 1 when the first AddWorker fails on SaveDesired and the retry succeeds", func() {
		const (
			workerType = "startupcount-failed-desired"
			workerID   = "startup-count-worker"
		)

		store := &failOnceStore{
			TriangularStoreInterface: supervisor.CreateTestTriangularStoreForWorkerType(workerType),
		}

		sup := supervisor.NewSupervisor[fsmv2.Observation[startupCountStatus], *config.DesiredState](supervisor.Config{
			WorkerType: workerType,
			Store:      store,
			Logger:     deps.NewNopFSMLogger(),
		})

		identity := deps.Identity{ID: workerID, Name: "Startup Count Worker", WorkerType: workerType}

		Expect(errors.Is(sup.AddWorker(identity, &startupCountWorker{}), errStoreFailedOnce)).To(BeTrue(),
			"the first AddWorker must fail on the injected store error")
		Expect(sup.ListWorkers()).To(BeEmpty())

		Expect(sup.AddWorker(identity, &startupCountWorker{})).To(Succeed())

		var stored fsmv2.Observation[startupCountStatus]
		Expect(store.LoadObservedTyped(context.Background(), workerType, workerID, &stored)).To(Succeed())
		Expect(stored.Metrics.Framework.StartupCount).To(Equal(int64(1)),
			"a failed AddWorker never started the worker, so it must not count as a startup")
	})
})
