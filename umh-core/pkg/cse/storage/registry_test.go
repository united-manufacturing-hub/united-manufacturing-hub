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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
)

var _ = Describe("Registry", func() {
	var registry *storage.Registry

	BeforeEach(func() {
		registry = storage.NewRegistry()
	})

	Describe("NewRegistry", func() {
		It("should create a non-nil registry", func() {
			Expect(registry).NotTo(BeNil())
		})

		It("should start with empty collections", func() {
			collections := registry.List()
			Expect(collections).To(BeEmpty())
		})
	})

	Describe("Register", func() {
		It("should register a collection successfully", func() {
			metadata := &storage.CollectionMetadata{
				Name:          "container_identity",
				WorkerType:    "container",
				Role:          storage.RoleIdentity,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion},
				IndexedFields: []string{storage.FieldSyncID},
			}

			err := registry.Register(metadata)
			Expect(err).NotTo(HaveOccurred())
			Expect(registry.IsRegistered("container_identity")).To(BeTrue())
		})

		Context("when collection already registered", func() {
			BeforeEach(func() {
				metadata := &storage.CollectionMetadata{
					Name:       "container_identity",
					WorkerType: "container",
					Role:       storage.RoleIdentity,
				}
				registry.Register(metadata)
			})

			It("should return error", func() {
				metadata := &storage.CollectionMetadata{
					Name:       "container_identity",
					WorkerType: "container",
					Role:       storage.RoleIdentity,
				}

				err := registry.Register(metadata)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with invalid metadata", func() {
			It("should return error for nil metadata", func() {
				err := registry.Register(nil)
				Expect(err).To(HaveOccurred())
			})

			It("should return error for empty collection name", func() {
				metadata := &storage.CollectionMetadata{
					Name:       "",
					WorkerType: "container",
					Role:       storage.RoleIdentity,
				}

				err := registry.Register(metadata)
				Expect(err).To(HaveOccurred())
			})

			It("should return error for empty worker type", func() {
				metadata := &storage.CollectionMetadata{
					Name:       "container_identity",
					WorkerType: "",
					Role:       storage.RoleIdentity,
				}

				err := registry.Register(metadata)
				Expect(err).To(HaveOccurred())
			})

			It("should return error for invalid role", func() {
				metadata := &storage.CollectionMetadata{
					Name:       "container_identity",
					WorkerType: "container",
					Role:       "invalid_role",
				}

				err := registry.Register(metadata)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Get", func() {
		BeforeEach(func() {
			metadata := &storage.CollectionMetadata{
				Name:       "container_identity",
				WorkerType: "container",
				Role:       storage.RoleIdentity,
			}
			registry.Register(metadata)
		})

		It("should retrieve registered collection", func() {
			retrieved, err := registry.Get("container_identity")
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved.Name).To(Equal("container_identity"))
		})

		It("should return error for nonexistent collection", func() {
			_, err := registry.Get("nonexistent")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetTriangularCollections", func() {
		BeforeEach(func() {
			registry.Register(&storage.CollectionMetadata{
				Name:       "container_identity",
				WorkerType: "container",
				Role:       storage.RoleIdentity,
			})
			registry.Register(&storage.CollectionMetadata{
				Name:       "container_desired",
				WorkerType: "container",
				Role:       storage.RoleDesired,
			})
			registry.Register(&storage.CollectionMetadata{
				Name:       "container_observed",
				WorkerType: "container",
				Role:       storage.RoleObserved,
			})
		})

		It("should retrieve all three triangular collections", func() {
			identity, desired, observed, err := registry.GetTriangularCollections("container")
			Expect(err).NotTo(HaveOccurred())

			Expect(identity.Name).To(Equal("container_identity"))
			Expect(desired.Name).To(Equal("container_desired"))
			Expect(observed.Name).To(Equal("container_observed"))
		})

		Context("when collections are missing", func() {
			It("should return error when only identity registered", func() {
				registry := storage.NewRegistry()
				registry.Register(&storage.CollectionMetadata{
					Name:       "container_identity",
					WorkerType: "container",
					Role:       storage.RoleIdentity,
				})

				_, _, _, err := registry.GetTriangularCollections("container")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("List", func() {
		It("should return all registered collections", func() {
			registry.Register(&storage.CollectionMetadata{
				Name:       "container_identity",
				WorkerType: "container",
				Role:       storage.RoleIdentity,
			})
			registry.Register(&storage.CollectionMetadata{
				Name:       "container_desired",
				WorkerType: "container",
				Role:       storage.RoleDesired,
			})

			collections := registry.List()
			Expect(collections).To(HaveLen(2))
		})
	})

	Describe("IsRegistered", func() {
		It("should return false for unregistered collection", func() {
			Expect(registry.IsRegistered("nonexistent")).To(BeFalse())
		})

		It("should return true for registered collection", func() {
			registry.Register(&storage.CollectionMetadata{
				Name:       "container_identity",
				WorkerType: "container",
				Role:       storage.RoleIdentity,
			})

			Expect(registry.IsRegistered("container_identity")).To(BeTrue())
		})
	})

	Describe("ConcurrentAccess", func() {
		BeforeEach(func() {
			for i := range 10 {
				registry.Register(&storage.CollectionMetadata{
					Name:       fmt.Sprintf("collection_%d", i),
					WorkerType: "test",
					Role:       storage.RoleIdentity,
				})
			}
		})

		It("should handle concurrent reads safely", func() {
			done := make(chan bool)
			for range 100 {
				go func() {
					collections := registry.List()
					Expect(collections).To(HaveLen(10))
					done <- true
				}()
			}

			for range 100 {
				<-done
			}
		})

		It("should handle concurrent RegisterVersion calls safely", func() {
			done := make(chan bool)
			for i := range 10 {
				go func(version int) {
					err := registry.RegisterVersion("test", storage.RoleIdentity, fmt.Sprintf("v%d", version))
					Expect(err).ToNot(HaveOccurred())
					done <- true
				}(i)
			}

			for range 10 {
				<-done
			}

			version := registry.GetVersion("collection_0")
			Expect(version).To(MatchRegexp(`^v\d+$`))
		})
	})
})

var _ = Describe("CollectionMetadata", func() {
	Describe("CSEFields", func() {
		It("should have all expected CSE field constants", func() {
			expectedFields := []string{
				storage.FieldSyncID,
				storage.FieldVersion,
				storage.FieldCreatedAt,
				storage.FieldUpdatedAt,
				storage.FieldDeletedAt,
				storage.FieldDeletedBy,
			}

			for _, field := range expectedFields {
				Expect(field).NotTo(BeEmpty())
			}
		})
	})

	Describe("Roles", func() {
		It("should have all expected role constants", func() {
			expectedRoles := []string{
				storage.RoleIdentity,
				storage.RoleDesired,
				storage.RoleObserved,
			}

			for _, role := range expectedRoles {
				Expect(role).NotTo(BeEmpty())
			}
		})
	})
})

var _ = Describe("Schema Versioning", func() {
	var registry *storage.Registry

	BeforeEach(func() {
		registry = storage.NewRegistry()
		registry.Register(&storage.CollectionMetadata{
			Name:       "workers",
			WorkerType: "container",
			Role:       storage.RoleIdentity,
		})
	})

	It("should register and retrieve schema version", func() {
		err := registry.RegisterVersion("container", storage.RoleIdentity, "v2")
		Expect(err).ToNot(HaveOccurred())
		version := registry.GetVersion("workers")
		Expect(version).To(Equal("v2"))
	})

	It("should return empty string for unversioned schema", func() {
		version := registry.GetVersion("workers")
		Expect(version).To(Equal(""))
	})

	It("should update schema version when registered multiple times", func() {
		err := registry.RegisterVersion("container", storage.RoleIdentity, "v1")
		Expect(err).ToNot(HaveOccurred())
		err = registry.RegisterVersion("container", storage.RoleIdentity, "v2")
		Expect(err).ToNot(HaveOccurred())
		version := registry.GetVersion("workers")
		Expect(version).To(Equal("v2"))
	})

	It("should track versions independently per collection", func() {
		registry.Register(&storage.CollectionMetadata{
			Name:       "datapoints",
			WorkerType: "sensor",
			Role:       storage.RoleIdentity,
		})
		err := registry.RegisterVersion("container", storage.RoleIdentity, "v2")
		Expect(err).ToNot(HaveOccurred())
		err = registry.RegisterVersion("sensor", storage.RoleIdentity, "v1")
		Expect(err).ToNot(HaveOccurred())

		Expect(registry.GetVersion("workers")).To(Equal("v2"))
		Expect(registry.GetVersion("datapoints")).To(Equal("v1"))
	})

	It("should return all versioned collections", func() {
		registry.Register(&storage.CollectionMetadata{
			Name:       "datapoints",
			WorkerType: "sensor",
			Role:       storage.RoleIdentity,
		})
		err := registry.RegisterVersion("container", storage.RoleIdentity, "v2")
		Expect(err).ToNot(HaveOccurred())
		err = registry.RegisterVersion("sensor", storage.RoleIdentity, "v1")
		Expect(err).ToNot(HaveOccurred())

		versions := registry.GetAllVersions()
		Expect(versions).To(HaveLen(2))
		Expect(versions["workers"]).To(Equal("v2"))
		Expect(versions["datapoints"]).To(Equal("v1"))
	})

	It("should reject empty version string", func() {
		err := registry.RegisterVersion("container", storage.RoleIdentity, "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("version cannot be empty"))
	})
})

var _ = Describe("Feature Registry", func() {
	var registry *storage.Registry

	BeforeEach(func() {
		registry = storage.NewRegistry()
	})

	It("should register and retrieve feature", func() {
		err := registry.RegisterFeature("delta_sync", true)
		Expect(err).ToNot(HaveOccurred())
		Expect(registry.HasFeature("delta_sync")).To(BeTrue())
	})

	It("should return false for non-existent feature", func() {
		Expect(registry.HasFeature("nonexistent_feature")).To(BeFalse())
	})

	It("should update feature when registered multiple times", func() {
		err := registry.RegisterFeature("delta_sync", true)
		Expect(err).ToNot(HaveOccurred())
		err = registry.RegisterFeature("delta_sync", false)
		Expect(err).ToNot(HaveOccurred())
		Expect(registry.HasFeature("delta_sync")).To(BeFalse())
	})

	It("should track features independently", func() {
		err := registry.RegisterFeature("delta_sync", true)
		Expect(err).ToNot(HaveOccurred())
		err = registry.RegisterFeature("e2e_encryption", false)
		Expect(err).ToNot(HaveOccurred())

		Expect(registry.HasFeature("delta_sync")).To(BeTrue())
		Expect(registry.HasFeature("e2e_encryption")).To(BeFalse())
	})

	It("should return all registered features", func() {
		err := registry.RegisterFeature("delta_sync", true)
		Expect(err).ToNot(HaveOccurred())
		err = registry.RegisterFeature("e2e_encryption", true)
		Expect(err).ToNot(HaveOccurred())
		err = registry.RegisterFeature("triangular_model", false)
		Expect(err).ToNot(HaveOccurred())

		features := registry.GetFeatures()
		Expect(features).To(HaveLen(3))
		Expect(features["delta_sync"]).To(BeTrue())
		Expect(features["e2e_encryption"]).To(BeTrue())
		Expect(features["triangular_model"]).To(BeFalse())
	})

	It("should reject empty feature name", func() {
		err := registry.RegisterFeature("", true)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("feature name cannot be empty"))
	})

	It("should handle concurrent RegisterFeature calls safely", func() {
		done := make(chan bool)
		for i := range 10 {
			go func(index int) {
				featureName := fmt.Sprintf("feature_%d", index)
				err := registry.RegisterFeature(featureName, true)
				Expect(err).ToNot(HaveOccurred())
				done <- true
			}(i)
		}

		for range 10 {
			<-done
		}

		features := registry.GetFeatures()
		Expect(features).To(HaveLen(10))
		for i := range 10 {
			featureName := fmt.Sprintf("feature_%d", i)
			Expect(registry.HasFeature(featureName)).To(BeTrue())
		}
	})
})

var _ = Describe("Registry with Type Metadata", func() {
	var registry *storage.Registry

	BeforeEach(func() {
		registry = storage.NewRegistry()
	})

	Describe("CollectionMetadata type fields", func() {
		It("stores type metadata in CollectionMetadata", func() {
			type TestObservedState struct {
				Name   string
				Status string
			}
			type TestDesiredState struct {
				Name    string
				Command string
			}

			observedType := reflect.TypeOf((*TestObservedState)(nil)).Elem()
			desiredType := reflect.TypeOf((*TestDesiredState)(nil)).Elem()

			metadata := &storage.CollectionMetadata{
				Name:         "test_observed",
				WorkerType:   "test",
				Role:         storage.RoleObserved,
				ObservedType: observedType,
				DesiredType:  desiredType,
			}

			err := registry.Register(metadata)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := registry.Get("test_observed")
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved.ObservedType).To(Equal(observedType))
			Expect(retrieved.DesiredType).To(Equal(desiredType))
		})
	})
})

var _ = Describe("GlobalRegistry", func() {
	Describe("Register", func() {
		It("should register collection in global registry", func() {
			metadata := &storage.CollectionMetadata{
				Name:       "global_test_identity",
				WorkerType: "global_test",
				Role:       storage.RoleIdentity,
			}

			err := storage.Register(metadata)
			Expect(err).NotTo(HaveOccurred())
			Expect(storage.IsRegistered("global_test_identity")).To(BeTrue())
		})
	})

	Describe("Get", func() {
		It("should retrieve from global registry", func() {
			metadata := &storage.CollectionMetadata{
				Name:       "global_get_test",
				WorkerType: "test",
				Role:       storage.RoleIdentity,
			}

			storage.Register(metadata)

			retrieved, err := storage.Get("global_get_test")
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved.Name).To(Equal("global_get_test"))
		})
	})

	Describe("List", func() {
		It("should list collections from global registry", func() {
			storage.Register(&storage.CollectionMetadata{
				Name:       "global_list_test",
				WorkerType: "test",
				Role:       storage.RoleIdentity,
			})

			collections := storage.List()
			Expect(collections).NotTo(BeEmpty())
		})
	})

	Describe("GetTriangularCollections", func() {
		It("should retrieve triangular collections from global registry", func() {
			storage.Register(&storage.CollectionMetadata{
				Name:       "global_triangular_identity",
				WorkerType: "global_triangular",
				Role:       storage.RoleIdentity,
			})
			storage.Register(&storage.CollectionMetadata{
				Name:       "global_triangular_desired",
				WorkerType: "global_triangular",
				Role:       storage.RoleDesired,
			})
			storage.Register(&storage.CollectionMetadata{
				Name:       "global_triangular_observed",
				WorkerType: "global_triangular",
				Role:       storage.RoleObserved,
			})

			identity, desired, observed, err := storage.GetTriangularCollections("global_triangular")
			Expect(err).NotTo(HaveOccurred())

			Expect(identity.Name).To(Equal("global_triangular_identity"))
			Expect(desired.Name).To(Equal("global_triangular_desired"))
			Expect(observed.Name).To(Equal("global_triangular_observed"))
		})
	})
})
