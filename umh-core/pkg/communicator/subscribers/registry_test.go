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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/subscribers"
)

var _ = Describe("Registry", func() {
	var (
		registry     *subscribers.Registry
		ttl          time.Duration
		cullInterval time.Duration
	)

	BeforeEach(func() {
		ttl = 100 * time.Millisecond
		cullInterval = 50 * time.Millisecond
		registry = subscribers.NewRegistry(cullInterval, ttl)
	})

	Describe("NewRegistry", func() {
		It("should create a new registry with the specified parameters", func() {
			Expect(registry).ToNot(BeNil())
			Expect(registry.Length()).To(Equal(0))
		})
	})

	Describe("Add", func() {
		Context("when adding a new subscriber", func() {
			It("should add the subscriber and initialize bootstrapped state to false", func() {
				email := "test@example.com"

				registry.Add(email)

				Expect(registry.Length()).To(Equal(1))

				// Check that bootstrapped state is initialized correctly
				bootstrapped := registry.IsBootstrapped(email)
				Expect(bootstrapped).To(BeFalse())
			})
		})

		Context("when adding an existing subscriber", func() {
			It("should reset the bootstrapped flag to false", func() {
				email := "test@example.com"

				// Add subscriber first time
				registry.Add(email)

				// Set subscriber as bootstrapped
				registry.SetBootstrapped(email, true)

				// Verify it was bootstrapped
				bootstrapped := registry.IsBootstrapped(email)
				Expect(bootstrapped).To(BeTrue())

				// Add the same subscriber again
				registry.Add(email)

				// Should still have only one subscriber
				Expect(registry.Length()).To(Equal(1))

				// Check that bootstrapped flag is reset
				bootstrapped = registry.IsBootstrapped(email)
				Expect(bootstrapped).To(BeFalse()) // Should be reset
			})
		})

		Context("when adding multiple subscribers", func() {
			It("should handle multiple unique subscribers correctly", func() {
				emails := []string{"user1@example.com", "user2@example.com", "user3@example.com"}

				for _, email := range emails {
					registry.Add(email)
				}

				Expect(registry.Length()).To(Equal(3))

				// Verify each subscriber has proper bootstrapped state
				for _, email := range emails {
					bootstrapped := registry.IsBootstrapped(email)
					Expect(bootstrapped).To(BeFalse())
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

	Describe("IsBootstrapped", func() {
		Context("when subscriber exists", func() {
			It("should return the correct bootstrapped state", func() {
				email := "test@example.com"
				registry.Add(email)

				// Initially should be false
				bootstrapped := registry.IsBootstrapped(email)
				Expect(bootstrapped).To(BeFalse())

				// Set to true
				registry.SetBootstrapped(email, true)

				bootstrapped = registry.IsBootstrapped(email)
				Expect(bootstrapped).To(BeTrue())
			})
		})

		Context("when subscriber does not exist", func() {
			It("should return false", func() {
				bootstrapped := registry.IsBootstrapped("nonexistent@example.com")
				Expect(bootstrapped).To(BeFalse())
			})
		})
	})

	Describe("ForEach", func() {
		Context("when registry is empty", func() {
			It("should not call the function", func() {
				callCount := 0
				registry.ForEach(func(email string, bootstrapped bool) {
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
				var receivedBootstrapped []bool

				registry.ForEach(func(email string, bootstrapped bool) {
					receivedEmails = append(receivedEmails, email)
					receivedBootstrapped = append(receivedBootstrapped, bootstrapped)
				})

				Expect(receivedEmails).To(HaveLen(3))
				Expect(receivedEmails).To(ConsistOf(emails))
				Expect(receivedBootstrapped).To(HaveLen(3))

				// Verify all are initially not bootstrapped
				for _, bootstrapped := range receivedBootstrapped {
					Expect(bootstrapped).To(BeFalse())
				}
			})
		})

		Context("when iterating over subscribers with different bootstrapped states", func() {
			It("should provide correct bootstrapped state for each subscriber", func() {
				email1 := "user1@example.com"
				email2 := "user2@example.com"

				registry.Add(email1)
				registry.Add(email2)

				// Set different bootstrapped states
				registry.SetBootstrapped(email1, true)
				registry.SetBootstrapped(email2, false)

				bootstrappedByEmail := make(map[string]bool)

				registry.ForEach(func(email string, bootstrapped bool) {
					bootstrappedByEmail[email] = bootstrapped
				})

				Expect(bootstrappedByEmail).To(HaveLen(2))
				Expect(bootstrappedByEmail[email1]).To(BeTrue())
				Expect(bootstrappedByEmail[email2]).To(BeFalse())
			})
		})
	})

	Describe("SetBootstrapped", func() {
		Context("when subscriber exists", func() {
			It("should update the bootstrapped state", func() {
				email := "test@example.com"
				registry.Add(email)

				// Initially should be false
				bootstrapped := registry.IsBootstrapped(email)
				Expect(bootstrapped).To(BeFalse())

				// Update to true
				registry.SetBootstrapped(email, true)

				bootstrapped = registry.IsBootstrapped(email)
				Expect(bootstrapped).To(BeTrue())

				// Update back to false
				registry.SetBootstrapped(email, false)

				bootstrapped = registry.IsBootstrapped(email)
				Expect(bootstrapped).To(BeFalse())
			})
		})

		Context("when subscriber does not exist", func() {
			It("should not panic or create the subscriber", func() {
				// This should not panic
				registry.SetBootstrapped("nonexistent@example.com", true)

				// Should still have no subscribers
				Expect(registry.Length()).To(Equal(0))

				// Should still return false for non-existent subscriber
				bootstrapped := registry.IsBootstrapped("nonexistent@example.com")
				Expect(bootstrapped).To(BeFalse())
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
			It("should return the correct count after expiration", func() {
				email := "test@example.com"
				registry.Add(email)

				Expect(registry.Length()).To(Equal(1))

				// Wait for expiration
				time.Sleep(ttl + cullInterval + 50*time.Millisecond)

				Expect(registry.Length()).To(Equal(0))
			})
		})
	})

	Describe("Expiration behavior", func() {
		Context("when bootstrapped state expires", func() {
			It("should also expire the bootstrapped state along with subscriber", func() {
				email := "test@example.com"
				registry.Add(email)
				registry.SetBootstrapped(email, true)

				// Verify both subscriber and bootstrapped state exist
				Expect(registry.Length()).To(Equal(1))
				Expect(registry.IsBootstrapped(email)).To(BeTrue())

				// Wait for expiration
				time.Sleep(ttl + cullInterval + 50*time.Millisecond)

				// Both should be gone
				Expect(registry.Length()).To(Equal(0))
				Expect(registry.IsBootstrapped(email)).To(BeFalse())
			})
		})

		Context("when re-adding expired subscriber", func() {
			It("should reset bootstrapped state to false", func() {
				email := "test@example.com"
				registry.Add(email)
				registry.SetBootstrapped(email, true)

				// Wait for expiration
				time.Sleep(ttl + cullInterval + 50*time.Millisecond)

				// Re-add the subscriber
				registry.Add(email)

				// Should be back but not bootstrapped
				Expect(registry.Length()).To(Equal(1))
				Expect(registry.IsBootstrapped(email)).To(BeFalse())
			})
		})
	})

	Describe("Concurrent access", func() {
		It("should handle concurrent operations safely", func() {
			email := "test@example.com"

			// Start multiple goroutines performing operations
			done := make(chan bool, 3)

			// Goroutine 1: Adding subscribers
			go func() {
				for i := 0; i < 100; i++ {
					registry.Add(email)
				}
				done <- true
			}()

			// Goroutine 2: Setting bootstrapped state
			go func() {
				for i := 0; i < 100; i++ {
					registry.SetBootstrapped(email, i%2 == 0)
				}
				done <- true
			}()

			// Goroutine 3: Reading state
			go func() {
				for i := 0; i < 100; i++ {
					registry.IsBootstrapped(email)
					registry.Length()
				}
				done <- true
			}()

			// Wait for all goroutines to complete
			for i := 0; i < 3; i++ {
				<-done
			}

			// Should not panic and should have consistent state
			Expect(registry.Length()).To(Equal(1))
		})
	})
})
