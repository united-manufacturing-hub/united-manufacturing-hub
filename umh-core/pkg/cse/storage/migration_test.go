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
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("Storage Migration Scenarios", func() {
	var (
		ctx   context.Context
		store persistence.Store
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newMockStore()
	})

	Context("Field Addition Migration", func() {
		type OldVersion struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		}

		type NewVersion struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Status   string `json:"status"`
			NewField string `json:"newField"` // Added field
		}

		It("should load old data into new struct with added field", func() {
			// Save data with old struct
			oldData := OldVersion{
				ID:     "worker-123",
				Name:   "Test Worker",
				Status: "running",
			}

			// Manually insert old data (simulating old version of code)
			collectionName := "test_observed"
			docBytes, err := json.Marshal(oldData)
			Expect(err).NotTo(HaveOccurred())

			var doc persistence.Document
			err = json.Unmarshal(docBytes, &doc)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, collectionName, doc)
			Expect(err).NotTo(HaveOccurred())

			// Load with new struct
			var loaded NewVersion
			doc, err = store.Get(ctx, collectionName, "worker-123")
			Expect(err).NotTo(HaveOccurred())

			docBytes, err = json.Marshal(doc)
			Expect(err).NotTo(HaveOccurred())

			err = json.Unmarshal(docBytes, &loaded)
			Expect(err).NotTo(HaveOccurred())

			// Old fields should be preserved
			Expect(loaded.ID).To(Equal("worker-123"))
			Expect(loaded.Name).To(Equal("Test Worker"))
			Expect(loaded.Status).To(Equal("running"))
			// New field should be zero value
			Expect(loaded.NewField).To(Equal(""))
		})
	})

	Context("Field Removal Migration", func() {
		type OldVersion struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Status   string `json:"status"`
			OldField string `json:"oldField"` // Will be removed
		}

		type NewVersion struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
			// OldField removed
		}

		It("should load old data into new struct ignoring removed field", func() {
			// Save data with old struct
			oldData := OldVersion{
				ID:       "worker-456",
				Name:     "Test Worker 2",
				Status:   "stopped",
				OldField: "deprecated value",
			}

			collectionName := "test_observed"
			docBytes, err := json.Marshal(oldData)
			Expect(err).NotTo(HaveOccurred())

			var doc persistence.Document
			err = json.Unmarshal(docBytes, &doc)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, collectionName, doc)
			Expect(err).NotTo(HaveOccurred())

			// Load with new struct
			var loaded NewVersion
			doc, err = store.Get(ctx, collectionName, "worker-456")
			Expect(err).NotTo(HaveOccurred())

			docBytes, err = json.Marshal(doc)
			Expect(err).NotTo(HaveOccurred())

			err = json.Unmarshal(docBytes, &loaded)
			Expect(err).NotTo(HaveOccurred())

			// Kept fields should be preserved
			Expect(loaded.ID).To(Equal("worker-456"))
			Expect(loaded.Name).To(Equal("Test Worker 2"))
			Expect(loaded.Status).To(Equal("stopped"))
			// Old field is ignored (not an error)
		})
	})

	Context("Field Rename Migration (using JSON tags)", func() {
		type OldVersion struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			OldStatus string `json:"status"` // Old field name
		}

		type NewVersion struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			NewStatus string `json:"status"` // New field name, same JSON tag
		}

		It("should load old data into renamed field via JSON tag", func() {
			// Save data with old struct
			oldData := OldVersion{
				ID:        "worker-789",
				Name:      "Test Worker 3",
				OldStatus: "active",
			}

			collectionName := "test_observed"
			docBytes, err := json.Marshal(oldData)
			Expect(err).NotTo(HaveOccurred())

			var doc persistence.Document
			err = json.Unmarshal(docBytes, &doc)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, collectionName, doc)
			Expect(err).NotTo(HaveOccurred())

			// Load with new struct
			var loaded NewVersion
			doc, err = store.Get(ctx, collectionName, "worker-789")
			Expect(err).NotTo(HaveOccurred())

			docBytes, err = json.Marshal(doc)
			Expect(err).NotTo(HaveOccurred())

			err = json.Unmarshal(docBytes, &loaded)
			Expect(err).NotTo(HaveOccurred())

			// Field renamed in struct but JSON tag unchanged
			Expect(loaded.ID).To(Equal("worker-789"))
			Expect(loaded.Name).To(Equal("Test Worker 3"))
			Expect(loaded.NewStatus).To(Equal("active"))
		})
	})
})
