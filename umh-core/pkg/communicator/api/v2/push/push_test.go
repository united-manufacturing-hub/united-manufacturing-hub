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
	"net/http"
	"net/http/httptest"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

type MockWatchdog struct {
	warningCount int
	okCount      int
	watcherUUID  uuid.UUID
}

func (m *MockWatchdog) Start() {}

func (m *MockWatchdog) RegisterHeartbeat(name string, warningsUntilFailure uint64, timeout uint64, onlyIfSubscribers bool) uuid.UUID {
	m.watcherUUID = uuid.New()
	return m.watcherUUID
}

func (m *MockWatchdog) UnregisterHeartbeat(uniqueIdentifier uuid.UUID) {}

func (m *MockWatchdog) ReportHeartbeatStatus(uniqueIdentifier uuid.UUID, status watchdog.HeartbeatStatus) {
	if uniqueIdentifier != m.watcherUUID {
		return
	}
	if status == watchdog.HEARTBEAT_STATUS_WARNING {
		m.warningCount++
	} else if status == watchdog.HEARTBEAT_STATUS_OK {
		m.okCount++
		m.warningCount = 0
	}
}

func (m *MockWatchdog) SetHasSubscribers(has bool) {}

func (m *MockWatchdog) GetWarningCount() int {
	return m.warningCount
}

func (m *MockWatchdog) GetOKCount() int {
	return m.okCount
}

var _ = Describe("Pusher Deadletter Handler", func() {
	var (
		mockDog      *MockWatchdog
		outboundCh   chan *models.UMHMessage
		deadletterCh chan push.DeadLetter
		logger       *zap.SugaredLogger
		instanceUUID uuid.UUID
	)

	BeforeEach(func() {
		mockDog = &MockWatchdog{}
		outboundCh = make(chan *models.UMHMessage, 100)
		deadletterCh = push.DefaultDeadLetterChanBuffer()
		logger = zap.NewNop().Sugar()
		instanceUUID = uuid.New()
	})

	AfterEach(func() {
		close(outboundCh)
		close(deadletterCh)
	})

	Context("when processing deadletter messages with failed HTTP requests", func() {
		It("should accumulate watchdog warnings without resetting on failures", func() {
			mockHTTPServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer mockHTTPServer.Close()

			backoff := push.DefaultBackoffPolicy()
			pusher := push.NewPusher(instanceUUID, "test-jwt", mockDog, outboundCh, deadletterCh, backoff, true, mockHTTPServer.URL, logger)
			pusher.Start()

			messages := []models.UMHMessage{
				{InstanceUUID: instanceUUID, Content: "test message", Email: "test@example.com"},
			}
			cookies := map[string]string{"token": "test-jwt"}

			deadletterCh <- push.DeadLetter{
				Cookies:       cookies,
				Messages:      messages,
				RetryAttempts: 0,
			}

			Eventually(func() int {
				return mockDog.GetWarningCount()
			}, "5s").Should(BeNumerically(">=", 1))

			Consistently(func() int {
				return mockDog.GetWarningCount()
			}, "2s").Should(BeNumerically(">=", 1))

			Expect(mockDog.GetOKCount()).To(Equal(0))
		})

		It("should report OK only after successful POST request", func() {
			mockHTTPServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer mockHTTPServer.Close()

			backoff := push.DefaultBackoffPolicy()
			pusher := push.NewPusher(instanceUUID, "test-jwt", mockDog, outboundCh, deadletterCh, backoff, true, mockHTTPServer.URL, logger)
			pusher.Start()

			messages := []models.UMHMessage{
				{InstanceUUID: instanceUUID, Content: "test message", Email: "test@example.com"},
			}
			cookies := map[string]string{"token": "test-jwt"}

			deadletterCh <- push.DeadLetter{
				Cookies:       cookies,
				Messages:      messages,
				RetryAttempts: 0,
			}

			Eventually(func() int {
				return mockDog.GetOKCount()
			}, "5s").Should(BeNumerically(">=", 1))

			Expect(mockDog.GetWarningCount()).To(Equal(0))
		})
	})
})
