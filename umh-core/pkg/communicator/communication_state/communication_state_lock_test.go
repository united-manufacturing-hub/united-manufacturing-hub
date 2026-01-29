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

package communication_state_test

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	communication_state "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

var _ = Describe("CommunicationState Lock Ordering", func() {
	var (
		state  *communication_state.CommunicationState
		logger *zap.SugaredLogger
	)

	BeforeEach(func() {
		zapLogger, _ := zap.NewDevelopment()
		logger = zapLogger.Sugar()

		inboundChan := make(chan *models.UMHMessage, 100)
		outboundChan := make(chan *models.UMHMessage, 100)

		state = communication_state.NewCommunicationState(
			nil, // watchdog - nil to avoid goroutines
			inboundChan,
			outboundChan,
			config.ReleaseChannelStable,
			nil, // systemSnapshotManager
			nil, // configManager
			"http://localhost",
			logger,
			false,
			nil, // topicBrowserCache
		)
	})

	Describe("concurrent SetLoginResponseForFSMv2 calls", func() {
		It("should not deadlock when called many times concurrently", func() {
			// This test focuses on SetLoginResponseForFSMv2's internal lock ordering.
			// The method acquires LoginResponseMu then mu - if concurrent calls
			// interleave incorrectly, they could deadlock.
			//
			// Running many concurrent calls stress tests the lock ordering.
			// A timeout detects deadlocks.

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			done := make(chan struct{})

			go func() {
				var wg sync.WaitGroup

				// Run many concurrent SetLoginResponseForFSMv2 calls
				for range 200 {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()
						state.SetLoginResponseForFSMv2(uuid.New().String())
					}()
				}

				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				// Success - no deadlock
			case <-ctx.Done():
				Fail("Deadlock detected - concurrent SetLoginResponseForFSMv2 calls hung")
			}
		})
	})
})
