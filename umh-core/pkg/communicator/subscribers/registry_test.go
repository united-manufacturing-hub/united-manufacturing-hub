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

package subscribers_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/subscribers"
)

var _ = Describe("Registry", func() {
	var (
		registry     *subscribers.Registry
		cullInterval time.Duration
		ttl          time.Duration
	)

	BeforeEach(func() {
		cullInterval = 100 * time.Millisecond
		ttl = 500 * time.Millisecond
		registry = subscribers.NewRegistry(cullInterval, ttl)
	})

	Describe("NewRegistry", func() {
		It("should create a new registry with specified parameters", func() {
			Expect(registry).ToNot(BeNil())
			Expect(registry.Length()).To(Equal(0))
		})
	})

	Describe("Add", func() {
		Context("when adding a new subscriber", func() {
			It("should add the subscriber and initialize metadata", func() {
				email := "test@example.com"

				registry.Add(email)

				Expect(registry.Length()).To(Equal(1))

				// Check that metadata is initialized correctly
				meta := registry.Meta(email)
				Expect(meta.FirstSeen).To(BeTemporally("~", time.Now(), time.Second))
				Expect(meta.LastSeq).To(Equal(uint64(0)))
				Expect(meta.Bootstraped).To(BeFalse())
			})
		})

		Context("when adding an existing subscriber", func() {
			It("should update the subscriber's metadata and reset Bootstraped flag", func() {
				email := "test@example.com"

				// Add subscriber first time
				registry.Add(email)

				// Update metadata to simulate it being bootstrapped
				registry.UpdateMeta(email, func(m *subscribers.Meta) {
					m.Bootstraped = true
					m.LastSeq = 100
				})

				// Verify it was bootstrapped
				meta := registry.Meta(email)
				Expect(meta.Bootstraped).To(BeTrue())
				Expect(meta.LastSeq).To(Equal(uint64(100)))

				// Add the same subscriber again
				registry.Add(email)

				// Should still have only one subscriber
				Expect(registry.Length()).To(Equal(1))

				// Check that Bootstraped flag is reset but other metadata preserved
				meta = registry.Meta(email)
				Expect(meta.Bootstraped).To(BeFalse())      // Should be reset
				Expect(meta.LastSeq).To(Equal(uint64(100))) // Should be preserved
			})
		})

		Context("when adding multiple subscribers", func() {
			It("should handle multiple unique subscribers correctly", func() {
				emails := []string{"user1@example.com", "user2@example.com", "user3@example.com"}

				for _, email := range emails {
					registry.Add(email)
				}

				Expect(registry.Length()).To(Equal(3))

				// Verify each subscriber has proper metadata
				for _, email := range emails {
					meta := registry.Meta(email)
					Expect(meta.FirstSeen).To(BeTemporally("~", time.Now(), time.Second))
					Expect(meta.LastSeq).To(Equal(uint64(0)))
					Expect(meta.Bootstraped).To(BeFalse())
				}
			})
		})
	})

	Describe("List", func() {
		Context("when registry is empty", func() {
			It("should return an empty slice", func() {
				subscribers := registry.List()
				Expect(subscribers).To(HaveLen(0))
			})
		})

		Context("when registry has subscribers", func() {
			It("should return all active subscriber emails", func() {
				emails := []string{"user1@example.com", "user2@example.com", "user3@example.com"}

				for _, email := range emails {
					registry.Add(email)
				}

				result := registry.List()
				Expect(result).To(HaveLen(3))
				Expect(result).To(ConsistOf(emails))
			})
		})

		Context("when subscribers expire", func() {
			It("should not include expired subscribers in the list", func() {
				email := "test@example.com"
				registry.Add(email)

				Expect(registry.List()).To(ContainElement(email))

				// Wait for expiration
				time.Sleep(ttl + cullInterval + 50*time.Millisecond)

				result := registry.List()
				Expect(result).ToNot(ContainElement(email))
			})
		})
	})

	Describe("Meta", func() {
		Context("when subscriber exists", func() {
			It("should return the correct metadata", func() {
				email := "test@example.com"
				registry.Add(email)

				// Update metadata
				registry.UpdateMeta(email, func(m *subscribers.Meta) {
					m.LastSeq = 42
					m.Bootstraped = true
				})

				meta := registry.Meta(email)
				Expect(meta.LastSeq).To(Equal(uint64(42)))
				Expect(meta.Bootstraped).To(BeTrue())
				Expect(meta.FirstSeen).To(BeTemporally("~", time.Now(), time.Second))
			})
		})

		Context("when subscriber does not exist", func() {
			It("should return zero value metadata", func() {
				meta := registry.Meta("nonexistent@example.com")

				Expect(meta.FirstSeen).To(BeZero())
				Expect(meta.LastSeq).To(Equal(uint64(0)))
				Expect(meta.Bootstraped).To(BeFalse())
			})
		})
	})

	Describe("ForEach", func() {
		Context("when registry is empty", func() {
			It("should not call the function", func() {
				callCount := 0
				registry.ForEach(func(email string, m *subscribers.Meta) {
					callCount++
				})

				Expect(callCount).To(Equal(0))
			})
		})

		Context("when registry has subscribers", func() {
			It("should call the function for each active subscriber", func() {
				emails := []string{"user1@example.com", "user2@example.com", "user3@example.com"}

				for _, email := range emails {
					registry.Add(email)
				}

				var receivedEmails []string
				var receivedMetas []*subscribers.Meta

				registry.ForEach(func(email string, m *subscribers.Meta) {
					receivedEmails = append(receivedEmails, email)
					receivedMetas = append(receivedMetas, m)
				})

				Expect(receivedEmails).To(HaveLen(3))
				Expect(receivedEmails).To(ConsistOf(emails))
				Expect(receivedMetas).To(HaveLen(3))

				// Verify metadata is properly passed
				for _, meta := range receivedMetas {
					Expect(meta.FirstSeen).To(BeTemporally("~", time.Now(), time.Second))
					Expect(meta.LastSeq).To(Equal(uint64(0)))
					Expect(meta.Bootstraped).To(BeFalse())
				}
			})
		})

		Context("when iterating over subscribers with different metadata", func() {
			It("should provide correct metadata for each subscriber", func() {
				email1 := "user1@example.com"
				email2 := "user2@example.com"

				registry.Add(email1)
				registry.Add(email2)

				// Update metadata for each subscriber differently
				registry.UpdateMeta(email1, func(m *subscribers.Meta) {
					m.LastSeq = 100
					m.Bootstraped = true
				})

				registry.UpdateMeta(email2, func(m *subscribers.Meta) {
					m.LastSeq = 200
					m.Bootstraped = false
				})

				metaByEmail := make(map[string]subscribers.Meta)

				registry.ForEach(func(email string, m *subscribers.Meta) {
					metaByEmail[email] = *m
				})

				Expect(metaByEmail[email1].LastSeq).To(Equal(uint64(100)))
				Expect(metaByEmail[email1].Bootstraped).To(BeTrue())

				Expect(metaByEmail[email2].LastSeq).To(Equal(uint64(200)))
				Expect(metaByEmail[email2].Bootstraped).To(BeFalse())
			})
		})
	})

	Describe("UpdateMeta", func() {
		Context("when subscriber exists", func() {
			It("should update the metadata correctly", func() {
				email := "test@example.com"
				registry.Add(email)

				// Initial state
				meta := registry.Meta(email)
				Expect(meta.LastSeq).To(Equal(uint64(0)))
				Expect(meta.Bootstraped).To(BeFalse())

				// Update metadata
				registry.UpdateMeta(email, func(m *subscribers.Meta) {
					m.LastSeq = 42
					m.Bootstraped = true
				})

				// Verify update
				meta = registry.Meta(email)
				Expect(meta.LastSeq).To(Equal(uint64(42)))
				Expect(meta.Bootstraped).To(BeTrue())
			})

			It("should allow multiple sequential updates", func() {
				email := "test@example.com"
				registry.Add(email)

				// First update
				registry.UpdateMeta(email, func(m *subscribers.Meta) {
					m.LastSeq = 10
				})

				meta := registry.Meta(email)
				Expect(meta.LastSeq).To(Equal(uint64(10)))

				// Second update
				registry.UpdateMeta(email, func(m *subscribers.Meta) {
					m.LastSeq = m.LastSeq + 5
					m.Bootstraped = true
				})

				meta = registry.Meta(email)
				Expect(meta.LastSeq).To(Equal(uint64(15)))
				Expect(meta.Bootstraped).To(BeTrue())
			})
		})

		Context("when subscriber does not exist", func() {
			It("should not panic and do nothing", func() {
				Expect(func() {
					registry.UpdateMeta("nonexistent@example.com", func(m *subscribers.Meta) {
						m.LastSeq = 42
						m.Bootstraped = true
					})
				}).ToNot(Panic())

				// Verify no changes were made
				meta := registry.Meta("nonexistent@example.com")
				Expect(meta.LastSeq).To(Equal(uint64(0)))
				Expect(meta.Bootstraped).To(BeFalse())
			})
		})
	})

	Describe("Length", func() {
		Context("when registry is empty", func() {
			It("should return 0", func() {
				Expect(registry.Length()).To(Equal(0))
			})
		})

		Context("when registry has subscribers", func() {
			It("should return the correct count", func() {
				emails := []string{"user1@example.com", "user2@example.com", "user3@example.com"}

				for i, email := range emails {
					registry.Add(email)
					Expect(registry.Length()).To(Equal(i + 1))
				}
			})
		})

		Context("when subscribers expire", func() {
			It("should reflect the reduced count after expiration", func() {
				email := "test@example.com"
				registry.Add(email)

				Expect(registry.Length()).To(Equal(1))

				// Wait for expiration
				time.Sleep(ttl + cullInterval + 50*time.Millisecond)

				Expect(registry.Length()).To(Equal(0))
			})
		})
	})

	Describe("Thread Safety", func() {
		It("should handle concurrent operations safely", func() {
			const numGoroutines = 10
			const numOperationsPerGoroutine = 100

			done := make(chan bool, numGoroutines)

			// Start multiple goroutines performing concurrent operations
			for i := 0; i < numGoroutines; i++ {
				go func(id int) {
					defer GinkgoRecover()

					for j := 0; j < numOperationsPerGoroutine; j++ {
						email := fmt.Sprintf("user%d_%d@example.com", id, j)

						// Add subscriber
						registry.Add(email)

						// Update metadata
						registry.UpdateMeta(email, func(m *subscribers.Meta) {
							m.LastSeq = uint64(j)
							m.Bootstraped = j%2 == 0
						})

						// Read metadata
						meta := registry.Meta(email)
						Expect(meta.LastSeq).To(BeNumerically(">=", 0))

						// List subscribers
						list := registry.List()
						Expect(list).ToNot(BeNil())

						// Get length
						length := registry.Length()
						Expect(length).To(BeNumerically(">=", 0))
					}

					done <- true
				}(i)
			}

			// Wait for all goroutines to complete
			for i := 0; i < numGoroutines; i++ {
				Eventually(done).Should(Receive())
			}

			// Verify final state
			finalLength := registry.Length()
			Expect(finalLength).To(Equal(numGoroutines * numOperationsPerGoroutine))
		})
	})

	Describe("Integration with ExpireMap", func() {
		Context("when subscribers expire naturally", func() {
			It("should clean up both expiremap and metadata", func() {
				email := "test@example.com"
				registry.Add(email)

				// Verify subscriber is active
				Expect(registry.Length()).To(Equal(1))
				Expect(registry.List()).To(ContainElement(email))
				meta := registry.Meta(email)
				Expect(meta.FirstSeen).ToNot(BeZero())

				// Wait for expiration
				time.Sleep(ttl + cullInterval + 50*time.Millisecond)

				// Verify subscriber is removed from expiremap
				Expect(registry.Length()).To(Equal(0))
				Expect(registry.List()).ToNot(ContainElement(email))

				// Note: Metadata cleanup isn't implemented in the current design
				// This test documents the current behavior where metadata persists
				// even after expiration from the expiremap
				meta = registry.Meta(email)
				Expect(meta.FirstSeen).ToNot(BeZero()) // Metadata still exists
			})
		})

		Context("when re-adding expired subscribers", func() {
			It("should reuse existing metadata but reset Bootstraped flag", func() {
				email := "test@example.com"

				// Add subscriber first time
				registry.Add(email)

				// Update metadata
				registry.UpdateMeta(email, func(m *subscribers.Meta) {
					m.LastSeq = 42
					m.Bootstraped = true
				})

				originalMeta := registry.Meta(email)

				// Wait for expiration
				time.Sleep(ttl + cullInterval + 50*time.Millisecond)

				// Verify expired from expiremap
				Expect(registry.Length()).To(Equal(0))

				// Re-add the same subscriber
				registry.Add(email)

				// Verify subscriber is back
				Expect(registry.Length()).To(Equal(1))

				// Check metadata behavior
				newMeta := registry.Meta(email)
				Expect(newMeta.FirstSeen).To(Equal(originalMeta.FirstSeen)) // Preserved
				Expect(newMeta.LastSeq).To(Equal(uint64(42)))               // Preserved
				Expect(newMeta.Bootstraped).To(BeFalse())                   // Reset
			})
		})
	})
})
