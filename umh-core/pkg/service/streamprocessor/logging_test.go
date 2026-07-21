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

package streamprocessor

import (
	"context"
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

var _ = Describe("demotes per-tick config dumps to DEBUG", func() {
	It("emits only the two lifecycle INFO lines per real create and nothing on retries/dumps", func() {
		ctx := context.Background()
		const spName = "test-sp"

		// White-box: capture every log the service emits via a zap observer core
		// and drive the real (unmodified) service through its manager methods.
		core, logs := observer.New(zapcore.DebugLevel)
		svc := NewDefaultService(spName)
		svc.logger = zap.New(core).Sugar()

		// FileSystemService is not dereferenced by Add/Update/Remove; nil is safe.
		// A zero-value config is sufficient — the methods only append/reassign it.
		var cfg dataflowcomponentserviceconfig.DataflowComponentServiceConfig

		Expect(svc.AddToManager(ctx, nil, &cfg, spName)).To(Succeed())
		infoAfterFirst := logs.FilterLevelExact(zapcore.InfoLevel)
		Expect(infoAfterFirst.Len()).To(Equal(2),
			"first AddToManager should log exactly 2 INFO lines (Adding + added to manager); "+
				"the per-tick config dumps must be DEBUG")
		Expect(messages(infoAfterFirst.All())).To(ConsistOf(
			"Adding StreamProcessor "+spName,
			"Stream Processor "+spName+" added to manager",
		))

		// Idempotent AddToManager retry logs zero INFO lines.
		err := svc.AddToManager(ctx, nil, &cfg, spName)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, ErrServiceAlreadyExists)).To(BeTrue())
		Expect(logs.FilterLevelExact(zapcore.InfoLevel).Len()).To(Equal(2),
			"second AddToManager must log ZERO additional INFO lines "+
				"(Adding moved below the exists-check so idempotent retries are silent)")

		// UpdateInManager emits none of the demoted lines at INFO, and emits them
		// at DEBUG.
		infoBeforeUpdate := logs.FilterLevelExact(zapcore.InfoLevel).Len()
		debugBeforeUpdate := logs.FilterLevelExact(zapcore.DebugLevel).Len()
		Expect(svc.UpdateInManager(ctx, nil, &cfg, spName)).To(Succeed())
		Expect(logs.FilterLevelExact(zapcore.InfoLevel).Len()).To(Equal(infoBeforeUpdate),
			"UpdateInManager must emit ZERO INFO lines (all lifecycle lines demoted to DEBUG)")
		newDebugMessages := messages(logs.FilterLevelExact(zapcore.DebugLevel).All()[debugBeforeUpdate:])
		Expect(newDebugMessages).To(ContainElement(ContainSubstring("Updating streamprocessor")),
			"'Updating streamprocessor' must reappear at DEBUG")
		Expect(newDebugMessages).To(ContainElement(ContainSubstring("Updated streamprocessor config in manager")),
			"'Updated streamprocessor config in manager' must reappear at DEBUG")
		Expect(newDebugMessages).To(ContainElement(ContainSubstring("Dataflow component config:")),
			"the 'Dataflow component config: %+v' dump must reappear at DEBUG")

		// RemoveFromManager retries emit zero INFO lines and emit the demoted
		// 'Removing dataflowcomponent' line at DEBUG. AddToManager above only
		// appends to the config slice and never creates a live FSM instance in
		// the manager, so GetInstance returns ok=false and RemoveFromManager
		// returns nil on each retry (config-slice path only). In production the
		// reconcile loop seeds a live FSM, so the first RemoveFromManager returns
		// standarderrors.ErrRemovalPending until the FSM is reaped; that
		// real-lifecycle path is not exercised here.
		infoBeforeRemove := logs.FilterLevelExact(zapcore.InfoLevel).Len()
		debugBeforeRemove := logs.FilterLevelExact(zapcore.DebugLevel).Len()
		for i := 0; i < 5; i++ {
			Expect(svc.RemoveFromManager(ctx, nil, spName)).To(Succeed(),
				"RemoveFromManager returns nil when no live FSM instance exists "+
					"(config-slice path only); production removal returns "+
					"ErrRemovalPending until the FSM is reaped")
		}
		Expect(logs.FilterLevelExact(zapcore.InfoLevel).Len()).To(Equal(infoBeforeRemove),
			"RemoveFromManager retries must emit ZERO INFO lines "+
				"('Removing dataflowcomponent' demoted to DEBUG)")
		removeDebugMessages := messages(logs.FilterLevelExact(zapcore.DebugLevel).All()[debugBeforeRemove:])
		var removeDebugCount int
		for _, m := range removeDebugMessages {
			if strings.Contains(m, "Removing dataflowcomponent") {
				removeDebugCount++
			}
		}
		Expect(removeDebugCount).To(Equal(5),
			"'Removing dataflowcomponent' must fire exactly once per retry (5 calls) at DEBUG")
	})
})

func messages(entries []observer.LoggedEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Message)
	}
	return out
}
