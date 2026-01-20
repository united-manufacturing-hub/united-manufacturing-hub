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
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/pull"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// MockWatchdog captures heartbeat status reports for testing.
type MockWatchdog struct {
	mu              sync.Mutex
	statusReports   []watchdog.HeartbeatStatus
	registeredNames []string
}

func NewMockWatchdog() *MockWatchdog {
	return &MockWatchdog{
		statusReports:   make([]watchdog.HeartbeatStatus, 0),
		registeredNames: make([]string, 0),
	}
}

func (m *MockWatchdog) Start() {}

func (m *MockWatchdog) RegisterHeartbeat(name string, warningsUntilFailure uint64, timeout uint64, onlyIfSubscribers bool) uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.registeredNames = append(m.registeredNames, name)

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

	result := make([]watchdog.HeartbeatStatus, len(m.statusReports))
	copy(result, m.statusReports)

	return result
}

func (m *MockWatchdog) GetLastStatus() watchdog.HeartbeatStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.statusReports) == 0 {
		return watchdog.HEARTBEAT_STATUS_OK
	}

	return m.statusReports[len(m.statusReports)-1]
}

func (m *MockWatchdog) CountStatus(status watchdog.HeartbeatStatus) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0

	for _, s := range m.statusReports {
		if s == status {
			count++
		}
	}

	return count
}

var _ = Describe("Puller Health Check Timing", func() {
	var (
		mockDog        *MockWatchdog
		inboundChannel chan *models.UMHMessage
		logger         *zap.SugaredLogger
	)

	BeforeEach(func() {
		mockDog = NewMockWatchdog()
		inboundChannel = make(chan *models.UMHMessage, 100)
		logger = zap.NewNop().Sugar()
	})

	Describe("GetCurrentStatus", func() {
		Context("when no request is in flight", func() {
			It("should return OK status if no recent errors", func() {
				puller := pull.NewPuller("test-jwt", mockDog, inboundChannel, true, "http://localhost", logger)

				status := puller.GetCurrentStatus()
				Expect(status).To(Equal(watchdog.HEARTBEAT_STATUS_OK))
			})
		})

		Context("when request is in flight for less than threshold", func() {
			It("should return OK status", func() {
				puller := pull.NewPuller("test-jwt", mockDog, inboundChannel, true, "http://localhost", logger)

				// Simulate request starting
				puller.SetRequestInFlight(true)
				puller.SetRequestStartTime(time.Now())

				status := puller.GetCurrentStatus()
				Expect(status).To(Equal(watchdog.HEARTBEAT_STATUS_OK))

				puller.SetRequestInFlight(false)
			})
		})

		Context("when request is in flight for more than threshold (10s)", func() {
			It("should return WARNING status", func() {
				puller := pull.NewPuller("test-jwt", mockDog, inboundChannel, true, "http://localhost", logger)

				// Simulate request that started > 10s ago
				puller.SetRequestInFlight(true)
				puller.SetRequestStartTime(time.Now().Add(-15 * time.Second))

				status := puller.GetCurrentStatus()
				Expect(status).To(Equal(watchdog.HEARTBEAT_STATUS_WARNING))

				puller.SetRequestInFlight(false)
			})
		})

		Context("when last request failed", func() {
			It("should return WARNING status", func() {
				puller := pull.NewPuller("test-jwt", mockDog, inboundChannel, true, "http://localhost", logger)

				// Simulate failed request
				puller.SetLastRequestFailed(true)

				status := puller.GetCurrentStatus()
				Expect(status).To(Equal(watchdog.HEARTBEAT_STATUS_WARNING))
			})
		})
	})

	Describe("Request tracking", func() {
		It("should track request start time", func() {
			puller := pull.NewPuller("test-jwt", mockDog, inboundChannel, true, "http://localhost", logger)

			before := time.Now()
			puller.SetRequestStartTime(time.Now())
			after := time.Now()

			startTime := puller.GetRequestStartTime()
			Expect(startTime).To(BeTemporally(">=", before))
			Expect(startTime).To(BeTemporally("<=", after))
		})

		It("should track request in-flight state", func() {
			puller := pull.NewPuller("test-jwt", mockDog, inboundChannel, true, "http://localhost", logger)

			Expect(puller.IsRequestInFlight()).To(BeFalse())

			puller.SetRequestInFlight(true)
			Expect(puller.IsRequestInFlight()).To(BeTrue())

			puller.SetRequestInFlight(false)
			Expect(puller.IsRequestInFlight()).To(BeFalse())
		})
	})
})
