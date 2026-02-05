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

package examples_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
)

var _ = Describe("Persistence Scenario", func() {
	var ctx context.Context
	var cancel context.CancelFunc

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	})

	AfterEach(func() {
		cancel()
	})

	Describe("Using FSMv2 worker via ApplicationSupervisor", func() {
		It("transitions to Running and executes compaction and maintenance", func() {
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: 3 * time.Second,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			Expect(result.Done).NotTo(BeNil())
			Expect(result.Shutdown).NotTo(BeNil())
			<-result.Done
			Expect(result.CompactionCycles).To(BeNumerically(">=", 1))
			Expect(result.MaintenanceCycles).To(BeNumerically(">=", 1))
		})

		It("handles custom logger", func() {
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: 1 * time.Second,
				Logger:   zap.NewNop().Sugar(),
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done
		})

		It("handles custom tick interval", func() {
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration:     1 * time.Second,
				TickInterval: 50 * time.Millisecond,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done
		})
	})

	Describe("Error conditions", func() {
		It("returns error for negative duration", func() {
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: -1 * time.Second,
			})
			Expect(result.Error).To(HaveOccurred())
			Expect(result.Error.Error()).To(ContainSubstring("invalid duration"))
		})

		It("returns error when context already cancelled", func() {
			cancelledCtx, cancelFn := context.WithCancel(ctx)
			cancelFn()

			result := examples.RunPersistenceScenario(cancelledCtx, examples.PersistenceRunConfig{
				Duration: 1 * time.Second,
			})
			Expect(result.Error).To(HaveOccurred())
			Expect(result.Error.Error()).To(ContainSubstring("context already cancelled"))
		})
	})

	Describe("Edge cases", func() {
		It("handles very short duration", func() {
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: 100 * time.Millisecond,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done
		})

		It("handles zero duration (runs until context cancelled)", func() {
			shortCtx, cancelFn := context.WithTimeout(ctx, 500*time.Millisecond)
			defer cancelFn()

			result := examples.RunPersistenceScenario(shortCtx, examples.PersistenceRunConfig{
				Duration: 0,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			Eventually(result.Done, 25*time.Second).Should(BeClosed())
		})
	})
})

var _ = Describe("PersistenceScenarioEntry registry", func() {
	It("is registered with CustomRunner that uses ApplicationSupervisor internally", func() {
		scenario, exists := examples.Registry["persistence"]
		Expect(exists).To(BeTrue())
		Expect(scenario.Name).To(Equal("persistence"))
		Expect(scenario.Description).NotTo(BeEmpty())
		Expect(scenario.Description).To(ContainSubstring("ApplicationSupervisor"))
		Expect(scenario.CustomRunner).NotTo(BeNil())
		Expect(scenario.YAMLConfig).To(BeEmpty())
	})
})
