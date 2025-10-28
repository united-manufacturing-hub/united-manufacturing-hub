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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/pull"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

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

			Expect(err).To(BeNil())
			Expect(elapsed).To(BeNumerically(">=", 5*time.Second))
			Expect(elapsed).To(BeNumerically("<", 6*time.Second))
		})

		It("should be thread-safe with concurrent restarts", func() {
			puller.Start()
			time.Sleep(100 * time.Millisecond)

			var wg sync.WaitGroup
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					puller.Restart()
				}()
			}

			wg.Wait()
		})

		It("should work when puller is not running", func() {
			err := puller.Restart()
			Expect(err).To(BeNil())
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
