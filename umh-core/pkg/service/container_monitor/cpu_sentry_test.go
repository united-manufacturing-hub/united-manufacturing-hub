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

package container_monitor_test

import (
	"context"
	"os"
	"sync"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// capturingSentryTransport keeps every event the Sentry client sends, instead
// of sending it over the network.
type capturingSentryTransport struct {
	events []*sentrygo.Event
	mu     sync.Mutex
}

func (t *capturingSentryTransport) Configure(_ sentrygo.ClientOptions)      {}
func (t *capturingSentryTransport) Flush(_ time.Duration) bool              { return true }
func (t *capturingSentryTransport) FlushWithContext(_ context.Context) bool { return true }
func (t *capturingSentryTransport) Close()                                  {}

func (t *capturingSentryTransport) SendEvent(event *sentrygo.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.events = append(t.events, event)
}

// neverStartedEvents counts the captured events that carry the never-started
// warning and the CPU feature tag.
func (t *capturingSentryTransport) neverStartedEvents() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	count := 0

	for _, event := range t.events {
		if event.Message == seamStillStartingWarning && event.Tags["feature"] == string(deps.FeatureSupportCPU) {
			count++
		}
	}

	return count
}

var _ = Describe("a CPU worker that never started", func() {
	It("is reported to Sentry once per process, however many services tick", func() {
		previousFlag, hadFlag := os.LookupEnv(usefsmv2CPUEnv)
		Expect(os.Setenv(usefsmv2CPUEnv, "true")).To(Succeed())
		DeferCleanup(func() {
			if hadFlag {
				_ = os.Setenv(usefsmv2CPUEnv, previousFlag)
			} else {
				_ = os.Unsetenv(usefsmv2CPUEnv)
			}
		})

		// A nil client is what the seam sees when the fsmv2 supervisor never
		// started.
		previousClient := fsmv2client.GetClient()

		fsmv2client.SetClient(nil)
		DeferCleanup(func() { fsmv2client.SetClient(previousClient) })

		// The Sentry hook forwards only levels its inner core accepts, so the
		// component logger must accept warnings.
		logger.GetLogger()

		core, _ := observer.New(zapcore.WarnLevel)
		DeferCleanup(zap.ReplaceGlobals(zap.New(core)))

		transport := &capturingSentryTransport{}
		Expect(sentrygo.Init(sentrygo.ClientOptions{
			Dsn:       "https://test@sentry.io/123",
			Transport: transport,
		})).To(Succeed())
		DeferCleanup(func() { _ = sentrygo.Init(sentrygo.ClientOptions{}) })

		container_monitor.ResetCPUWorkerNeverStartedWarning()

		testDataPath := GinkgoT().TempDir()

		for range 2 {
			service := container_monitor.NewContainerMonitorServiceWithPath(filesystem.NewMockFileSystem(), testDataPath)

			for range 3 {
				status, err := service.GetStatus(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(status.CPUHealth).To(Equal(models.Degraded))
			}
		}

		sentrygo.Flush(time.Second)
		Expect(transport.neverStartedEvents()).To(Equal(1))
	})
})
