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
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	protocolconverterfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter/runtime_config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("ConfigDivergence reset and staging", func() {
	var (
		instance      *protocolconverterfsm.ProtocolConverterInstance
		mockService   *protocolconvertersvc.MockProtocolConverterService
		componentName string
		ctx           context.Context
		tick          uint64
		mockRegistry  *serviceregistry.Registry
		startTime     time.Time
	)

	BeforeEach(func() {
		componentName = "test-pc-config-divergence"
		ctx = context.Background()
		tick = 0
		startTime = time.Now()

		// Desired Active, lifecycle walked to stopped below.
		instance, mockService, _ = fsmtest.SetupProtocolConverterInstance(componentName, protocolconverterfsm.OperationalStateStopped)
		mockRegistry = serviceregistry.NewMockRegistry()
	})

	It("resets ConfigDivergence as the first statement of UpdateObservedStateOfInstance, before the ctx.Err() guard", func() {
		// Phase 1: walk the lifecycle to operational stopped so the
		// to_be_created/creating early-return no longer short-circuits
		// UpdateObservedStateOfInstance before the divergence branch.
		var err error
		tick, err = fsmtest.TestProtocolConverterStateTransition(
			ctx, instance, mockService, mockRegistry, componentName,
			internalfsm.LifecycleStateToBeCreated,
			internalfsm.LifecycleStateCreating, 5, tick, startTime)
		Expect(err).NotTo(HaveOccurred())

		mockService.ExistingComponents[componentName] = true
		fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

		tick, err = fsmtest.TestProtocolConverterStateTransition(
			ctx, instance, mockService, mockRegistry, componentName,
			internalfsm.LifecycleStateCreating,
			protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
		Expect(err).NotTo(HaveOccurred())

		// Desired Active so the "both stopped" early-return does not skip the
		// divergence evaluation.
		Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

		// ServiceExists must report true so the divergence branch (which sets
		// ConfigDivergence inside the ServiceExists check) is entered.
		mockService.ServiceExistsResult = true

		// Build a CONVERGED observed-config baseline that exactly matches what
		// UpdateObservedStateOfInstance will render from the spec, using the
		// same agent location (empty), global vars (nil), node name and pc ID
		// the function uses internally. This is assigned as the LAST step after
		// any TransitionToProtocolConverterState call, which clobbers
		// GetConfigResult via ConfigureProtocolConverterServiceConfig.
		cfg := instance.GetConfig()
		rendered, err := runtime_config.BuildRuntimeConfig(
			cfg,
			map[string]string{},
			nil,
			runtime_config.BridgedByPlaceholder,
			componentName,
		)
		Expect(err).NotTo(HaveOccurred())

		// --- Divergent tick: observed config differs in the read DFC Input ---
		// Derive divergence by replacing the Input map with a fresh, different
		// one. The diff comparator reports this as "Input config differences"
		// (NOT a connection Target mutation, which prints as a pointer
		// address). The injected Input map is deliberately LARGE (50 keys each
		// with a long value) so the raw ConfigDiffRuntime output exceeds 400
		// runes and forces BoundDiff to emit its truncation marker — the raw
		// diff never contains that marker, so asserting it pins the BoundDiff
		// call rather than the raw-diffStr assignment.
		divergent := rendered
		largeInput := make(map[string]any, 50)
		longValue := strings.Repeat("x", 30)
		for i := 0; i < 50; i++ {
			largeInput[fmt.Sprintf("input_key_%02d", i)] = map[string]any{
				"value":   longValue,
				"mapping": `root = {"message":"diverged"}`,
			}
		}
		divergent.DataflowComponentReadServiceConfig.BenthosConfig.Input = largeInput
		mockService.GetConfigResult = divergent

		// Drive one observation tick with a live context. After the GREEN
		// implementation, ConfigDivergence must be non-empty and carry the
		// "Input config differences" text produced by ConfigDiffRuntime, AND
		// the BoundDiff truncation marker (the raw diffStr has no such marker,
		// so this assertion fails if the cell still assigns raw diffStr).
		tick++
		Expect(instance.UpdateObservedStateOfInstance(
			ctx, mockRegistry, fsm.SystemSnapshot{Tick: tick})).To(Succeed())
		Expect(instance.ObservedState.ConfigDivergence).To(ContainSubstring("Input config differences"))
		Expect(instance.ObservedState.ConfigDivergence).To(ContainSubstring("…[truncated,"))

		// --- EDGE 11b-i ORDERING PIN ---
		// The divergent tick above left ConfigDivergence non-empty. Now call
		// UpdateObservedStateOfInstance with an ALREADY-EXPIRED context. The
		// entry reset must have run BEFORE the ctx.Err() guard, so the stale
		// value is cleared and the function returns nil with ConfigDivergence
		// == "". This is the ONLY assertion that goes RED when the reset is
		// placed after the ctx guard; every other ordering passes regardless.
		expiredCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
		defer cancel()
		tick++
		Expect(instance.UpdateObservedStateOfInstance(
			expiredCtx, mockRegistry, fsm.SystemSnapshot{Tick: tick})).To(Succeed())
		Expect(instance.ObservedState.ConfigDivergence).To(Equal(""))

		// --- CONVERGED-CLEARS PATH ---
		// Re-establish a divergent observed config so ConfigDivergence is
		// non-empty again, then drive a converged tick with a LIVE context.
		// The reset at the top of UpdateObservedStateOfInstance must clear
		// the stale divergent value on the normal control flow (where the
		// divergence branch is simply not entered), distinct from the
		// expired-ctx early return above.
		mockService.GetConfigResult = divergent
		tick++
		Expect(instance.UpdateObservedStateOfInstance(
			ctx, mockRegistry, fsm.SystemSnapshot{Tick: tick})).To(Succeed())
		Expect(instance.ObservedState.ConfigDivergence).To(ContainSubstring("Input config differences"))

		mockService.GetConfigResult = rendered
		tick++
		Expect(instance.UpdateObservedStateOfInstance(
			ctx, mockRegistry, fsm.SystemSnapshot{Tick: tick})).To(Succeed())
		Expect(instance.ObservedState.ConfigDivergence).To(Equal(""))
	})
})
