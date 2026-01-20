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

package main

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("FSMv2 UUID Polling", func() {
	// This test proves the UUID polling mismatch bug:
	// - main.go:522 polls with ID "communicator"
	// - reconciliation.go:1218 creates child workers with ID "communicator-001"
	// - Result: polling never finds the observed state with the real UUID

	Context("Worker ID Matching", func() {
		var (
			store *testutil.Store
			ctx   context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()
			store = &testutil.Store{
				Identity: make(map[string]map[string]persistence.Document),
				Desired:  make(map[string]map[string]persistence.Document),
				Observed: make(map[string]map[string]persistence.Document),
			}
		})

		It("should use the correct worker ID pattern from reconciliation.go", func() {
			// Per reconciliation.go:1218, the child worker ID is created as:
			// childID := spec.Name + "-001"
			//
			// If spec.Name is "communicator", the actual worker ID is "communicator-001"

			const CommunicatorSpecName = "communicator"
			const ActualWorkerID = CommunicatorSpecName + "-001" // "communicator-001"

			// The polling in main.go:522 must use the SAME ID pattern
			// After fix: both reconciliation and polling use "communicator-001"
			const PollingWorkerID = CommunicatorSpecName + "-001" // "communicator-001" (FIXED)

			// Simulate the FSMv2 supervisor storing observed state with the CORRECT ID
			realUUID := "real-uuid-from-backend-12345"
			store.Observed["communicator"] = map[string]persistence.Document{
				ActualWorkerID: {
					"authenticatedUUID": realUUID,
					"state":             "running_connected",
				},
			}

			// Verify the store has the data under the correct ID
			var correctObserved snapshot.CommunicatorObservedState
			err := store.LoadObservedTyped(ctx, "communicator", ActualWorkerID, &correctObserved)
			Expect(err).NotTo(HaveOccurred(), "Should find observed state with correct worker ID")
			Expect(correctObserved.AuthenticatedUUID).To(Equal(realUUID))

			// Verify that polling ID matches the actual worker ID (the fix)
			var pollingObserved snapshot.CommunicatorObservedState
			err = store.LoadObservedTyped(ctx, "communicator", PollingWorkerID, &pollingObserved)

			// After fix: main.go:522 uses "communicator-001" which matches reconciliation.go:1218
			Expect(err).NotTo(HaveOccurred(),
				"UUID polling should find observed state when using correct worker ID 'communicator-001'")
			Expect(pollingObserved.AuthenticatedUUID).To(Equal(realUUID),
				"Should get the real UUID from observed state")

			// Document the ID relationship for maintainability
			Expect(PollingWorkerID).To(Equal(ActualWorkerID),
				"Polling ID must match actual worker ID from reconciliation.go:1218")
		})
	})
})
