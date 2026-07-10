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

package state_test

// On an alive tick every application state emits the additive union of its own
// declared children, the config-worker kernel (only when the registry is
// configured), and the registry's dynamic children. With no registry the union
// collapses to exactly the own children (no kernel), keeping the change additive
// and the existing suite green. On a stop tick (shutdown OR disabled) it returns
// nil unchanged.
//
// nil → supervisor falls back to legacy ChildrenSpecs path.
// Non-nil empty → supervisor despawns ALL dataflow components.
//
// Do not weaken the stop-tick assertions to BeEmpty(): BeEmpty() passes for both
// nil and non-nil empty and would mask the despawn-everything bug.

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/state"
)

// buildApplicationSnap builds a minimal valid snapshot for application state tests.
func buildApplicationSnap(shutdownRequested bool) fsmv2.Snapshot {
	return fsmv2.Snapshot{
		Observed: fsmv2.Observation[snapshot.ApplicationStatus]{},
		Desired: &fsmv2.WrappedDesiredState[snapshot.ApplicationConfig]{
			BaseDesiredState: config.BaseDesiredState{ShutdownRequested: shutdownRequested},
			Config:           snapshot.ApplicationConfig{},
		},
	}
}

// buildDisabledSnap builds a stop-tick snapshot via the disable path
// (IsDisabled=true, IsShutdownRequested=false). It carries a full alive union so
// a state that re-emits the union instead of force-nilling on the stop path is
// caught.
func buildDisabledSnap() fsmv2.Snapshot {
	own := []config.ChildSpec{{Name: "communicator", WorkerType: "communicator", Enabled: true}}
	dynamic := []config.ChildSpec{{Name: "example", WorkerType: "example", Enabled: true}}
	return fsmv2.Snapshot{
		Observed: fsmv2.Observation[snapshot.ApplicationStatus]{
			Status: snapshot.ApplicationStatus{
				DynamicChildren:    dynamic,
				RegistryConfigured: true,
			},
		},
		Desired: &fsmv2.WrappedDesiredState[snapshot.ApplicationConfig]{
			BaseDesiredState: config.BaseDesiredState{ShutdownRequested: false, Disabled: true},
			Config:           snapshot.ApplicationConfig{},
			ChildrenSpecs:    own,
		},
	}
}

// buildUnionSnap builds an alive-tick snapshot carrying the worker's own
// declared children plus the registry signals that drive the additive union.
func buildUnionSnap(ownChildren, dynamicChildren []config.ChildSpec, registryConfigured bool) fsmv2.Snapshot {
	return fsmv2.Snapshot{
		Observed: fsmv2.Observation[snapshot.ApplicationStatus]{
			Status: snapshot.ApplicationStatus{
				DynamicChildren:    dynamicChildren,
				RegistryConfigured: registryConfigured,
			},
		},
		Desired: &fsmv2.WrappedDesiredState[snapshot.ApplicationConfig]{
			BaseDesiredState: config.BaseDesiredState{ShutdownRequested: false},
			Config:           snapshot.ApplicationConfig{},
			ChildrenSpecs:    ownChildren,
		},
	}
}

// buildInfraIssueSnap builds an alive-tick snapshot whose ChildrenView reports a
// circuit-open child, so HasInfrastructureIssues() is true. This drives
// RunningState through the Running->Degraded transition and DegradedState
// through the stay-degraded return, the two alive paths the plain union builders
// never reach (their ChildrenView is empty, so infrastructure is always healthy).
// It carries a full alive union so those infrastructure-degraded paths assert the
// union output rather than nil.
func buildInfraIssueSnap(ownChildren, dynamicChildren []config.ChildSpec, registryConfigured bool) fsmv2.Snapshot {
	return fsmv2.Snapshot{
		Observed: fsmv2.Observation[snapshot.ApplicationStatus]{
			ChildrenView: config.ChildrenView{
				Children: []config.ChildInfo{{Name: "stuck", IsCircuitOpen: true}},
			},
			Status: snapshot.ApplicationStatus{
				DynamicChildren:    dynamicChildren,
				RegistryConfigured: registryConfigured,
			},
		},
		Desired: &fsmv2.WrappedDesiredState[snapshot.ApplicationConfig]{
			BaseDesiredState: config.BaseDesiredState{ShutdownRequested: false},
			Config:           snapshot.ApplicationConfig{},
			ChildrenSpecs:    ownChildren,
		},
	}
}

