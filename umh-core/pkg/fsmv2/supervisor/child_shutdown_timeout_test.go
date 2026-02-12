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

package supervisor_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

var _ = Describe("Child Shutdown Timeout", func() {
	Describe("when child done channel never closes", func() {
		It("should not block forever during Shutdown", func() {
			observedLogs, logger := createChildShutdownObservedLogger()

			store := supervisor.CreateTestTriangularStore()

			s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType:              "test",
				Store:                   store,
				Logger:                  logger,
				TickInterval:            50 * time.Millisecond,
				GracefulShutdownTimeout: 100 * time.Millisecond,
				ChildShutdownTimeout:    200 * time.Millisecond,
			})

			child := supervisor.CreateTestSupervisorWithCircuitState(false)

			stuckDone := make(chan struct{})
			defer close(stuckDone)

			s.TestSetChild("stuck-child", child, stuckDone)
			s.TestMarkAsStarted()

			shutdownDone := make(chan struct{})
			go func() {
				s.Shutdown()
				close(shutdownDone)
			}()

			Eventually(shutdownDone, 2*time.Second).Should(BeClosed(),
				"Shutdown should complete within timeout, not block forever")

			timeoutLogs := filterChildShutdownLogs(observedLogs, "child_shutdown_timeout")
			Expect(timeoutLogs).ToNot(BeEmpty(), "Expected child_shutdown_timeout log entry")

			timeoutLog := timeoutLogs[0]
			Expect(timeoutLog.ContextMap()).To(HaveKey("child_name"))
			Expect(timeoutLog.ContextMap()["child_name"]).To(Equal("stuck-child"))
		})

		It("should proceed normally when child shuts down within timeout", func() {
			_, logger := createChildShutdownObservedLogger()

			store := supervisor.CreateTestTriangularStore()

			s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType:              "test",
				Store:                   store,
				Logger:                  logger,
				TickInterval:            50 * time.Millisecond,
				GracefulShutdownTimeout: 100 * time.Millisecond,
				ChildShutdownTimeout:    1 * time.Second,
			})

			child := supervisor.CreateTestSupervisorWithCircuitState(false)

			normalDone := make(chan struct{})

			s.TestSetChild("normal-child", child, normalDone)
			s.TestMarkAsStarted()

			go func() {
				time.Sleep(50 * time.Millisecond)
				close(normalDone)
			}()

			shutdownDone := make(chan struct{})
			go func() {
				s.Shutdown()
				close(shutdownDone)
			}()

			Eventually(shutdownDone, 2*time.Second).Should(BeClosed(),
				"Shutdown should complete when child finishes normally")
		})
	})
})

func createChildShutdownObservedLogger() (*observer.ObservedLogs, deps.FSMLogger) {
	core, logs := observer.New(zapcore.DebugLevel)
	logger := deps.NewFSMLogger(zap.New(core).Sugar())

	return logs, logger
}

func filterChildShutdownLogs(logs *observer.ObservedLogs, message string) []observer.LoggedEntry {
	var filtered []observer.LoggedEntry

	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, message) {
			filtered = append(filtered, entry)
		}
	}

	return filtered
}
