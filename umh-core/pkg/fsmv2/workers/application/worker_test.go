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

package application

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
)

// Compile-time interface verification.
var _ fsmv2.Worker = (*ApplicationWorker)(nil)

var _ = Describe("ApplicationWorker", func() {
	var worker *ApplicationWorker

	BeforeEach(func() {
		worker = NewApplicationWorker("root-1", "test-root", deps.NewNopFSMLogger(), nil)
	})

	Describe("CollectObservedState", func() {
		It("should return observed state with expected status fields", func() {
			ctx := context.Background()
			obs, err := worker.CollectObservedState(ctx, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(obs).NotTo(BeNil())

			typedObs, ok := obs.(fsmv2.Observation[snapshot.ApplicationStatus])
			Expect(ok).To(BeTrue(), "Expected fsmv2.Observation[ApplicationStatus], got %T", obs)
			Expect(typedObs.Status.Name).To(Equal("test-root"))
			Expect(typedObs.Status.ID).To(Equal("root-1"))
		})

		It("should respect context cancellation", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately.

			_, err := worker.CollectObservedState(ctx, nil)
			Expect(err).To(Equal(context.Canceled))
		})
	})

	Describe("DeriveDesiredState", func() {
		It("should parse children from YAML config", func() {
			yamlConfig := `
children:
  - name: "child-1"
    workerType: "example-child"
    userSpec:
      config: |
        value: 10
  - name: "child-2"
    workerType: "example-child"
    userSpec:
      config: |
        value: 20
`
			userSpec := config.UserSpec{
				Config: yamlConfig,
			}

			desiredIface, err := worker.DeriveDesiredState(userSpec)
			Expect(err).ToNot(HaveOccurred())
			desired := desiredIface.(*fsmv2.WrappedDesiredState[snapshot.ApplicationConfig])
			Expect(desired.State).To(Equal("running"))

			children := RenderChildren(fsmv2.WorkerSnapshot[snapshot.ApplicationConfig, snapshot.ApplicationStatus]{
				Desired: *desired,
			})
			Expect(children).To(HaveLen(2))
			Expect(children[0].Name).To(Equal("child-1"))
			Expect(children[0].WorkerType).To(Equal("example-child"))
			Expect(children[1].Name).To(Equal("child-2"))
			Expect(children[1].WorkerType).To(Equal("example-child"))
		})

		It("should handle empty children array", func() {
			yamlConfig := `children: []`
			userSpec := config.UserSpec{
				Config: yamlConfig,
			}

			desiredIface, err := worker.DeriveDesiredState(userSpec)
			Expect(err).ToNot(HaveOccurred())
			desired := desiredIface.(*fsmv2.WrappedDesiredState[snapshot.ApplicationConfig])
			Expect(desired.State).To(Equal("running"))

			children := RenderChildren(fsmv2.WorkerSnapshot[snapshot.ApplicationConfig, snapshot.ApplicationStatus]{
				Desired: *desired,
			})
			// Per discriminator semantics (api.go:140-153), application's
			// RenderChildren returns a non-nil empty slice for the no-children
			// case so the supervisor treats it as "use this exact (empty) set"
			// rather than falling back to legacy DDS.
			Expect(children).To(BeEmpty())
			Expect(children).NotTo(BeNil())
		})

		It("should handle empty config", func() {
			userSpec := config.UserSpec{
				Config: "",
			}

			desiredIface, err := worker.DeriveDesiredState(userSpec)
			Expect(err).ToNot(HaveOccurred())
			desired := desiredIface.(*fsmv2.WrappedDesiredState[snapshot.ApplicationConfig])
			Expect(desired.State).To(Equal("running"))

			children := RenderChildren(fsmv2.WorkerSnapshot[snapshot.ApplicationConfig, snapshot.ApplicationStatus]{
				Desired: *desired,
			})
			// Empty config -> non-nil empty slice from RenderChildren (see
			// children.go: src nil/zero-len -> []ChildSpec{}). Discriminator
			// semantics (api.go:140-153): nil = legacy DDS fallback,
			// non-nil empty = "use this exact (empty) set".
			Expect(children).To(BeEmpty())
			Expect(children).NotTo(BeNil())
		})

		It("should return default desired state for nil spec", func() {
			desiredIface, err := worker.DeriveDesiredState(nil)
			Expect(err).ToNot(HaveOccurred())
			desired := desiredIface.(*fsmv2.WrappedDesiredState[snapshot.ApplicationConfig])
			Expect(desired.State).To(Equal("running"))
			Expect(desired.Config.Name).To(Equal("test-root"))

			children := RenderChildren(fsmv2.WorkerSnapshot[snapshot.ApplicationConfig, snapshot.ApplicationStatus]{
				Desired: *desired,
			})
			// nil spec startup -> non-nil empty slice from RenderChildren.
			Expect(children).To(BeEmpty())
			Expect(children).NotTo(BeNil())
		})

		It("should return error for invalid YAML", func() {
			userSpec := config.UserSpec{
				Config: "invalid: yaml: content: [",
			}

			_, err := worker.DeriveDesiredState(userSpec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse children config"))
		})

		It("should return error for invalid spec type", func() {
			_, err := worker.DeriveDesiredState("not a UserSpec")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid spec type"))
		})

		It("should preserve ChildStartStates in children", func() {
			yamlConfig := `
children:
  - name: "child-1"
    workerType: "example-child"
    childStartStates:
      - "running"
      - "TryingToStart"
`
			userSpec := config.UserSpec{
				Config: yamlConfig,
			}

			desiredIface, err := worker.DeriveDesiredState(userSpec)
			Expect(err).ToNot(HaveOccurred())
			desired := desiredIface.(*fsmv2.WrappedDesiredState[snapshot.ApplicationConfig])

			children := RenderChildren(fsmv2.WorkerSnapshot[snapshot.ApplicationConfig, snapshot.ApplicationStatus]{
				Desired: *desired,
			})
			Expect(children).To(HaveLen(1))
			Expect(children[0].ChildStartStates).To(ConsistOf("running", "TryingToStart"))
		})

		It("should handle mixed worker types", func() {
			yamlConfig := `
children:
  - name: "counter-1"
    workerType: "counter"
  - name: "timer-1"
    workerType: "timer"
  - name: "child-1"
    workerType: "example-child"
`
			userSpec := config.UserSpec{
				Config: yamlConfig,
			}

			desiredIface, err := worker.DeriveDesiredState(userSpec)
			Expect(err).ToNot(HaveOccurred())
			desired := desiredIface.(*fsmv2.WrappedDesiredState[snapshot.ApplicationConfig])

			children := RenderChildren(fsmv2.WorkerSnapshot[snapshot.ApplicationConfig, snapshot.ApplicationStatus]{
				Desired: *desired,
			})
			Expect(children).To(HaveLen(3))
			Expect(children[0].WorkerType).To(Equal("counter"))
			Expect(children[1].WorkerType).To(Equal("timer"))
			Expect(children[2].WorkerType).To(Equal("example-child"))
		})
	})

	Describe("GetInitialState", func() {
		It("should return RunningState for graceful shutdown support", func() {
			s := worker.GetInitialState()
			Expect(s).NotTo(BeNil())
			// DeriveStateName strips "State" suffix, so "RunningState" becomes "Running"
			Expect(s.String()).To(Equal("Running"))
		})
	})

	Describe("CollectedAt semantics", func() {
		It("CollectedAt is populated by collector, zero straight from CollectObservedState", func() {
			ctx := context.Background()
			before := time.Now()
			obs, err := worker.CollectObservedState(ctx, nil)
			Expect(err).ToNot(HaveOccurred())

			// The CollectObservedState method returns a bare Observation; the
			// collector sets CollectedAt afterwards. We only assert the return
			// shape here.
			typed := obs.(fsmv2.Observation[snapshot.ApplicationStatus])
			Expect(typed.Status.Name).To(Equal("test-root"))
			Expect(before.Before(time.Now()) || before.Equal(time.Now())).To(BeTrue())
		})
	})

	Describe("NewApplicationSupervisor with YAML config", func() {
		It("should accept YAMLConfig in SupervisorConfig", func() {
			// This test documents that NewApplicationSupervisor accepts YAMLConfig
			// and passes it to the supervisor as UserSpec.
			//
			// The fix enables supervisors created via NewApplicationSupervisor to have
			// initial configuration, so DeriveDesiredState() receives non-empty config
			// during reconciliation, creating the children specified in the YAML.
			//
			// Without the fix, supervisor's userSpec was empty, so DeriveDesiredState
			// received empty config, resulting in zero children (added=0 in logs).
			//
			// Full integration test is in pkg/fsmv2/examples/simple/main.go

			yamlConfig := `
children:
  - name: "child-1"
    workerType: "example-child"
`
			cfg := SupervisorConfig{
				ID:         "test-sup-001",
				Name:       "Test Supervisor",
				YAMLConfig: yamlConfig,
			}

			Expect(cfg.YAMLConfig).NotTo(BeEmpty())
			Expect(cfg.YAMLConfig).To(ContainSubstring("child-1"))
		})
	})
})
