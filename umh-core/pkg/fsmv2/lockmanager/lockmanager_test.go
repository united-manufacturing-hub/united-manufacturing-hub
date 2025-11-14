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

package lockmanager_test

import (
	"os"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/lockmanager"
)

func TestLockManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LockManager Suite")
}

var _ = Describe("LockManager", func() {
	var (
		manager *lockmanager.LockManager
		lock1   *lockmanager.Lock
		lock2   *lockmanager.Lock
	)

	BeforeEach(func() {
		os.Setenv("ENABLE_LOCK_ORDER_CHECKS", "1")
		manager = lockmanager.NewLockManager()
		lock1 = manager.NewLock("Lock1", 1)
		lock2 = manager.NewLock("Lock2", 2)
	})

	AfterEach(func() {
		os.Unsetenv("ENABLE_LOCK_ORDER_CHECKS")
	})

	Describe("Lock acquisition tracking", func() {
		It("should track lock acquisition", func() {
			lock1.Lock()
			defer lock1.Unlock()

			Expect(manager.IsHeld("Lock1")).To(BeTrue())
		})

		It("should track lock release", func() {
			lock1.Lock()
			lock1.Unlock()

			Expect(manager.IsHeld("Lock1")).To(BeFalse())
		})

		It("should track RLock acquisition", func() {
			lock1.RLock()
			defer lock1.RUnlock()

			Expect(manager.IsHeld("Lock1")).To(BeTrue())
		})
	})

	Describe("Lock order violation detection", func() {
		It("should panic when acquiring lower-level lock while holding higher-level lock", func() {
			lock2.Lock()
			defer lock2.Unlock()

			Expect(func() {
				lock1.Lock()
			}).To(Panic())
		})

		It("should allow acquiring locks in correct order", func() {
			lock1.Lock()
			defer lock1.Unlock()

			lock2.Lock()
			defer lock2.Unlock()

			Expect(manager.IsHeld("Lock1")).To(BeTrue())
			Expect(manager.IsHeld("Lock2")).To(BeTrue())
		})

		It("should allow acquiring same-level lock multiple times", func() {
			lock1Duplicate := manager.NewLock("Lock1Duplicate", 1)

			lock1.Lock()
			defer lock1.Unlock()

			lock1Duplicate.Lock()
			defer lock1Duplicate.Unlock()

			Expect(manager.IsHeld("Lock1")).To(BeTrue())
			Expect(manager.IsHeld("Lock1Duplicate")).To(BeTrue())
		})
	})

	Describe("Goroutine isolation", func() {
		It("should track locks independently per goroutine", func() {
			done := make(chan bool)

			go func() {
				defer GinkgoRecover()
				lock1.Lock()
				time.Sleep(50 * time.Millisecond)
				Expect(manager.IsHeld("Lock1")).To(BeTrue())
				lock1.Unlock()
				done <- true
			}()

			Expect(manager.IsHeld("Lock1")).To(BeFalse())
			<-done
		})

		It("should allow different goroutines to acquire locks in different orders", func() {
			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				lock1.Lock()
				time.Sleep(10 * time.Millisecond)
				lock2.Lock()
				lock2.Unlock()
				lock1.Unlock()
			}()

			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				time.Sleep(5 * time.Millisecond)
				lock2.Lock()
				lock2.Unlock()
			}()

			wg.Wait()
		})
	})

	Describe("Environment variable gating", func() {
		It("should disable tracking when env var is not set", func() {
			os.Unsetenv("ENABLE_LOCK_ORDER_CHECKS")
			freshManager := lockmanager.NewLockManager()
			freshLock1 := freshManager.NewLock("FreshLock1", 1)
			freshLock2 := freshManager.NewLock("FreshLock2", 2)

			freshLock2.Lock()
			defer freshLock2.Unlock()

			Expect(func() {
				freshLock1.Lock()
				freshLock1.Unlock()
			}).NotTo(Panic())
		})

		It("should have zero overhead when disabled", func() {
			os.Unsetenv("ENABLE_LOCK_ORDER_CHECKS")
			freshManager := lockmanager.NewLockManager()
			freshLock := freshManager.NewLock("PerfLock", 1)

			start := time.Now()
			for i := 0; i < 10000; i++ {
				freshLock.Lock()
				freshLock.Unlock()
			}
			duration := time.Since(start)

			Expect(duration).To(BeNumerically("<", 100*time.Millisecond))
		})
	})

	Describe("Lock wrapper API", func() {
		It("should provide Mutex-compatible Lock/Unlock", func() {
			lock1.Lock()
			Expect(manager.IsHeld("Lock1")).To(BeTrue())
			lock1.Unlock()
			Expect(manager.IsHeld("Lock1")).To(BeFalse())
		})

		It("should provide RWMutex-compatible RLock/RUnlock", func() {
			lock1.RLock()
			Expect(manager.IsHeld("Lock1")).To(BeTrue())
			lock1.RUnlock()
			Expect(manager.IsHeld("Lock1")).To(BeFalse())
		})
	})

	Describe("Multiple lock releases", func() {
		It("should handle multiple acquisitions of the same lock correctly", func() {
			lock1.RLock()
			lock1.RLock()

			Expect(manager.IsHeld("Lock1")).To(BeTrue())

			lock1.RUnlock()
			Expect(manager.IsHeld("Lock1")).To(BeTrue())

			lock1.RUnlock()
			Expect(manager.IsHeld("Lock1")).To(BeFalse())
		})
	})

	Describe("Panic message details", func() {
		It("should provide detailed error information on violation", func() {
			var panicMsg string
			lock2.Lock()
			defer lock2.Unlock()

			func() {
				defer func() {
					if r := recover(); r != nil {
						panicMsg = r.(string)
					}
				}()
				lock1.Lock()
			}()

			Expect(panicMsg).To(ContainSubstring("lock order violation"))
			Expect(panicMsg).To(ContainSubstring("Lock1"))
			Expect(panicMsg).To(ContainSubstring("Lock2"))
			Expect(panicMsg).To(ContainSubstring("level 1"))
			Expect(panicMsg).To(ContainSubstring("level 2"))
		})
	})
})