var _ = Describe("Application state nil-Children invariant", func() {
	// Each application state must return nil Children (not an empty slice).
	// Violation would despawn ALL dataflow components.

	Describe("RunningState", func() {
		It("returns nil Children on healthy tick", func() {
			result := (&state.RunningState{}).Next(buildApplicationSnap(false))
			Expect(result.Children).To(BeNil())
		})

		It("returns nil Children on shutdown tick", func() {
			result := (&state.RunningState{}).Next(buildApplicationSnap(true))
			Expect(result.Children).To(BeNil())
		})

		It("returns nil Children when a disabled snapshot short-circuits to Stopped", func() {
			// ShouldStop() is true on the disabled path, so RunningState must
			// force-nil rather than re-emit the disabled snapshot's live union.
			result := (&state.RunningState{}).Next(buildDisabledSnap())
			Expect(result.Children).To(BeNil())
		})
	})

	Describe("DegradedState", func() {
		It("returns nil Children on degraded tick", func() {
			result := (&state.DegradedState{}).Next(buildApplicationSnap(false))
			Expect(result.Children).To(BeNil())
		})

		It("returns nil Children on shutdown tick", func() {
			result := (&state.DegradedState{}).Next(buildApplicationSnap(true))
			Expect(result.Children).To(BeNil())
		})

		It("returns nil Children when a disabled snapshot short-circuits to Stopped", func() {
			result := (&state.DegradedState{}).Next(buildDisabledSnap())
			Expect(result.Children).To(BeNil())
		})
	})

	Describe("StoppedState", func() {
		It("returns nil Children on stopped tick", func() {
			result := (&state.StoppedState{}).Next(buildApplicationSnap(false))
			Expect(result.Children).To(BeNil())
		})

		It("returns nil Children and signals removal on shutdown tick", func() {
			result := (&state.StoppedState{}).Next(buildApplicationSnap(true))
			Expect(result.Children).To(BeNil())
			Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
		})

		It("returns nil Children and stays resident on disabled tick", func() {
			// Reached via Running/Degraded ShouldStop() with IsDisabled=true,
			// IsShutdownRequested=false. StoppedState must force-nil the union
			// here, not re-emit a live child set for a disabled supervisor, and
			// must NOT signal removal: a disabled root supervisor stays resident.
			result := (&state.StoppedState{}).Next(buildDisabledSnap())
			Expect(result.Children).To(BeNil())
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		})
	})
})

