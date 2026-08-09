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

package subscriber_test

import (
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/subscriber"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
	"go.uber.org/zap"
)

// mockWatchdog implements watchdog.Iface for testing.
type mockWatchdog struct{}

func (m *mockWatchdog) Start()                                                    {}
func (m *mockWatchdog) RegisterHeartbeat(string, uint64, uint64, bool) uuid.UUID  { return uuid.New() }
func (m *mockWatchdog) UnregisterHeartbeat(uuid.UUID)                             {}
func (m *mockWatchdog) ReportHeartbeatStatus(uuid.UUID, watchdog.HeartbeatStatus) {}
func (m *mockWatchdog) SetHasSubscribers(bool)                                    {}

var _ = Describe("SubscriberHandler Race Condition", func() {
	var (
		handler *subscriber.Handler
		logger  *zap.SugaredLogger
	)

	BeforeEach(func() {
		zapLogger, _ := zap.NewDevelopment()
		logger = zapLogger.Sugar()

		// Create handler with minimal dependencies - the FSMv2 channel is required
		// (NewHandler rejects nil) but unused by the instanceUUID race tests.
		handler = subscriber.NewHandler(
			&mockWatchdog{},
			uuid.New(),
			time.Minute,
			time.Minute,
			config.ReleaseChannelStable,
			false,
			nil, // systemSnapshotManager
			nil, // configManager
			logger,
			nil, // topicBrowserCommunicator
			make(chan *types.UMHMessage, 10),
			nil, // featureUsage
		)
	})

	Describe("instanceUUID thread safety", func() {
		It("should handle concurrent SetInstanceUUID and GetInstanceUUID calls without race conditions", func() {
			// This test verifies that the race condition fix is in place.
			// Without mutex protection, running this test with -race will fail.

			var wg sync.WaitGroup
			iterations := 100

			// Start multiple goroutines writing to instanceUUID
			for range iterations {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					handler.SetInstanceUUID(uuid.New())
				}()
			}

			// Start multiple goroutines reading from instanceUUID
			for range iterations {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					_ = handler.GetInstanceUUID()
				}()
			}

			wg.Wait()
			// If we reach here without race detector complaints, the test passes
		})

		It("should return the correct UUID after SetInstanceUUID is called", func() {
			expectedUUID := uuid.New()
			handler.SetInstanceUUID(expectedUUID)

			actualUUID := handler.GetInstanceUUID()
			Expect(actualUUID).To(Equal(expectedUUID))
		})
	})
})
