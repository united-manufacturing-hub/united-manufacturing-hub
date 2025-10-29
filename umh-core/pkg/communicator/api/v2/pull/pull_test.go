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

package pull_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/pull"
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

var _ = Describe("Pull Restart", func() {
	var (
		puller         *pull.Puller
		dog            *watchdog.Watchdog
		inboundChannel chan *models.UMHMessage
		ctx            context.Context
		cancel         context.CancelFunc
		testLogger     *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		testLogger = logger.For(logger.ComponentCommunicator)
		dog = watchdog.NewWatchdog(ctx, time.NewTicker(1*time.Second), false, testLogger)
		go dog.Start()

		inboundChannel = make(chan *models.UMHMessage, 100)
		puller = pull.NewPuller("test-jwt", dog, inboundChannel, true, "http://test-api", testLogger)
	})

	AfterEach(func() {
		if puller != nil {
			puller.Stop()
			time.Sleep(100 * time.Millisecond)
		}
		cancel()
	})

	Describe("Restart", func() {
		It("should stop, reset HTTP client, wait 5s, and restart", func() {
			puller.Start()
			time.Sleep(100 * time.Millisecond)

			startTime := time.Now()
			err := puller.Restart()
			elapsed := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred())
			Expect(elapsed).To(BeNumerically(">=", 5*time.Second))
			Expect(elapsed).To(BeNumerically("<=", 7*time.Second))
		})

		It("should be thread-safe with concurrent restarts", func() {
			puller.Start()
			time.Sleep(100 * time.Millisecond)

			var wg sync.WaitGroup
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					Expect(puller.Restart()).To(Succeed())
				}()
			}

			wg.Wait()
		})

		It("should work when puller is not running", func() {
			err := puller.Restart()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Stop", func() {
		It("should be idempotent when called multiple times", func() {
			puller.Start()
			time.Sleep(100 * time.Millisecond)

			puller.Stop()
			puller.Stop()
			puller.Stop()
		})

		It("should be thread-safe with concurrent stops", func() {
			puller.Start()
			time.Sleep(100 * time.Millisecond)

			var wg sync.WaitGroup
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					puller.Stop()
				}()
			}

			wg.Wait()
		})
	})
})

var _ = Describe("Watchdog Heartbeat Bug Fix", func() {
	var (
		puller         *pull.Puller
		mockDog        *MockWatchdog
		inboundChannel chan *models.UMHMessage
		testLogger     *zap.SugaredLogger
	)

	BeforeEach(func() {
		testLogger = logger.For(logger.ComponentCommunicator)
		mockDog = NewMockWatchdog()
		inboundChannel = make(chan *models.UMHMessage, 100)
		puller = pull.NewPuller("test-jwt", mockDog, inboundChannel, true, "https://management.umh.app", testLogger)
	})

	AfterEach(func() {
		if puller != nil {
			puller.Stop()
			time.Sleep(100 * time.Millisecond)
		}
	})

	It("should NOT report heartbeat OK when HTTP request fails", func() {
		puller.Start()
		time.Sleep(50 * time.Millisecond)

		mockDog.Reset()

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
})

var _ = Describe("HTTP Request Cancellation on Stop", func() {
	var (
		puller         *pull.Puller
		mockDog        *MockWatchdog
		inboundChannel chan *models.UMHMessage
		testLogger     *zap.SugaredLogger
		slowServer     *httptest.Server
		serverBlocked  chan struct{}
	)

	BeforeEach(func() {
		testLogger = logger.For(logger.ComponentCommunicator)
		mockDog = NewMockWatchdog()
		inboundChannel = make(chan *models.UMHMessage, 100)

		serverBlocked = make(chan struct{})
		slowServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serverBlocked <- struct{}{}
			time.Sleep(35 * time.Second)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"umh_messages": []}`))
		}))

		puller = pull.NewPuller("test-jwt", mockDog, inboundChannel, true, slowServer.URL, testLogger)
	})

	AfterEach(func() {
		if puller != nil {
			puller.Stop()
			time.Sleep(100 * time.Millisecond)
		}
		if slowServer != nil {
			slowServer.Close()
		}
	})

	It("should cancel HTTP request when Stop is called", func() {
		puller.Start()

		select {
		case <-serverBlocked:
		case <-time.After(2 * time.Second):
			Fail("Server never received request")
		}

		startTime := time.Now()
		done := puller.Stop()

		select {
		case <-done:
			elapsed := time.Since(startTime)
			Expect(elapsed).To(BeNumerically("<", 2*time.Second), "Expected Stop() to return quickly (<2s), but took %v. HTTP request was not cancelled.", elapsed)
		case <-time.After(40 * time.Second):
			Fail("Stop() did not complete within 40 seconds. HTTP request is blocking shutdown.")
		}
	})

	It("should handle context cancellation gracefully without errors", func() {
		puller.Start()

		select {
		case <-serverBlocked:
		case <-time.After(2 * time.Second):
			Fail("Server never received request")
		}

		done := puller.Stop()
		<-done

		time.Sleep(100 * time.Millisecond)
	})
})

var _ = Describe("Channel Send stopChan Monitoring", func() {
	var (
		puller         *pull.Puller
		mockDog        *MockWatchdog
		inboundChannel chan *models.UMHMessage
		testLogger     *zap.SugaredLogger
		testServer     *httptest.Server
	)

	BeforeEach(func() {
		testLogger = logger.For(logger.ComponentCommunicator)
		mockDog = NewMockWatchdog()
		inboundChannel = make(chan *models.UMHMessage, 1)

		testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := `{"umh_messages": [
				{"email": "test1@example.com", "content": "message1", "instance_uuid": "test-instance", "metadata": {}},
				{"email": "test2@example.com", "content": "message2", "instance_uuid": "test-instance", "metadata": {}}
			]}`

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		}))

		puller = pull.NewPuller("test-jwt", mockDog, inboundChannel, true, testServer.URL, testLogger)
	})

	AfterEach(func() {
		if puller != nil {
			puller.Stop()
			time.Sleep(100 * time.Millisecond)
		}
		if testServer != nil {
			testServer.Close()
		}
	})

	It("should monitor stopChan during channel send operations", func() {
		puller.Start()

		Eventually(func() int {
			return len(inboundChannel)
		}, 3*time.Second, 50*time.Millisecond).Should(Equal(1), "Channel should receive at least 1 message")

		time.Sleep(100 * time.Millisecond)

		startTime := time.Now()
		done := puller.Stop()

		select {
		case <-done:
			elapsed := time.Since(startTime)
			Expect(elapsed).To(BeNumerically("<", 2*time.Second), "Stop() should return quickly by checking stopChan in channel send select")
		case <-time.After(12 * time.Second):
			Fail("Stop() blocked > 12s, likely waiting for 10s insertionTimeout without stopChan check")
		}
	})
})
