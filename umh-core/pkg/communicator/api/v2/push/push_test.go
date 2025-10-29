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

package push_test

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

type MockWatchdog struct {
	statusReports []watchdog.HeartbeatStatus
	mu            sync.Mutex
}

func NewMockWatchdog() *MockWatchdog {
	return &MockWatchdog{
		statusReports: []watchdog.HeartbeatStatus{},
	}
}

func (m *MockWatchdog) Start() {}

func (m *MockWatchdog) RegisterHeartbeat(name string, warningsUntilFailure uint64, timeout uint64, onlyIfSubscribers bool) uuid.UUID {
	return uuid.New()
}

func (m *MockWatchdog) RegisterHeartbeatWithRestart(name string, warningsUntilFailure uint64, timeout uint64, onlyIfSubscribers bool, restartFunc func() error) uuid.UUID {
	return uuid.New()
}

func (m *MockWatchdog) UnregisterHeartbeat(uniqueIdentifier uuid.UUID) {}

func (m *MockWatchdog) ReportHeartbeatStatus(uniqueIdentifier uuid.UUID, status watchdog.HeartbeatStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusReports = append(m.statusReports, status)
}

func (m *MockWatchdog) SetHasSubscribers(has bool) {}

func (m *MockWatchdog) GetStatusReports() []watchdog.HeartbeatStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]watchdog.HeartbeatStatus{}, m.statusReports...)
}

func (m *MockWatchdog) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusReports = []watchdog.HeartbeatStatus{}
}

var _ = Describe("Push Restart", func() {
	var (
		pusher          *push.Pusher
		dog             *watchdog.Watchdog
		outboundChannel chan *models.UMHMessage
		deadletterCh    chan push.DeadLetter
		ctx             context.Context
		cancel          context.CancelFunc
		testLogger      *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		testLogger = logger.For(logger.ComponentCommunicator)
		dog = watchdog.NewWatchdog(ctx, time.NewTicker(1*time.Second), false, testLogger)
		go dog.Start()

		outboundChannel = make(chan *models.UMHMessage, 100)
		deadletterCh = push.DefaultDeadLetterChanBuffer()
		backoff := push.DefaultBackoffPolicy()
		pusher = push.NewPusher(uuid.New(), "test-jwt", dog, outboundChannel, deadletterCh, backoff, true, "http://test-api", testLogger)
	})

	AfterEach(func() {
		if pusher != nil {
			pusher.Stop()
			time.Sleep(100 * time.Millisecond)
		}
		cancel()
	})

	Describe("Restart", func() {
		It("should stop, reset HTTP client, wait 5s, and restart", func() {
			pusher.Start()
			time.Sleep(100 * time.Millisecond)

			startTime := time.Now()
			err := pusher.Restart()
			elapsed := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred())
			Expect(elapsed).To(BeNumerically(">=", 5*time.Second))
			Expect(elapsed).To(BeNumerically("<", 6*time.Second))
		})

		It("should be thread-safe with concurrent restarts", func() {
			pusher.Start()
			time.Sleep(100 * time.Millisecond)

			var wg sync.WaitGroup
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					Expect(pusher.Restart()).To(Succeed())
				}()
			}

			wg.Wait()
		})

		It("should work when pusher is not running", func() {
			err := pusher.Restart()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Stop", func() {
		It("should be idempotent when called multiple times", func() {
			pusher.Start()
			time.Sleep(100 * time.Millisecond)

			pusher.Stop()
			pusher.Stop()
			pusher.Stop()
		})

		It("should be thread-safe with concurrent stops", func() {
			pusher.Start()
			time.Sleep(100 * time.Millisecond)

			var wg sync.WaitGroup
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					pusher.Stop()
				}()
			}

			wg.Wait()
		})
	})
})

var _ = Describe("Watchdog Heartbeat Bug Fix", func() {
	var (
		pusher          *push.Pusher
		mockDog         *MockWatchdog
		outboundChannel chan *models.UMHMessage
		deadletterCh    chan push.DeadLetter
		testLogger      *zap.SugaredLogger
	)

	BeforeEach(func() {
		testLogger = logger.For(logger.ComponentCommunicator)
		mockDog = NewMockWatchdog()
		outboundChannel = make(chan *models.UMHMessage, 100)
		deadletterCh = push.DefaultDeadLetterChanBuffer()
		backoff := push.DefaultBackoffPolicy()
		pusher = push.NewPusher(uuid.New(), "test-jwt", mockDog, outboundChannel, deadletterCh, backoff, true, "https://management.umh.app", testLogger)
	})

	AfterEach(func() {
		if pusher != nil {
			pusher.Stop()
			time.Sleep(100 * time.Millisecond)
		}
	})

	It("should NOT report heartbeat OK when HTTP request fails", func() {
		pusher.Start()
		time.Sleep(50 * time.Millisecond)

		mockDog.Reset()

		msg := &models.UMHMessage{
			InstanceUUID: uuid.New(),
			Email:        "test@example.com",
			Content:      "test-message",
		}
		outboundChannel <- msg

		time.Sleep(500 * time.Millisecond)

		reports := mockDog.GetStatusReports()

		okCount := 0
		for _, status := range reports {
			if status == watchdog.HEARTBEAT_STATUS_OK {
				okCount++
			}
		}

		Expect(okCount).To(Equal(0), "Expected NO heartbeat OK reports when HTTP fails, but got %d", okCount)
	})

	It("should report heartbeat OK ONLY after HTTP request succeeds", func() {
		pusher.Start()
		time.Sleep(50 * time.Millisecond)

		mockDog.Reset()

		time.Sleep(100 * time.Millisecond)

		reports := mockDog.GetStatusReports()

		foundOK := false
		for _, status := range reports {
			if status == watchdog.HEARTBEAT_STATUS_OK {
				foundOK = true
				break
			}
		}

		Expect(foundOK).To(BeFalse(), "Expected heartbeat OK to be reported AFTER HTTP success, but it was reported before any HTTP request")
	})
})
