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

//go:build test

package protocolconverter_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	protocolconverterfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("a bridge that waits", func() {
	It("logs the wait at Warn once per throttle window, not on every tick", func() {
		// Run the logger's one-time setup first, so it does not replace the
		// observer below. The bridge's own logger and the throttle logger then
		// both write to the observer.
		logger.GetLogger()

		core, logs := observer.New(zapcore.DebugLevel)
		DeferCleanup(zap.ReplaceGlobals(zap.New(core)))
		logger.ResetThrottleLoggerForTest(zap.New(core).Sugar())
		DeferCleanup(func() { logger.ResetThrottleLoggerForTest(nil) })

		const name = "test-pc-waits"

		instance, mockService, _ := fsmtest.SetupProtocolConverterInstance(name, protocolconverterfsm.OperationalStateActive)
		Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())
		mockService.BridgeMustWaitReason = "Cannot create bridge - limit exceeded"
		registry := serviceregistry.NewMockRegistry()
		start := time.Now()

		for tick := range uint64(50) {
			snapshot := fsm.SystemSnapshot{
				Tick:         tick,
				SnapshotTime: start.Add(time.Duration(tick) * constants.DefaultTickerTime),
				CurrentConfig: config.FullConfig{
					Agent: config.AgentConfig{Location: map[int]string{0: "test-location"}},
				},
			}
			err, _ := instance.Reconcile(context.Background(), snapshot, registry)
			Expect(err).NotTo(HaveOccurred())
		}

		Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateToBeCreated))

		waits := logs.FilterMessageSnippet("Bridge " + name + " waits").FilterLevelExact(zapcore.WarnLevel)
		// The throttle logs the first wait, then once more to say it suppresses
		// further occurrences for its window.
		Expect(waits.Len()).To(Equal(2))
	})
})
