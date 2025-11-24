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

package storage_test

import (
	"fmt"
	"reflect"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
)

type ParentObservedState struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type ParentDesiredState struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

var _ = Describe("TypeRegistry", func() {
	var registry *storage.TypeRegistry

	BeforeEach(func() {
		registry = storage.NewTypeRegistry()
	})

	Describe("RegisterWorkerType", func() {
		It("stores type metadata for worker", func() {
			observedType := reflect.TypeOf((*ParentObservedState)(nil)).Elem()
			desiredType := reflect.TypeOf((*ParentDesiredState)(nil)).Elem()

			err := registry.RegisterWorkerType("parent", observedType, desiredType)

			Expect(err).NotTo(HaveOccurred())
			retrievedObserved := registry.GetObservedType("parent")
			Expect(retrievedObserved).To(Equal(observedType))
			retrievedDesired := registry.GetDesiredType("parent")
			Expect(retrievedDesired).To(Equal(desiredType))
		})

		It("rejects duplicate worker type registration", func() {
			observedType := reflect.TypeOf((*ParentObservedState)(nil)).Elem()
			desiredType := reflect.TypeOf((*ParentDesiredState)(nil)).Elem()

			err := registry.RegisterWorkerType("parent", observedType, desiredType)
			Expect(err).NotTo(HaveOccurred())

			err = registry.RegisterWorkerType("parent", observedType, desiredType)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already registered"))
		})

		It("handles concurrent registration safely", func() {
			var wg sync.WaitGroup
			errors := make([]error, 10)

			for i := range 10 {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					workerType := fmt.Sprintf("worker%d", idx)
					observedType := reflect.TypeOf((*ParentObservedState)(nil)).Elem()
					desiredType := reflect.TypeOf((*ParentDesiredState)(nil)).Elem()
					errors[idx] = registry.RegisterWorkerType(workerType, observedType, desiredType)
				}(i)
			}

			wg.Wait()

			for i, err := range errors {
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("worker%d registration failed", i))
			}

			for i := range 10 {
				workerType := fmt.Sprintf("worker%d", i)
				observedType := registry.GetObservedType(workerType)
				Expect(observedType).NotTo(BeNil())
			}
		})
	})

	Describe("GetObservedType", func() {
		It("returns nil for unregistered worker type", func() {
			observedType := registry.GetObservedType("nonexistent")
			Expect(observedType).To(BeNil())
		})
	})
})
