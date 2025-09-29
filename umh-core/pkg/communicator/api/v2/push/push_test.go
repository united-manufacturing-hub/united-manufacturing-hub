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

var _ = Describe("Push non-blocking behavior", func() {
	It("should not block when channel is full", func() {
		channelSize := 5
		outboundChannel := make(chan *models.UMHMessage, channelSize)
		deadletterCh := make(chan push.DeadLetter, 100)

		logger, _ := zap.NewDevelopment()
		sugaredLogger := logger.Sugar()
		ctx := context.Background()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		dog := watchdog.NewWatchdog(ctx, ticker, false, sugaredLogger)

		backoff := tools.NewBackoff(time.Millisecond, time.Millisecond, time.Second, tools.BackoffPolicyExponential)
		pusher := push.NewPusher(
			uuid.New(),
			"test-jwt",
			dog,
			outboundChannel,
			deadletterCh,
			backoff,
			true,
			"http://localhost:8080",
			sugaredLogger,
		)

		// fill the channel
		for range channelSize {
			pusher.Push(models.UMHMessage{
				Content: "fill-message",
				Email:   "test@example.com",
			})
		}

		Expect(outboundChannel).To(HaveLen(channelSize))

		start := time.Now()
		pusher.Push(models.UMHMessage{
			Content: "overflow-message",
			Email:   "test@example.com",
		})
		duration := time.Since(start)

		Expect(duration).To(BeNumerically("<", 5*time.Millisecond))

		// channel should still be full (oldest dropped, newest added)
		Expect(outboundChannel).To(HaveLen(channelSize))

		close(outboundChannel)
		close(deadletterCh)
	})
})
