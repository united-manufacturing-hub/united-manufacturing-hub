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
	"errors"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	protocolconverterfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter/runtime_config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
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

var _ = Describe("ConfigDivergence composition into StatusReason", func() {
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
		componentName = "test-pc-composition"
		ctx = context.Background()
		tick = 0
		startTime = time.Now()

		instance, mockService, _ = fsmtest.SetupProtocolConverterInstance(componentName, protocolconverterfsm.OperationalStateStopped)
		mockRegistry = serviceregistry.NewMockRegistry()
	})

	// snapshotFor builds a SystemSnapshot matching the shape the reconcile loop
	// uses in production (tick-pinned time, agent location carried for the
	// observed-state render).
	snapshotFor := func(t uint64) fsm.SystemSnapshot {
		return fsm.SystemSnapshot{
			Tick:         t,
			SnapshotTime: startTime.Add(time.Duration(t) * constants.DefaultTickerTime),
			CurrentConfig: config.FullConfig{
				Agent: config.AgentConfig{
					Location: map[int]string{},
				},
			},
		}
	}

	It("composes the divergence into StatusReason on a diverged tick and bounds it across removal ticks (edges 5 and 14)", func() {
		var err error
		// Walk the lifecycle to operational stopped so the
		// to_be_created/creating early-return no longer short-circuits
		// UpdateObservedStateOfInstance before the divergence branch.
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

		mockService.ServiceExistsResult = true

		// Converged baseline: render the config exactly as
		// UpdateObservedStateOfInstance will render it from the spec, and assign
		// it to GetConfigResult as the LAST setup statement (after any
		// TransitionToProtocolConverterState call that clobbers it via
		// ConfigureProtocolConverterServiceConfig).
		cfg := instance.GetConfig()
		rendered, err := runtime_config.BuildRuntimeConfig(
			cfg,
			map[string]string{},
			nil,
			runtime_config.BridgedByPlaceholder,
			componentName,
		)
		Expect(err).NotTo(HaveOccurred())
		mockService.GetConfigResult = rendered

		// --- Divergent config: large Input map (>400 runes) so BoundDiff
		// emits its truncation marker; the diff comparator reports this as
		// "Input config differences".
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

		// --- EDGE 5: one full Reconcile() tick on an active-diverging instance.
		// Composition must append "re-applying config: " to StatusReason.
		tick++
		err, _ = instance.Reconcile(ctx, snapshotFor(tick), mockRegistry)
		Expect(err).NotTo(HaveOccurred())
		Expect(instance.ObservedState.ServiceInfo.StatusReason).To(ContainSubstring("re-applying config: "))
		Expect(instance.ObservedState.ConfigDivergence).To(ContainSubstring("Input config differences"))
		Expect(instance.ObservedState.ServiceInfo.StatusReason).To(ContainSubstring(instance.ObservedState.ConfigDivergence))

		// --- EDGE 5 APPEND-SEMANTICS PIN ---
		// ContainSubstring passes under overwrite too, so it cannot
		// discriminate append from overwrite. When Step 3 wrote a non-empty
		// operational reason on this tick, the divergence text must come AFTER
		// it with "; " as the join. Assert the divergence text is a SUFFIX
		// preceded by "; " — this goes RED if composition overwrites
		// StatusReason instead of appending.
		statusReason := instance.ObservedState.ServiceInfo.StatusReason
		if idx := strings.Index(statusReason, "re-applying config:"); idx > 0 {
			Expect(statusReason[:idx]).To(HaveSuffix("; "),
				"operational reason must be preserved in front of the divergence text, joined with \"; \"")
		}

		// --- EDGE 14: request removal, then loop 30 ticks with a pending removal.
		Expect(instance.Remove(ctx)).To(Succeed())

		// Put the mock into stopped flags so IsProtocolConverterStopped()
		// returns true during the stopping state, letting the FSM converge
		// stopped -> removing. TransitionToProtocolConverterState also resets
		// GetConfigResult, so re-inject the divergent config afterwards so
		// ConfigDivergence stays non-empty on every operational tick and is
		// non-empty when the FSM enters the removing branch.
		fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)
		mockService.GetConfigResult = divergent
		// Pending removal: RemoveFromManager must hold the FSM in removing
		// across many ticks. This requires the mock to consult
		// RemoveFromManagerError BEFORE mutating config; the current code
		// mutates then returns the error, completing removal after one tick.
		mockService.RemoveFromManagerError = standarderrors.ErrRemovalPending

		sawRemoving := false
		removingTickCount := 0
		divergenceDuringRemoving := false

		for i := 0; i < 30; i++ {
			wasRemoving := instance.IsRemoving()
			if wasRemoving {
				sawRemoving = true
				removingTickCount++
			}

			tick++
			rErr, _ := instance.Reconcile(ctx, snapshotFor(tick), mockRegistry)
			// Once removal completes Reconcile returns ErrInstanceRemoved;
			// the loop keeps ticking to observe the removal-window invariant.
			_ = rErr

			if wasRemoving && instance.ObservedState.ConfigDivergence != "" {
				divergenceDuringRemoving = true
			}
		}

		// Mock fix: removal stays pending across many ticks.
		Expect(sawRemoving).To(BeTrue(), "expected the FSM to enter the removing state")
		Expect(removingTickCount).To(BeNumerically(">=", 10), "expected at least 10 removing ticks while removal is pending")
		// The IsRemoving branch clears ConfigDivergence per tick so the
		// composition block does not stamp "re-applying config" on an
		// instance being deleted.
		Expect(divergenceDuringRemoving).To(BeFalse(), "ConfigDivergence must be cleared during removal ticks")
	})

	It("composes the divergence into StatusReason even when reconcileStateTransition errors on a diverged tick (edge 15)", func() {
		var err error
		// Walk the lifecycle to operational stopped, same setup as edge 5/14.
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

		// Desired Active so the FSM will try to start the instance.
		Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

		mockService.ServiceExistsResult = true

		// Converged baseline, then make it divergent with a large Input map.
		cfg := instance.GetConfig()
		rendered, err := runtime_config.BuildRuntimeConfig(
			cfg,
			map[string]string{},
			nil,
			runtime_config.BridgedByPlaceholder,
			componentName,
		)
		Expect(err).NotTo(HaveOccurred())

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

		// snapshotFor is scoped to the edge-15 It because the Describe-level
		// helper lives inside the first It's closure; rebuild it here.
		snapshotFor := func(t uint64) fsm.SystemSnapshot {
			return fsm.SystemSnapshot{
				Tick:         t,
				SnapshotTime: startTime.Add(time.Duration(t) * constants.DefaultTickerTime),
				CurrentConfig: config.FullConfig{
					Agent: config.AgentConfig{
						Location: map[int]string{},
					},
				},
			}
		}

		// Tick 1: stopped with desired Active. This tick fires EventStart only
		// (no service call yet), so no error is injected — it just advances the
		// FSM into starting_connection.
		tick++
		rErr, _ := instance.Reconcile(ctx, snapshotFor(tick), mockRegistry)
		Expect(rErr).NotTo(HaveOccurred())

		// Tick 2: FSM is now in starting_connection. reconcileStateTransition
		// calls StartConnectionInstance, which hits StartConnectionError and
		// returns an error → Reconcile enters the `if err != nil` block and
		// returns. The composition block MUST have run BEFORE that return, or
		// the divergence diagnostic is lost on this overload tick.
		mockService.StartConnectionError = errors.New("injected start failure")
		tick++
		err2, _ := instance.Reconcile(ctx, snapshotFor(tick), mockRegistry)
		_ = err2

		// Prove the error path was actually hit — without this the test is
		// vacuous (a tick that never called StartConnection cannot discriminate
		// before-err vs after-err placement).
		Expect(mockService.StartConnectionCalled).To(BeTrue(),
			"StartConnection must be called so the error path is actually exercised")

		// The divergence must have been staged by
		// UpdateObservedStateOfInstance on this tick.
		Expect(instance.ObservedState.ConfigDivergence).NotTo(BeEmpty(),
			"ConfigDivergence must be staged on the diverged tick")

		// The load-bearing assertion: composition ran BEFORE the err-block
		// return, so StatusReason carries the divergence text. This goes RED
		// if the composition block is placed AFTER the err block.
		Expect(instance.ObservedState.ServiceInfo.StatusReason).To(ContainSubstring("re-applying config: "),
			"composition must run before the err-block return so the divergence surfaces on error ticks")

		// Reset the injected error so downstream specs are not affected.
		mockService.StartConnectionError = nil
	})
})