var _ = Describe("Application state additive-union render", func() {
	// On the alive tick every application state must emit the additive union
	// of its own declared children, the config-worker kernel (only when the
	// registry is configured), and the registry's dynamic children, deduped
	// by Name with every child Enabled. On a shutdown tick it must still
	// return nil.
	type stateNext func(any) fsmv2.NextResult[any, any]

	states := map[string]stateNext{
		"RunningState":  func(s any) fsmv2.NextResult[any, any] { return (&state.RunningState{}).Next(s) },
		"DegradedState": func(s any) fsmv2.NextResult[any, any] { return (&state.DegradedState{}).Next(s) },
		"StoppedState":  func(s any) fsmv2.NextResult[any, any] { return (&state.StoppedState{}).Next(s) },
	}

	It("emits the kernel-inclusive union when configured, exactly own children when not, and nil on shutdown", func() {
		own := []config.ChildSpec{{Name: "communicator", WorkerType: "communicator", Enabled: true}}
		dynamic := []config.ChildSpec{{Name: "example", WorkerType: "example", Enabled: false}}

		names := func(specs []config.ChildSpec) []string {
			out := make([]string, 0, len(specs))
			for _, s := range specs {
				out = append(out, s.Name)
			}
			return out
		}

		for name, next := range states {
			// (a) configured: own + kernel + dynamic, deduped, all Enabled.
			children := next(buildUnionSnap(own, dynamic, true)).Children
			Expect(children).NotTo(BeNil(), "%s alive-configured union must not be nil", name)
			Expect(names(children)).To(ConsistOf("communicator", "config-worker", "example"),
				"%s alive-configured union must dedupe own ∪ kernel ∪ dynamic by Name", name)
			for _, c := range children {
				Expect(c.Enabled).To(BeTrue(), "%s union child %q must be Enabled", name, c.Name)
			}

			// (b) no registry: exactly own children, no kernel (additive no-op).
			bareChildren := next(buildUnionSnap(own, nil, false)).Children
			Expect(names(bareChildren)).To(ConsistOf("communicator"),
				"%s bare union must equal exactly own children (no kernel)", name)

			// shutdown tick: still nil for every state.
			shutdown := next(buildApplicationSnap(true))
			Expect(shutdown.Children).To(BeNil(), "%s must still return nil Children on shutdown", name)
		}
	})

	It("configured-but-no-dynamic emits exactly own + kernel", func() {
		// The realistic state right after the registry is published but before
		// any dynamic child is recorded: own children plus the forced kernel.
		own := []config.ChildSpec{{Name: "communicator", WorkerType: "communicator", Enabled: true}}

		names := func(specs []config.ChildSpec) []string {
			out := make([]string, 0, len(specs))
			for _, s := range specs {
				out = append(out, s.Name)
			}
			return out
		}

		for name, next := range states {
			children := next(buildUnionSnap(own, nil, true)).Children
			Expect(children).NotTo(BeNil(), "%s configured-no-dynamic union must not be nil", name)
			Expect(names(children)).To(ConsistOf("communicator", "config-worker"),
				"%s configured-no-dynamic union must be own + kernel", name)
		}
	})

	It("drops a dynamic child that collides with the kernel name (P7 no-hijack)", func() {
		// A dynamic registry entry that reuses the kernel's Name must never shadow
		// the real kernel: dedup keeps the FIRST occurrence (kernel), so the
		// config_worker that owns the shared registry cannot be hijacked or
		// removed via an Upsert under its own name.
		own := []config.ChildSpec{{Name: "communicator", WorkerType: "communicator", Enabled: true}}
		impostor := []config.ChildSpec{{Name: "config-worker", WorkerType: "impostor", Enabled: true}}

		for name, next := range states {
			children := next(buildUnionSnap(own, impostor, true)).Children

			kernelCount := 0
			for _, c := range children {
				if c.Name == "config-worker" {
					kernelCount++
					Expect(c.WorkerType).To(Equal("configworker"),
						"%s kernel child must keep its real worker type, not the impostor's", name)
				}
			}
			Expect(kernelCount).To(Equal(1),
				"%s must emit exactly one config-worker (the kernel), dropping the colliding dynamic child", name)
		}
	})

	It("emits nil for the empty union (own=nil, dynamic=nil, unconfigured)", func() {
		// With no own children, no dynamic children, and no registry the union is
		// empty and MUST collapse to nil — never a non-nil empty slice, which
		// would order the supervisor to despawn ALL children. BeNil() fails for a
		// non-nil empty slice, so it pins the collapse-to-nil guard directly.
		for name, next := range states {
			children := next(buildUnionSnap(nil, nil, false)).Children
			Expect(children).To(BeNil(), "%s empty union must be nil, not a non-nil empty slice", name)
		}
	})

	It("emits the union on the Running->Degraded transition and the stay-degraded return", func() {
		// The plain union builders leave ChildrenView empty, so RunningState
		// always hits its catch-all and DegradedState its recovery return — the
		// Running->Degraded transition and stay-degraded paths are never reached.
		// buildInfraIssueSnap injects a circuit-open child so HasInfrastructureIssues()
		// is true, driving both: each must emit the full union, not nil. The child
		// set must not change merely because the parent crosses Running<->Degraded.
		own := []config.ChildSpec{{Name: "communicator", WorkerType: "communicator", Enabled: true}}
		dynamic := []config.ChildSpec{{Name: "example", WorkerType: "example", Enabled: false}}

		names := func(specs []config.ChildSpec) []string {
			out := make([]string, 0, len(specs))
			for _, s := range specs {
				out = append(out, s.Name)
			}
			return out
		}

		snap := buildInfraIssueSnap(own, dynamic, true)

		// RunningState detects the infrastructure issue and transitions to
		// Degraded; it must carry the union, not despawn every child.
		running := (&state.RunningState{}).Next(snap)
		Expect(running.Children).NotTo(BeNil(), "Running->Degraded transition must emit the union")
		Expect(names(running.Children)).To(ConsistOf("communicator", "config-worker", "example"),
			"Running->Degraded transition must emit own ∪ kernel ∪ dynamic")

		// DegradedState sees the issue persist and stays degraded; it must keep
		// rendering the union.
		degraded := (&state.DegradedState{}).Next(snap)
		Expect(degraded.Children).NotTo(BeNil(), "stay-degraded return must emit the union")
		Expect(names(degraded.Children)).To(ConsistOf("communicator", "config-worker", "example"),
			"stay-degraded return must emit own ∪ kernel ∪ dynamic")
	})

	It("kernel wins a Name collision with a dynamic child", func() {
		// A dynamic registry entry reusing the reserved kernel Name must not
		// shadow the kernel: the union keeps exactly one 'config-worker' and it
		// is the kernel's WorkerType, pinning kernel > registry precedence.
		dynamic := []config.ChildSpec{{Name: "config-worker", WorkerType: "impostor", Enabled: false}}

		for name, next := range states {
			children := next(buildUnionSnap(nil, dynamic, true)).Children
			count := 0
			var survivor config.ChildSpec
			for _, c := range children {
				if c.Name == "config-worker" {
					count++
					survivor = c
				}
			}
			Expect(count).To(Equal(1), "%s must dedupe the kernel-name collision to one entry", name)
			Expect(survivor.WorkerType).To(Equal("configworker"),
				"%s kernel must win the collision (kernel > registry)", name)
			Expect(survivor.Enabled).To(BeTrue(), "%s surviving kernel must be Enabled", name)
		}
	})

	It("kernel and dynamic win a Name collision with an own child", func() {
		// An own child reusing a reserved name must not shadow the kernel or a
		// dynamic registry child, pinning kernel > registry > own precedence.
		own := []config.ChildSpec{
			{Name: "config-worker", WorkerType: "own-impostor", Enabled: true},
			{Name: "shared", WorkerType: "own-shared", Enabled: true},
		}
		dynamic := []config.ChildSpec{{Name: "shared", WorkerType: "registry-shared", Enabled: false}}

		for name, next := range states {
			children := next(buildUnionSnap(own, dynamic, true)).Children

			byName := map[string]config.ChildSpec{}
			counts := map[string]int{}
			for _, c := range children {
				counts[c.Name]++
				byName[c.Name] = c
			}
			Expect(counts["config-worker"]).To(Equal(1), "%s must keep one config-worker", name)
			Expect(byName["config-worker"].WorkerType).To(Equal("configworker"),
				"%s kernel must beat a same-named own child", name)
			Expect(counts["shared"]).To(Equal(1), "%s must keep one 'shared'", name)
			Expect(byName["shared"].WorkerType).To(Equal("registry-shared"),
				"%s registry child must beat a same-named own child", name)
		}
	})
})
