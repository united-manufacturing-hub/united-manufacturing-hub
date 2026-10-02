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

package integration_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("Application supervisor records a removed worker in the store", func() {
	const configWorkerKey = "configworker"

	AfterEach(func() {
		register.ClearDeps(configWorkerKey)
	})

	storedRoleDocuments := func(ctx context.Context, store *storage.TriangularStore, workerType, id string) map[string]persistence.Document {
		identity, err := store.LoadIdentity(ctx, workerType, id)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		desired, err := store.LoadDesired(ctx, workerType, id) //nolint:staticcheck // the helper is polymorphic over worker types, so LoadDesiredTyped[T] cannot apply
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		desiredDoc, ok := desired.(persistence.Document)
		ExpectWithOffset(1, ok).To(BeTrue(), "desired must load as a document, got %T", desired)

		observed, err := store.LoadObserved(ctx, workerType, id) //nolint:staticcheck // the helper is polymorphic over worker types, so LoadObservedTyped[T] cannot apply
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		observedDoc, ok := observed.(persistence.Document)
		ExpectWithOffset(1, ok).To(BeTrue(), "observed must load as a document, got %T", observed)

		return map[string]persistence.Document{
			storage.RoleIdentity: identity,
			storage.RoleDesired:  desiredDoc,
			storage.RoleObserved: observedDoc,
		}
	}

	It("tombstones a removed worker's documents and clears the tombstone when the worker is added again", func() {
		ctx := context.Background()
		logger := deps.NewNopFSMLogger()

		w := dynamicchildren.NewWriter()
		register.SetDeps[*dynamicchildren.Registry](configWorkerKey, w.Registry())

		ref := dynamicchildren.Ref{WorkerType: "helloworld", Name: "hello-1"}
		childID := config.ChildID(ref.Name)

		Expect(w.Upsert(ref, map[string]any{"state": "running"})).To(Succeed())

		sup, store, _ := newAppSupervisorWithStore(logger)
		sup.TestMarkAsStarted()

		childRunning := func() bool {
			_ = sup.TestTick(ctx)

			child, ok := sup.GetChildren()[ref.Name]

			return ok && child != nil && childStateName(child) == "Running"
		}

		Eventually(childRunning, "5s", "100ms").Should(BeTrue(),
			"the helloworld child must first spawn and reach Running")

		for role, doc := range storedRoleDocuments(ctx, store, ref.WorkerType, childID) {
			Expect(doc).NotTo(HaveKey(storage.FieldDeletedAt), "a running worker's %s document must not carry a tombstone", role)
		}

		By("deleting the ref: the child is removed and its documents are tombstoned, not deleted")
		w.Delete(ref)

		Eventually(func() bool {
			_ = sup.TestTick(ctx)

			_, ok := sup.GetChildren()[ref.Name]

			return !ok
		}, "5s", "100ms").Should(BeTrue(), "the helloworld child must be removed once its ref is deleted")

		var deletedAt time.Time

		for role, doc := range storedRoleDocuments(ctx, store, ref.WorkerType, childID) {
			Expect(doc).To(HaveKey(storage.FieldDeletedAt), "the removed worker's %s document must carry a tombstone", role)
			Expect(doc[storage.FieldDeletedBy]).To(Equal("supervisor"))

			stamp, ok := doc[storage.FieldDeletedAt].(time.Time)
			Expect(ok).To(BeTrue(), "%s must be a time.Time", storage.FieldDeletedAt)

			deletedAt = stamp
		}

		By("adding the ref again: the new child's documents carry no tombstone and a newer observation")
		Expect(w.Upsert(ref, map[string]any{"state": "running"})).To(Succeed())

		Eventually(childRunning, "5s", "100ms").Should(BeTrue(),
			"the helloworld child must spawn again and reach Running")

		Eventually(func() bool {
			_ = sup.TestTick(ctx)

			docs := storedRoleDocuments(ctx, store, ref.WorkerType, childID)
			for _, doc := range docs {
				if _, marked := doc[storage.FieldDeletedAt]; marked {
					return false
				}
			}

			collectedAt, err := time.Parse(time.RFC3339Nano, docs[storage.RoleObserved]["collected_at"].(string))

			return err == nil && collectedAt.After(deletedAt)
		}, "5s", "100ms").Should(BeTrue(),
			"the worker added again must have documents without a tombstone and an observation taken after the removal")
	})
})
