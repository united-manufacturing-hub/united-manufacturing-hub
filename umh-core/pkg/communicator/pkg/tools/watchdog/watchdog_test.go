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

package watchdog

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

var _ = Describe("Watchdog", func() {
	// Normally Watch is triggered by main, but in tests we need to call it manually

	var panickingUUIDs map[uuid.UUID]bool
	var panickingUUIDsLock sync.Mutex
	var otherpanic atomic.Value
	var dog atomic.Pointer[Watchdog]
	var dogCnclAtomic atomic.Value

	BeforeEach(func() {
		panickingUUIDs = make(map[uuid.UUID]bool)
		panickingUUIDsLock = sync.Mutex{}
		otherpanic.Store(false)
		ctx, cncl := context.WithCancel(context.Background())
		dogCnclAtomic.Store(cncl)
		// Start Watch in a goroutine
		go func() {
			defer func() {
				if r := recover(); r != nil {
					// Extract witch test caused the panic
					// Heartbeat too old test-2 (cd41ec9f-b168-4b58-a41c-4e582b6a2122)
					// We want to get the uuid
					uuidRegex := regexp.MustCompile(`\[.+?\].+((\w{8})-(\w{4})-(\w{4})-(\w{4})-(\w{12}))`)
					matches := uuidRegex.FindStringSubmatch(r.(string))
					if len(matches) > 1 {
						// zap.S().Debuff("Panic was caused by UUID: %s", matches[1])
						u := uuid.MustParse(matches[1])
						panickingUUIDsLock.Lock()
						panickingUUIDs[u] = true
						panickingUUIDsLock.Unlock()
					} else {
						otherpanic.Store(true)
					}
				}
			}()
			wd := NewWatchdog(ctx, time.NewTicker(1*time.Second), false, logger.For(logger.ComponentCommunicator))
			dog.Store(wd)
			wd.Start()
		}()
		// zap.S().Debuff("Finished BeforeEach")
		time.Sleep(100 * time.Millisecond)
	})

	AfterEach(func() {
		time.Sleep(10 * time.Millisecond)
		// zap.S().Debuff("Started AfterEach")
		dogCnclAtomic.Load().(context.CancelFunc)()
		// zap.S().Debuff("Finished AfterEach")
	})

	When("Registering a new heartbeat", func() {
		It("should register and return an UUID", func() {
			// zap.S().Debuff("Before RegisterHeartbeat")
			uuid := dog.Load().RegisterHeartbeat("test-1", 0, 0, false)
			Expect(uuid).ToNot(BeNil())
			// zap.S().Debuff("After RegisterHeartbeat")
		})
	})

	When("Loading twice", func() {
		It("should return the same dog", func() {
			load1 := dog.Load()
			load2 := dog.Load()
			Expect(load1).To(Equal(load2))
		})
	})

	When("Registering a new heartbeat", func() {
		It("should panic if the same name is used again", func() {
			uuid := dog.Load().RegisterHeartbeat("test-2", 0, 0, false)
			Expect(uuid).ToNot(BeNil())
			Expect(func() {
				dog.Load().RegisterHeartbeat("test-2", 0, 0, false)
			}).To(Panic())
		})
	})

	When("Not sending heartbeats", func() {
		It("should panic when the heartbeat is not sent", func() {
			uuid := dog.Load().RegisterHeartbeat("test-3", 0, 1, false)
			Expect(uuid).ToNot(BeNil())
			time.Sleep(3 * time.Second)
			panickingUUIDsLock.Lock()
			Expect(panickingUUIDs[uuid]).To(BeTrue())
			panickingUUIDsLock.Unlock()
		})
	})

	When("Sending heartbeats", func() {
		It("should not panic when the heartbeat is sent", func() {
			uuid := dog.Load().RegisterHeartbeat("test-4", 0, 5, false)
			Expect(uuid).ToNot(BeNil())
			time.Sleep(3 * time.Second)
			dog.Load().ReportHeartbeatStatus(uuid, HEARTBEAT_STATUS_OK)
			time.Sleep(3 * time.Second)
			panickingUUIDsLock.Lock()
			Expect(panickingUUIDs[uuid]).To(BeFalse())
			panickingUUIDsLock.Unlock()
		})
	})

	When("Sending unregistering", func() {
		It("should not panic", func() {
			uuid := dog.Load().RegisterHeartbeat("test-5", 0, 1, false)
			Expect(uuid).ToNot(BeNil())
			dog.Load().UnregisterHeartbeat(uuid)
			time.Sleep(3 * time.Second)
			panickingUUIDsLock.Lock()
			Expect(panickingUUIDs[uuid]).To(BeFalse())
			panickingUUIDsLock.Unlock()
		})
	})

	When("Sending warnings", func() {
		It("should not panic", func() {
			uuid := dog.Load().RegisterHeartbeat("test-6", 5, 0, false)
			Expect(uuid).ToNot(BeNil())
			for range 4 {
				dog.Load().ReportHeartbeatStatus(uuid, HEARTBEAT_STATUS_WARNING)
				panickingUUIDsLock.Lock()
				Expect(panickingUUIDs[uuid]).To(BeFalse())
				panickingUUIDsLock.Unlock()
			}
		})
	})

	When("Sending to many warnings", func() {
		It("should panic", func() {
			uuid := dog.Load().RegisterHeartbeat("test-7", 5, 0, false)
			Expect(uuid).ToNot(BeNil())
			for range 5 {
				dog.Load().ReportHeartbeatStatus(uuid, HEARTBEAT_STATUS_WARNING)
			}
			time.Sleep(1 * time.Second)
			panickingUUIDsLock.Lock()
			Expect(panickingUUIDs[uuid]).To(BeTrue())
			panickingUUIDsLock.Unlock()
		})
	})

	When("No subscriber is present and it is configured to only fail if they are", func() {
		It("should not panic", func() {
			uuid := dog.Load().RegisterHeartbeat("test-8", 5, 0, true)
			Expect(uuid).ToNot(BeNil())
			for range 5 {
				dog.Load().ReportHeartbeatStatus(uuid, HEARTBEAT_STATUS_WARNING)
			}
			time.Sleep(1 * time.Second)
			panickingUUIDsLock.Lock()
			Expect(panickingUUIDs[uuid]).To(BeFalse())
			panickingUUIDsLock.Unlock()
		})
	})

	When("Watchdog has restart callback", func() {
		It("should call restart function before panic", func() {
			var restartCalled atomic.Bool
			restartFunc := func() error {
				restartCalled.Store(true)

				return nil
			}

			uuid := dog.Load().RegisterHeartbeatWithRestart("test-restart-1", 0, 2, false, restartFunc)
			Expect(uuid).ToNot(BeNil())
			time.Sleep(3 * time.Second)

			Expect(restartCalled.Load()).To(BeTrue())
			panickingUUIDsLock.Lock()
			Expect(panickingUUIDs[uuid]).To(BeFalse())
			panickingUUIDsLock.Unlock()
		})
	})

	When("Watchdog restart callback fails", func() {
		It("should panic after failed restart", func() {
			restartFunc := func() error {
				return errors.New("restart failed")
			}

			uuid := dog.Load().RegisterHeartbeatWithRestart("test-restart-2", 0, 2, false, restartFunc)
			Expect(uuid).ToNot(BeNil())
			time.Sleep(3 * time.Second)

			panickingUUIDsLock.Lock()
			Expect(panickingUUIDs[uuid]).To(BeTrue())
			panickingUUIDsLock.Unlock()
		})
	})

	When("Watchdog has nil restart callback", func() {
		It("should panic immediately without restart attempt", func() {
			uuid := dog.Load().RegisterHeartbeatWithRestart("test-restart-3", 0, 2, false, nil)
			Expect(uuid).ToNot(BeNil())
			time.Sleep(3 * time.Second)

			panickingUUIDsLock.Lock()
			Expect(panickingUUIDs[uuid]).To(BeTrue())
			panickingUUIDsLock.Unlock()
		})
	})

	When("Watchdog restart succeeds and resets counter", func() {
		It("should reset counter after successful restart", func() {
			var restartCount atomic.Int32
			restartFunc := func() error {
				restartCount.Add(1)

				return nil
			}

			uuid := dog.Load().RegisterHeartbeatWithRestart("test-restart-4", 0, 2, false, restartFunc)
			Expect(uuid).ToNot(BeNil())
			time.Sleep(3 * time.Second)

			Expect(restartCount.Load()).To(Equal(int32(1)))

			time.Sleep(3 * time.Second)
			Expect(restartCount.Load()).To(Equal(int32(2)))

			panickingUUIDsLock.Lock()
			Expect(panickingUUIDs[uuid]).To(BeFalse())
			panickingUUIDsLock.Unlock()
		})
	})

	When("Multiple restart attempts occur", func() {
		It("should handle multiple restart cycles", func() {
			var restartCount atomic.Int32
			restartFunc := func() error {
				restartCount.Add(1)

				return nil
			}

			uuid := dog.Load().RegisterHeartbeatWithRestart("test-restart-5", 0, 2, false, restartFunc)
			Expect(uuid).ToNot(BeNil())
			time.Sleep(3 * time.Second)

			Expect(restartCount.Load()).To(BeNumerically(">=", int32(1)))
			panickingUUIDsLock.Lock()
			Expect(panickingUUIDs[uuid]).To(BeFalse())
			panickingUUIDsLock.Unlock()
		})
	})
})
