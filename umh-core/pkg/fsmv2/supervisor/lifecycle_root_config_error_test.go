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
	"context"
	"errors"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// countingDeriveWorker returns its err from every derive once err is set, and
// counts derive calls so a test can wait for whole tick intervals of a
// started root supervisor without sleeping.
type countingDeriveWorker struct {
	err         error
	deriveCalls atomic.Int64
}

func (w *countingDeriveWorker) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: "test-worker", CollectedAt: time.Now()}, nil
}

func (w *countingDeriveWorker) DeriveDesiredState(any) (fsmv2.DesiredState, error) {
	w.deriveCalls.Add(1)

	if w.err != nil {
		return nil, w.err
	}

	return &config.DesiredState{State: "running"}, nil
}

func (w *countingDeriveWorker) GetInitialState() fsmv2.State[any, any] { return &mockState{} }

func sentryErrorCount(recorder *recordingSentryLogger, event string) int {
	count := 0

	for _, e := range recorder.sentryErrorEvents() {
		if e == event {
			count++
		}
	}

	return count
}

var _ = Describe("Supervisor root tick loop logging", func() {
	It("keeps a root worker's config error out of Sentry while a plain derive error still reaches it", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// The root worker's config error must not reach Sentry from the
		// tick loop: warnConfigErrorOnce in the derive path already reported
		// it.
		configRecorder := newRecordingSentryLogger()
		configSuper := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              "test",
			Store:                   createTestTriangularStore(),
			Logger:                  configRecorder,
			TickInterval:            20 * time.Millisecond,
			GracefulShutdownTimeout: 100 * time.Millisecond,
		})

		configWorker := &countingDeriveWorker{}
		Expect(configSuper.AddWorker(mockIdentity(), configWorker)).NotTo(HaveOccurred())
		configWorker.err = &config.ConfigError{Field: "read", Err: errors.New("value is not a number")}

		configSuper.Start(ctx)
		defer configSuper.Shutdown()

		// Wait for the first config_error warning.
		Eventually(func() int {
			return configRecorder.warnCount("config_error")
		}, 5*time.Second).Should(BeNumerically(">=", 1))

		// Then wait more tick intervals by counting derive calls. Every tick
		// of this supervisor derives: the derive cache stores only successful
		// derives, so a failed derive is retried next tick.
		afterWarn := configWorker.deriveCalls.Load()

		Eventually(func() int64 {
			return configWorker.deriveCalls.Load()
		}, 5*time.Second).Should(BeNumerically(">=", afterWarn+5))

		Expect(sentryErrorCount(configRecorder, "tick_error")).To(Equal(0),
			"a config error must not reach Sentry from the root tick loop")
		Expect(configRecorder.sentryWarnEvents()).To(BeEmpty(),
			"a config error must not reach Sentry as a warning either")
		Expect(configRecorder.warnCount("config_error")).To(Equal(1),
			"the config error is warned exactly once")

		// A plain derive error is not a config error, so the root tick loop
		// still sends it to Sentry.
		plainRecorder := newRecordingSentryLogger()
		plainSuper := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              "test",
			Store:                   createTestTriangularStore(),
			Logger:                  plainRecorder,
			TickInterval:            20 * time.Millisecond,
			GracefulShutdownTimeout: 100 * time.Millisecond,
		})

		plainWorker := &countingDeriveWorker{}
		Expect(plainSuper.AddWorker(mockIdentity(), plainWorker)).NotTo(HaveOccurred())
		plainWorker.err = errors.New("boom")

		plainSuper.Start(ctx)
		defer plainSuper.Shutdown()

		Eventually(func() int {
			return sentryErrorCount(plainRecorder, "tick_error")
		}, 5*time.Second).Should(BeNumerically(">=", 1),
			"a plain derive error must still reach Sentry from the root tick loop")
	})
})

var _ = Describe("Supervisor worker restart logging", func() {
	It("warns a config error that breaks a restart re-creation exactly once", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// The factory builds the replacement worker; its derive fails with
		// the same kind of config error a broken spec produces.
		err := factory.RegisterFactoryByType("configerrorrestart",
			func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
				return &countingDeriveWorker{
					err: &config.ConfigError{Field: "read", Err: errors.New("value is not a number")},
				}
			})
		Expect(err).NotTo(HaveOccurred())

		recorder := newRecordingSentryLogger()
		super := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: "configerrorrestart",
			Store:      createTestTriangularStore(),
			Logger:     recorder,
		})

		defer super.Shutdown()

		identity := deps.Identity{
			ID:         "restart-worker",
			Name:       "Restart Worker",
			WorkerType: "configerrorrestart",
		}

		// The old worker asks for a restart; the replacement's derive then
		// fails with the config error.
		removalState := &mockState{signal: fsmv2.SignalNeedsRemoval}
		removalState.nextState = removalState

		Expect(super.AddWorker(identity, &mockWorker{
			observed:     createMockObservedStateWithID(identity.ID),
			initialState: removalState,
		})).NotTo(HaveOccurred())

		super.TestMarkAsStarted()
		super.TestSetPendingRestart(identity.ID)

		tickErr := super.TestTick(ctx)

		// The restart's re-add fails on the config error. The restart itself
		// returns nil: the re-add stays pending and later ticks retry it, so the
		// tick returns nil (the parent worker's own derive succeeded).
		Expect(tickErr).NotTo(HaveOccurred())
		Expect(super.ListWorkers()).To(BeEmpty(),
			"the old worker is removed and the re-add waits for the config to render")

		Expect(recorder.warnCount("config_error")).To(Equal(1),
			"the restart warns the config error exactly once")
		Expect(sentryErrorCount(recorder, "worker_add_derive_desired_failed")).To(Equal(0),
			"the restart's derive failure must not also reach Sentry as a worker_add_derive_desired_failed error")
		Expect(recorder.sentryWarnEvents()).To(BeEmpty())
		Expect(recorder.sentryErrorEvents()).To(BeEmpty(),
			"the restart's config error must not reach Sentry from any site")
	})
})
