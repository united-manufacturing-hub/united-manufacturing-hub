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

package fsm_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// fix4cTestConfig is a minimal config type used to drive the BaseFSMManager
// in fix-4c regression tests.
type fix4cTestConfig struct {
	Name         string
	DesiredState string
}

// fakeFSMInstance is a minimal FSMInstance used in fix-4c tests. It tracks
// SetDesiredFSMState calls so tests can assert Step 2 ran for non-Removed
// instances and was skipped for Removed ones.
type fakeFSMInstance struct {
	name            string
	currentState    string
	desiredState    string
	setDesiredCalls int
	reconcileCalls  int
	// flapDesired, when set, causes SetDesiredFSMState to revert to flapDesired
	// after each call. Simulates a parent FSM that keeps overwriting the
	// desired state every ~100ms (the ENG-4862 wedge scenario).
	flapDesired string
}

func (f *fakeFSMInstance) GetCurrentFSMState() string { return f.currentState }
func (f *fakeFSMInstance) GetDesiredFSMState() string { return f.desiredState }
func (f *fakeFSMInstance) SetDesiredFSMState(s string) error {
	f.setDesiredCalls++
	f.desiredState = s
	if f.flapDesired != "" {
		f.desiredState = f.flapDesired
	}
	return nil
}
func (f *fakeFSMInstance) IsTransientStreakCounterMaxed() bool { return false }
func (f *fakeFSMInstance) Reconcile(ctx context.Context, _ publicfsm.SystemSnapshot, _ serviceregistry.Provider) (error, bool) {
	f.reconcileCalls++
	return nil, false
}
func (f *fakeFSMInstance) Remove(_ context.Context) error                { return nil }
func (f *fakeFSMInstance) GetLastObservedState() publicfsm.ObservedState { return nil }
func (f *fakeFSMInstance) GetMinimumRequiredTime() time.Duration         { return 0 }

// FSMInstanceActions
func (f *fakeFSMInstance) CreateInstance(_ context.Context, _ filesystem.Service) error { return nil }
func (f *fakeFSMInstance) RemoveInstance(_ context.Context, _ filesystem.Service) error { return nil }
func (f *fakeFSMInstance) StartInstance(_ context.Context, _ filesystem.Service) error  { return nil }
func (f *fakeFSMInstance) StopInstance(_ context.Context, _ filesystem.Service) error   { return nil }
func (f *fakeFSMInstance) UpdateObservedStateOfInstance(_ context.Context, _ serviceregistry.Provider, _ publicfsm.SystemSnapshot) error {
	return nil
}
func (f *fakeFSMInstance) CheckForCreation(_ context.Context, _ filesystem.Service) bool {
	return false
}

func newFix4cManager(initial map[string]*fakeFSMInstance, configs []fix4cTestConfig) *publicfsm.BaseFSMManager[fix4cTestConfig] {
	mgr := publicfsm.NewBaseFSMManager[fix4cTestConfig](
		"fix4c-test",
		"/dev/null",
		func(_ config.FullConfig) ([]fix4cTestConfig, error) {
			return configs, nil
		},
		func(c fix4cTestConfig) (string, error) {
			return c.Name, nil
		},
		func(c fix4cTestConfig) (string, error) {
			return c.DesiredState, nil
		},
		func(c fix4cTestConfig) (publicfsm.FSMInstance, error) {
			return &fakeFSMInstance{name: c.Name, currentState: "to_be_created", desiredState: c.DesiredState}, nil
		},
		func(_ publicfsm.FSMInstance, _ fix4cTestConfig) (bool, error) {
			return true, nil
		},
		func(_ publicfsm.FSMInstance, _ fix4cTestConfig) error {
			return nil
		},
		func(_ publicfsm.FSMInstance) (time.Duration, error) {
			return 0, nil
		},
	)
	for name, inst := range initial {
		mgr.AddInstanceForTest(name, inst)
	}
	return mgr
}

func runFix4cReconcile(mgr *publicfsm.BaseFSMManager[fix4cTestConfig]) (error, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
}

var _ = Describe("BaseFSMManager fix 4c — manager-recreate guard (ENG-4862)", func() {
	Context("when an instance is in LifecycleStateRemoved", func() {
		It("cleans it up via Step 3 even with continuous desired-state flapping", func() {
			inst := &fakeFSMInstance{
				name:         "bridge-1",
				currentState: internalfsm.LifecycleStateRemoved,
				desiredState: "active",
				flapDesired:  "active", // parent FSM keeps reverting desired state — without the guard, Step 2 wins every tick
			}
			mgr := newFix4cManager(
				map[string]*fakeFSMInstance{"bridge-1": inst},
				[]fix4cTestConfig{{Name: "bridge-1", DesiredState: "stopped"}},
			)

			Eventually(func() int {
				_, _ = runFix4cReconcile(mgr)
				return len(mgr.GetInstances())
			}, "2s", "10ms").Should(Equal(0), "Removed instance should be deleted by Step 3 even with continuous desired-state flapping")

			Expect(inst.setDesiredCalls).To(Equal(0), "Step 2 must NOT call SetDesiredFSMState on a Removed instance")
		})
	})

	Context("when an instance is NOT in LifecycleStateRemoved", func() {
		It("still applies desired-state changes via Step 2", func() {
			inst := &fakeFSMInstance{
				name:         "bridge-2",
				currentState: "running",
				desiredState: "active",
			}
			mgr := newFix4cManager(
				map[string]*fakeFSMInstance{"bridge-2": inst},
				[]fix4cTestConfig{{Name: "bridge-2", DesiredState: "stopped"}},
			)

			_, _ = runFix4cReconcile(mgr)

			Expect(inst.setDesiredCalls).To(BeNumerically(">=", 1), "Step 2 must call SetDesiredFSMState on a non-Removed instance with mismatched desired state")
			Expect(inst.desiredState).To(Equal("stopped"))
			Expect(mgr.GetInstances()).To(HaveKey("bridge-2"), "non-Removed instance must NOT be cleaned up")
		})
	})

	Context("end-to-end recreate after ForceRemove", func() {
		It("removes the wedged instance and creates a fresh one from config on the next tick", func() {
			// Reproduce the ENG-4862 wedge: instance is Removed but parent FSM keeps
			// flapping desired state, causing Step 2 to win over Step 3 without the guard.
			wedged := &fakeFSMInstance{
				name:         "bridge-3",
				currentState: internalfsm.LifecycleStateRemoved,
				desiredState: "active",
				flapDesired:  "active", // parent FSM keeps overwriting desired back to active
			}
			mgr := newFix4cManager(
				map[string]*fakeFSMInstance{"bridge-3": wedged},
				[]fix4cTestConfig{{Name: "bridge-3", DesiredState: "stopped"}},
			)

			Eventually(func() bool {
				_, _ = runFix4cReconcile(mgr)
				_, hasWedged := mgr.GetInstances()["bridge-3"]
				return !hasWedged || mgr.GetInstances()["bridge-3"] != publicfsm.FSMInstance(wedged)
			}, "2s", "10ms").Should(BeTrue(), "wedged Removed instance must be cleaned out")

			Eventually(func() string {
				_, _ = runFix4cReconcile(mgr)
				inst, ok := mgr.GetInstances()["bridge-3"]
				if !ok {
					return ""
				}
				return inst.GetCurrentFSMState()
			}, "2s", "10ms").Should(Equal("to_be_created"), "manager must recreate the instance from config after deletion")

			Expect(wedged.setDesiredCalls).To(Equal(0), "Step 2 must NOT call SetDesiredFSMState on the wedged Removed instance — that's what wedges it")
		})
	})
})
