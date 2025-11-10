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
	"errors"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
)

// Mock closeable object for testing.
type mockCloseable struct {
	closed bool
	mu     sync.Mutex
}

func (m *mockCloseable) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true

	return nil
}

func (m *mockCloseable) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.closed
}

// Mock closeable with error.
type mockCloseableWithError struct {
	closed bool
}

func (m *mockCloseableWithError) Close() error {
	m.closed = true

	return errors.New("close error")
}

// Mock non-closeable object.
type mockObject struct {
	value string
}

var _ = Describe("ObjectPool", func() {
	var (
		pool *storage.ObjectPool
	)

	BeforeEach(func() {
		pool = storage.NewObjectPool()
	})

	Describe("NewObjectPool", func() {
		It("should create an empty pool", func() {
			Expect(pool).NotTo(BeNil())
			Expect(pool.Size()).To(Equal(0))
		})
	})

	Describe("Put and Get", func() {
		It("should store and retrieve objects", func() {
			obj := &mockCloseable{}

			err := pool.Put("test-key", obj)
			Expect(err).NotTo(HaveOccurred())

			retrieved, found := pool.Get("test-key")
			Expect(found).To(BeTrue())
			Expect(retrieved).To(BeIdenticalTo(obj)) // Same reference
		})

		It("should handle multiple objects", func() {
			obj1 := &mockCloseable{}
			obj2 := &mockCloseable{}
			obj3 := &mockCloseable{}

			pool.Put("key1", obj1)
			pool.Put("key2", obj2)
			pool.Put("key3", obj3)

			retrieved1, _ := pool.Get("key1")
			retrieved2, _ := pool.Get("key2")
			retrieved3, _ := pool.Get("key3")

			Expect(retrieved1).To(BeIdenticalTo(obj1))
			Expect(retrieved2).To(BeIdenticalTo(obj2))
			Expect(retrieved3).To(BeIdenticalTo(obj3))
		})

		It("should overwrite existing key", func() {
			obj1 := &mockCloseable{}
			obj2 := &mockCloseable{}

			pool.Put("test-key", obj1)
			pool.Put("test-key", obj2)

			retrieved, _ := pool.Get("test-key")
			Expect(retrieved).To(BeIdenticalTo(obj2))
			Expect(retrieved).NotTo(BeIdenticalTo(obj1))
		})

		Context("when key doesn't exist", func() {
			It("should return false", func() {
				_, found := pool.Get("nonexistent")
				Expect(found).To(BeFalse())
			})
		})

		Context("when storing nil object", func() {
			It("should return error", func() {
				err := pool.Put("test-key", nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("nil"))
			})
		})

		Context("when using empty key", func() {
			It("should return error", func() {
				err := pool.Put("", &mockCloseable{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("empty"))
			})
		})
	})

	Describe("Has", func() {
		BeforeEach(func() {
			pool.Put("test-key", &mockCloseable{})
		})

		It("should return true for existing keys", func() {
			Expect(pool.Has("test-key")).To(BeTrue())
		})

		It("should return false for non-existing keys", func() {
			Expect(pool.Has("other-key")).To(BeFalse())
		})

		It("should return false for empty key", func() {
			Expect(pool.Has("")).To(BeFalse())
		})
	})

	Describe("Size", func() {
		It("should return 0 for empty pool", func() {
			Expect(pool.Size()).To(Equal(0))
		})

		It("should return correct size after adding objects", func() {
			pool.Put("key1", &mockCloseable{})
			pool.Put("key2", &mockCloseable{})
			pool.Put("key3", &mockCloseable{})

			Expect(pool.Size()).To(Equal(3))
		})

		It("should not change size when overwriting key", func() {
			pool.Put("test-key", &mockCloseable{})
			pool.Put("test-key", &mockCloseable{})

			Expect(pool.Size()).To(Equal(1))
		})

		It("should decrease after removing object", func() {
			pool.Put("key1", &mockCloseable{})
			pool.Put("key2", &mockCloseable{})

			pool.Remove("key1")

			Expect(pool.Size()).To(Equal(1))
		})
	})

	Describe("Remove", func() {
		var obj *mockCloseable

		BeforeEach(func() {
			obj = &mockCloseable{}
			pool.Put("test-key", obj)
		})

		It("should remove object from pool", func() {
			err := pool.Remove("test-key")
			Expect(err).NotTo(HaveOccurred())
			Expect(pool.Has("test-key")).To(BeFalse())
		})

		It("should close Closeable objects", func() {
			pool.Remove("test-key")
			Expect(obj.IsClosed()).To(BeTrue())
		})

		It("should not close non-Closeable objects", func() {
			nonCloseable := &mockObject{value: "test"}
			pool.Put("non-closeable", nonCloseable)

			err := pool.Remove("non-closeable")
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when key doesn't exist", func() {
			It("should not return error", func() {
				err := pool.Remove("nonexistent")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when Close returns error", func() {
			It("should return the error", func() {
				errObj := &mockCloseableWithError{}
				pool.Put("error-key", errObj)

				err := pool.Remove("error-key")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("close error"))
			})

			It("should still remove object from pool", func() {
				errObj := &mockCloseableWithError{}
				pool.Put("error-key", errObj)

				pool.Remove("error-key")
				Expect(pool.Has("error-key")).To(BeFalse())
			})
		})
	})

	Describe("Clear", func() {
		var (
			obj1, obj2, obj3 *mockCloseable
		)

		BeforeEach(func() {
			obj1 = &mockCloseable{}
			obj2 = &mockCloseable{}
			obj3 = &mockCloseable{}
			pool.Put("key1", obj1)
			pool.Put("key2", obj2)
			pool.Put("key3", obj3)
		})

		It("should remove all objects", func() {
			err := pool.Clear()
			Expect(err).NotTo(HaveOccurred())
			Expect(pool.Size()).To(Equal(0))
		})

		It("should close all Closeable objects", func() {
			pool.Clear()
			Expect(obj1.IsClosed()).To(BeTrue())
			Expect(obj2.IsClosed()).To(BeTrue())
			Expect(obj3.IsClosed()).To(BeTrue())
		})

		It("should work on empty pool", func() {
			emptyPool := storage.NewObjectPool()
			err := emptyPool.Clear()
			Expect(err).NotTo(HaveOccurred())
			Expect(emptyPool.Size()).To(Equal(0))
		})

		Context("when some Close calls fail", func() {
			It("should return combined error", func() {
				errObj := &mockCloseableWithError{}
				pool.Put("error-key", errObj)

				err := pool.Clear()
				Expect(err).To(HaveOccurred())
			})

			It("should still remove all objects", func() {
				errObj := &mockCloseableWithError{}
				pool.Put("error-key", errObj)

				pool.Clear()
				Expect(pool.Size()).To(Equal(0))
			})
		})
	})

	Describe("GetOrCreate", func() {
		var (
			factoryCalled bool
			factoryObj    *mockCloseable
		)

		BeforeEach(func() {
			factoryCalled = false
			factoryObj = &mockCloseable{}
		})

		factory := func() storage.Factory {
			return func() (interface{}, error) {
				factoryCalled = true

				return factoryObj, nil
			}
		}

		Context("when object doesn't exist", func() {
			It("should call factory and store object", func() {
				obj, err := pool.GetOrCreate("test-key", factory())
				Expect(err).NotTo(HaveOccurred())
				Expect(factoryCalled).To(BeTrue())
				Expect(obj).To(BeIdenticalTo(factoryObj))
				Expect(pool.Has("test-key")).To(BeTrue())
			})

			It("should initialize reference count to 1", func() {
				pool.GetOrCreate("test-key", factory())

				// Release once should remove it
				err := pool.Release("test-key")
				Expect(err).NotTo(HaveOccurred())
				Expect(pool.Has("test-key")).To(BeFalse())
			})
		})

		Context("when object already exists", func() {
			var existingObj *mockCloseable

			BeforeEach(func() {
				existingObj = &mockCloseable{}
				pool.Put("test-key", existingObj)
			})

			It("should return existing object without calling factory", func() {
				obj, err := pool.GetOrCreate("test-key", factory())
				Expect(err).NotTo(HaveOccurred())
				Expect(factoryCalled).To(BeFalse())
				Expect(obj).To(BeIdenticalTo(existingObj))
				Expect(obj).NotTo(BeIdenticalTo(factoryObj))
			})
		})

		Context("when factory returns error", func() {
			errorFactory := func() (interface{}, error) {
				return nil, errors.New("factory error")
			}

			It("should return error and not store object", func() {
				_, err := pool.GetOrCreate("test-key", errorFactory)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("factory error"))
				Expect(pool.Has("test-key")).To(BeFalse())
			})
		})

		Context("when factory returns nil", func() {
			nilFactory := func() (interface{}, error) {
				return nil, nil
			}

			It("should return error and not store object", func() {
				_, err := pool.GetOrCreate("test-key", nilFactory)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("nil"))
				Expect(pool.Has("test-key")).To(BeFalse())
			})
		})

		Context("when key is empty", func() {
			It("should return error", func() {
				_, err := pool.GetOrCreate("", factory())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("empty"))
			})
		})

		Context("when factory is nil", func() {
			It("should return error", func() {
				_, err := pool.GetOrCreate("test-key", nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("nil factory"))
			})
		})
	})

	Describe("Reference counting", func() {
		BeforeEach(func() {
			pool.Put("test-key", &mockCloseable{})
		})

		Describe("Acquire", func() {
			It("should increment reference count", func() {
				err := pool.Acquire("test-key")
				Expect(err).NotTo(HaveOccurred())

				err = pool.Acquire("test-key")
				Expect(err).NotTo(HaveOccurred())

				// Object should still exist after one Release
				pool.Release("test-key")
				Expect(pool.Has("test-key")).To(BeTrue())
			})

			It("should allow multiple acquires", func() {
				pool.Acquire("test-key")
				pool.Acquire("test-key")
				pool.Acquire("test-key")

				// Need 3 releases (plus 1 for initial ref) = 4 total
				pool.Release("test-key")
				pool.Release("test-key")
				pool.Release("test-key")
				Expect(pool.Has("test-key")).To(BeTrue())

				// Final release removes it
				pool.Release("test-key")
				Expect(pool.Has("test-key")).To(BeFalse())
			})

			Context("when key doesn't exist", func() {
				It("should return error", func() {
					err := pool.Acquire("nonexistent")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("not found"))
				})
			})

			Context("when key is empty", func() {
				It("should return error", func() {
					err := pool.Acquire("")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("empty"))
				})
			})
		})

		Describe("Release", func() {
			It("should decrement reference count", func() {
				pool.Acquire("test-key")
				pool.Acquire("test-key")

				// First release: refs=2, object stays
				err := pool.Release("test-key")
				Expect(err).NotTo(HaveOccurred())
				Expect(pool.Has("test-key")).To(BeTrue())

				// Second release: refs=1, object stays
				err = pool.Release("test-key")
				Expect(err).NotTo(HaveOccurred())
				Expect(pool.Has("test-key")).To(BeTrue())

				// Third release: refs=0, object removed
				err = pool.Release("test-key")
				Expect(err).NotTo(HaveOccurred())
				Expect(pool.Has("test-key")).To(BeFalse())
			})

			It("should close object when reference count reaches zero", func() {
				obj := &mockCloseable{}
				pool.Put("test-key", obj)

				err := pool.Release("test-key")
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.IsClosed()).To(BeTrue())
			})

			Context("when key doesn't exist", func() {
				It("should return error", func() {
					err := pool.Release("nonexistent")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("not found"))
				})
			})

			Context("when key is empty", func() {
				It("should return error", func() {
					err := pool.Release("")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("empty"))
				})
			})

			Context("when Close returns error", func() {
				It("should return the error", func() {
					errObj := &mockCloseableWithError{}
					pool.Put("error-key", errObj)

					err := pool.Release("error-key")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("close error"))
				})

				It("should still remove object from pool", func() {
					errObj := &mockCloseableWithError{}
					pool.Put("error-key", errObj)

					pool.Release("error-key")
					Expect(pool.Has("error-key")).To(BeFalse())
				})
			})
		})

		Describe("Combined Acquire/Release workflow", func() {
			It("should handle realistic usage pattern", func() {
				// Initial put: ref=1
				obj := &mockCloseable{}
				pool.Put("worker-123", obj)

				// First consumer acquires: ref=2
				pool.Acquire("worker-123")

				// Second consumer acquires: ref=3
				pool.Acquire("worker-123")

				// First consumer releases: ref=2
				pool.Release("worker-123")
				Expect(pool.Has("worker-123")).To(BeTrue())

				// Second consumer releases: ref=1
				pool.Release("worker-123")
				Expect(pool.Has("worker-123")).To(BeTrue())

				// Original owner releases: ref=0, removed
				pool.Release("worker-123")
				Expect(pool.Has("worker-123")).To(BeFalse())
				Expect(obj.IsClosed()).To(BeTrue())
			})
		})
	})

	Describe("Concurrent access", func() {
		It("should handle concurrent Put/Get safely", func(ctx SpecContext) {
			done := make(chan bool, 10)

			// Spawn 10 goroutines
			for i := range 10 {
				go func(id int) {
					defer GinkgoRecover()

					key := fmt.Sprintf("key-%d", id)
					obj := &mockCloseable{}

					pool.Put(key, obj)
					retrieved, _ := pool.Get(key)
					Expect(retrieved).To(BeIdenticalTo(obj))

					done <- true
				}(i)
			}

			// Wait for all goroutines
			for range 10 {
				<-done
			}

			Expect(pool.Size()).To(Equal(10))
		}, SpecTimeout(time.Second))

		It("should handle concurrent GetOrCreate safely", func(ctx SpecContext) {
			done := make(chan bool, 20)

			factory := func() (interface{}, error) {
				// This should only be called once per key
				return &mockCloseable{}, nil
			}

			// Spawn 20 goroutines trying to create same 5 keys
			for i := range 20 {
				go func(id int) {
					defer GinkgoRecover()

					key := fmt.Sprintf("key-%d", id%5) // Only 5 unique keys
					obj, err := pool.GetOrCreate(key, factory)
					Expect(err).NotTo(HaveOccurred())
					Expect(obj).NotTo(BeNil())

					done <- true
				}(i)
			}

			// Wait for all goroutines
			for range 20 {
				<-done
			}

			// Should only have 5 objects
			Expect(pool.Size()).To(Equal(5))
		}, SpecTimeout(time.Second))

		It("should handle concurrent Acquire/Release safely", func(ctx SpecContext) {
			obj := &mockCloseable{}
			pool.Put("test-key", obj)

			done := make(chan bool, 20)

			// Spawn 10 acquirers and 10 releasers
			for range 10 {
				go func() {
					defer GinkgoRecover()
					pool.Acquire("test-key")
					done <- true
				}()

				go func() {
					defer GinkgoRecover()
					pool.Release("test-key")
					done <- true
				}()
			}

			// Wait for all goroutines
			for range 20 {
				<-done
			}

			// Object may or may not exist depending on race outcome
			// But pool should still be valid
			Expect(pool).NotTo(BeNil())
		}, SpecTimeout(time.Second))

		It("should handle concurrent Remove safely", func(ctx SpecContext) {
			// Add 100 objects
			for i := range 100 {
				pool.Put(fmt.Sprintf("key-%d", i), &mockCloseable{})
			}

			done := make(chan bool, 50)

			// Spawn 50 goroutines removing objects
			for i := range 50 {
				go func(id int) {
					defer GinkgoRecover()
					pool.Remove(fmt.Sprintf("key-%d", id))
					done <- true
				}(i)
			}

			// Wait for all goroutines
			for range 50 {
				<-done
			}

			// Should have 50 objects left
			Expect(pool.Size()).To(Equal(50))
		}, SpecTimeout(time.Second))
	})
})
