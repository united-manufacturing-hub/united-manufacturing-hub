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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
)

// Compile-time interface verification.
var _ fsmv2.Worker = (*ApplicationWorker)(nil)

var _ = Describe("ApplicationWorker", func() {
	var worker *ApplicationWorker

	BeforeEach(func() {
		worker = NewApplicationWorker("root-1", "test-root")
	})

	Describe("CollectObservedState", func() {
		It("should return observed state with timestamp", func() {
			ctx := context.Background()
			obs, err := worker.CollectObservedState(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(obs).NotTo(BeNil())

			typedObs, ok := obs.(*snapshot.ApplicationObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs.GetTimestamp()).NotTo(BeZero())
			Expect(typedObs.Name).To(Equal("test-root"))
		})

		It("should respect context cancellation", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately.

			_, err := worker.CollectObservedState(ctx)
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

			desired, err := worker.DeriveDesiredState(userSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(desired.State).To(Equal("running"))
			Expect(desired.ChildrenSpecs).To(HaveLen(2))
			Expect(desired.ChildrenSpecs[0].Name).To(Equal("child-1"))
			Expect(desired.ChildrenSpecs[0].WorkerType).To(Equal("example-child"))
			Expect(desired.ChildrenSpecs[1].Name).To(Equal("child-2"))
			Expect(desired.ChildrenSpecs[1].WorkerType).To(Equal("example-child"))
		})

		It("should handle empty children array", func() {
			yamlConfig := `children: []`
			userSpec := config.UserSpec{
				Config: yamlConfig,
			}

			desired, err := worker.DeriveDesiredState(userSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(desired.State).To(Equal("running"))
			Expect(desired.ChildrenSpecs).To(BeEmpty())
		})

		It("should handle empty config", func() {
			userSpec := config.UserSpec{
				Config: "",
			}

			desired, err := worker.DeriveDesiredState(userSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(desired.State).To(Equal("running"))
			Expect(desired.ChildrenSpecs).To(BeNil())
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

		It("should preserve state mapping in children", func() {
			yamlConfig := `
children:
  - name: "child-1"
    workerType: "example-child"
    stateMapping:
      running: "active"
      stopped: "idle"
`
			userSpec := config.UserSpec{
				Config: yamlConfig,
			}

			desired, err := worker.DeriveDesiredState(userSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(desired.ChildrenSpecs).To(HaveLen(1))
			Expect(desired.ChildrenSpecs[0].StateMapping).To(HaveKeyWithValue("running", "active"))
			Expect(desired.ChildrenSpecs[0].StateMapping).To(HaveKeyWithValue("stopped", "idle"))
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

			desired, err := worker.DeriveDesiredState(userSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(desired.ChildrenSpecs).To(HaveLen(3))
			Expect(desired.ChildrenSpecs[0].WorkerType).To(Equal("counter"))
			Expect(desired.ChildrenSpecs[1].WorkerType).To(Equal("timer"))
			Expect(desired.ChildrenSpecs[2].WorkerType).To(Equal("example-child"))
		})
	})

	Describe("GetInitialState", func() {
		It("should return nil (placeholder for future state machine)", func() {
			state := worker.GetInitialState()
			Expect(state).To(BeNil())
		})
	})

	Describe("Timestamp freshness", func() {
		It("should return recent timestamp", func() {
			ctx := context.Background()
			before := time.Now()
			obs, err := worker.CollectObservedState(ctx)
			after := time.Now()

			Expect(err).ToNot(HaveOccurred())
			timestamp := obs.GetTimestamp()
			Expect(timestamp.After(before) || timestamp.Equal(before)).To(BeTrue())
			Expect(timestamp.Before(after) || timestamp.Equal(after)).To(BeTrue())
		})
	})
})
