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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("AddWorker error paths", func() {
	newSupervisor := func() (*supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState], *mockTriangularStore) {
		store := newMockTriangularStore()

		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              "test",
			Store:                   store,
			Logger:                  deps.NewNopFSMLogger(),
			GracefulShutdownTimeout: 100 * time.Millisecond,
		})

		return s, store
	}

	It("returns the derive error and writes no documents", func() {
		identity := mockIdentity()
		s, store := newSupervisor()

		deriveErr := errors.New("derive desired failed")
		addErr := s.AddWorker(identity, &mockWorker{deriveErr: deriveErr})

		Expect(addErr).To(HaveOccurred())
		Expect(errors.Is(addErr, deriveErr)).To(BeTrue(),
			"AddWorker must report the failing derive, not swallow it")
		Expect(s.ListWorkers()).To(BeEmpty())
		Expect(store.SaveAndClearCalls).To(BeEmpty())
	})

	It("returns the collect error and writes no documents", func() {
		identity := mockIdentity()
		s, store := newSupervisor()

		collectErr := errors.New("collect observed failed")
		addErr := s.AddWorker(identity, &mockWorker{collectErr: collectErr})

		Expect(addErr).To(HaveOccurred())
		Expect(errors.Is(addErr, collectErr)).To(BeTrue(),
			"AddWorker must report the failing collection, not swallow it")
		Expect(s.ListWorkers()).To(BeEmpty())
		Expect(store.SaveAndClearCalls).To(BeEmpty())
	})

	It("returns the identity-save error, saves nothing further and never clears the tombstone", func() {
		identity := mockIdentity()
		s, store := newSupervisor()
		store.SaveIdentityErr = errors.New("save identity failed")

		Expect(s.AddWorker(identity, &mockWorker{})).To(HaveOccurred())
		Expect(s.ListWorkers()).To(BeEmpty(),
			"a worker whose identity could not be saved must not be added")
		Expect(store.SaveAndClearCalls).To(BeEmpty(),
			"a failed identity save must not be followed by further writes")
		Expect(store.ClearTombstoneCalls).To(BeEmpty(),
			"the tombstone must not be cleared when a save failed")
	})
})
