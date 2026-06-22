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

package protocolconverter

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

var _ = Describe("demotes per-tick config dumps to DEBUG", func() {
	It("emits only the two lifecycle INFO lines per real create and nothing on retries/dumps", func() {
		ctx := context.Background()
		const protConvName = "test-pc"

		// White-box: capture every log the service emits via a zap observer core
		// and drive the real (unmodified) service through its manager methods.
		core, logs := observer.New(zapcore.DebugLevel)
		svc := NewDefaultProtocolConverterService(protConvName)
		svc.logger = zap.New(core).Sugar()

		// filesystemService is not dereferenced by Add/Update/Remove; nil is safe.
		// A zero-value runtime config is sufficient — the methods only append/reassign it.
		var cfg protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime

		Expect(svc.AddToManager(ctx, nil, &cfg, protConvName, false)).To(Succeed())
		infoAfterFirst := logs.FilterLevelExact(zapcore.InfoLevel)
		Expect(infoAfterFirst.Len()).To(Equal(2),
			"first AddToManager should log exactly 2 INFO lines (Adding + added to manager); "+
				"the per-tick config dumps must be DEBUG")
		Expect(messages(infoAfterFirst.All())).To(ConsistOf(
			"Adding ProtocolConverter "+protConvName,
			"ProtocolConverter "+protConvName+" added to manager",
		))

		// Idempotent AddToManager retry logs zero INFO lines.
		err := svc.AddToManager(ctx, nil, &cfg, protConvName, false)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, ErrServiceAlreadyExists)).To(BeTrue())
		Expect(logs.FilterLevelExact(zapcore.InfoLevel).Len()).To(Equal(2),
			"second AddToManager must log ZERO additional INFO lines "+
				"(Adding moved below the exists-check so idempotent retries are silent)")

		// UpdateInManager emits none of the demoted lines at INFO, and emits them
		// at DEBUG.
		infoBeforeUpdate := logs.FilterLevelExact(zapcore.InfoLevel).Len()
		debugBeforeUpdate := logs.FilterLevelExact(zapcore.DebugLevel).Len()
		Expect(svc.UpdateInManager(ctx, nil, &cfg, protConvName, false)).To(Succeed())
		Expect(logs.FilterLevelExact(zapcore.InfoLevel).Len()).To(Equal(infoBeforeUpdate),
			"UpdateInManager must emit ZERO INFO lines (all lifecycle lines demoted to DEBUG)")
		for _, e := range logs.FilterLevelExact(zapcore.InfoLevel).All() {
			Expect(e.Message).NotTo(ContainSubstring("Updating protocolconverter"),
				"'Updating protocolconverter' must be demoted to DEBUG, not INFO")
			Expect(e.Message).NotTo(ContainSubstring("Updated protocolconverter config in manager"),
				"'Updated protocolconverter config in manager' must be demoted to DEBUG, not INFO")
			Expect(e.Message).NotTo(ContainSubstring("Connection config:"),
				"the 'Connection config: %+v' dump must be demoted to DEBUG, not INFO")
			Expect(e.Message).NotTo(ContainSubstring("Dataflow component config:"),
				"the 'Dataflow component config: %+v' dump must be demoted to DEBUG, not INFO")
		}
		newDebugMessages := messages(logs.FilterLevelExact(zapcore.DebugLevel).All()[debugBeforeUpdate:])
		Expect(newDebugMessages).To(ContainElement(ContainSubstring("Updating protocolconverter")),
			"'Updating protocolconverter' must reappear at DEBUG")
		Expect(newDebugMessages).To(ContainElement(ContainSubstring("Updated protocolconverter config in manager")),
			"'Updated protocolconverter config in manager' must reappear at DEBUG")
		Expect(newDebugMessages).To(ContainElement(ContainSubstring("Connection config:")),
			"the 'Connection config: %+v' dump must reappear at DEBUG")
		Expect(newDebugMessages).To(ContainElement(ContainSubstring("Dataflow component config:")),
			"the 'Dataflow component config: %+v' dump must reappear at DEBUG")

		// RemoveFromManager retries emit zero INFO lines and emit the demoted
		// 'Removing dataflowcomponent' line at DEBUG. The fresh child managers hold
		// no FSM instances, so each retry slices the already-removed config
		// (idempotent) and returns nil.
		infoBeforeRemove := logs.FilterLevelExact(zapcore.InfoLevel).Len()
		debugBeforeRemove := logs.FilterLevelExact(zapcore.DebugLevel).Len()
		for i := 0; i < 5; i++ {
			Expect(svc.RemoveFromManager(ctx, nil, protConvName)).To(Succeed(),
				"RemoveFromManager must be idempotent and return nil on each retry")
		}
		Expect(logs.FilterLevelExact(zapcore.InfoLevel).Len()).To(Equal(infoBeforeRemove),
			"RemoveFromManager retries must emit ZERO INFO lines "+
				"('Removing dataflowcomponent' demoted to DEBUG)")
		removeDebugMessages := messages(logs.FilterLevelExact(zapcore.DebugLevel).All()[debugBeforeRemove:])
		Expect(removeDebugMessages).To(ContainElement(ContainSubstring("Removing dataflowcomponent")),
			"'Removing dataflowcomponent' must reappear at DEBUG across retries")
	})
})

func messages(entries []observer.LoggedEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Message)
	}
	return out
}
