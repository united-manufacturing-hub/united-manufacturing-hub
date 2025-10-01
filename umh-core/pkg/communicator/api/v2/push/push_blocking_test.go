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
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

var _ = Describe("Push blocking under connection loss", func() {
	It("should not block when backend is unresponsive and channels fill up", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(30 * time.Second)
		}))
		defer server.Close()

		channelSize := 5
		outboundChannel := make(chan *models.UMHMessage, channelSize)
		deadletterCh := make(chan push.DeadLetter, 10)

		logger, _ := zap.NewDevelopment()
		sugaredLogger := logger.Sugar()
		ctx := context.Background()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		dog := watchdog.NewWatchdog(ctx, ticker, false, sugaredLogger)

		backoff := tools.NewBackoff(time.Millisecond, time.Millisecond, 10*time.Millisecond, tools.BackoffPolicyExponential)
		pusher := push.NewPusher(
			uuid.New(),
			"test-jwt",
			dog,
			outboundChannel,
			deadletterCh,
			backoff,
			true,
			server.URL,
			sugaredLogger,
		)

		pusher.Start()

		var wg sync.WaitGroup
		messageCount := 200
		blockedCount := 0
		var mu sync.Mutex

		for i := range messageCount {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				start := time.Now()
				pusher.Push(models.UMHMessage{
					Content: "test-message",
					Email:   "test@example.com",
				})
				duration := time.Since(start)

				if duration > 100*time.Millisecond {
					mu.Lock()
					blockedCount++
					mu.Unlock()
				}
			}(i)
		}

		done := make(chan bool)
		go func() {
			wg.Wait()
			done <- true
		}()

		select {
		case <-done:
			Expect(blockedCount).To(Equal(0), "No Push() calls should block")
		case <-time.After(5 * time.Second):
			Fail("Test timed out - Push() is blocking!")
		}
	})
})
