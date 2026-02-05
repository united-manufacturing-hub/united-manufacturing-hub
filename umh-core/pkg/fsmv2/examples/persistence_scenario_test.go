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

	Describe("Worker lifecycle", func() {
		It("fires compaction immediately on first tick because LastCompactionAt starts at zero time", func() {
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: 2 * time.Second,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done
			Expect(result.CompactionCycles).To(BeNumerically(">=", 1),
				"compaction should fire on first tick because LastCompactionAt starts at Go zero time (year 0001)")
		})

		It("executes maintenance after compaction completes", func() {
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: 3 * time.Second,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done
			Expect(result.MaintenanceCycles).To(BeNumerically(">=", 1),
				"maintenance should run after compaction; both start at zero time so both fire early")
		})

		It("reports healthy status when all actions succeed", func() {
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: 2 * time.Second,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done
			Expect(result.Healthy).To(BeTrue(),
				"worker should be healthy when compaction and maintenance complete without errors")
		})
	})

	Describe("Observable metrics and timestamps", func() {
		It("updates LastCompactionAt to a time after scenario start", func() {
			startTime := time.Now()
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: 2 * time.Second,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done
			Expect(result.LastCompactionAt).NotTo(BeZero())
			Expect(result.LastCompactionAt.After(startTime)).To(BeTrue(),
				"LastCompactionAt should be updated to execution time, not stay at zero")
		})

		It("updates LastMaintenanceAt to a time after scenario start", func() {
			startTime := time.Now()
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: 2 * time.Second,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done
			Expect(result.LastMaintenanceAt).NotTo(BeZero())
			Expect(result.LastMaintenanceAt.After(startTime)).To(BeTrue(),
				"LastMaintenanceAt should be updated to execution time, not stay at zero")
		})

		It("returns all expected result fields after completion", func() {
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: 2 * time.Second,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			Expect(result.Done).NotTo(BeNil())
			Expect(result.Shutdown).NotTo(BeNil())
			<-result.Done
			Expect(result.CompactionCycles).To(BeNumerically(">=", 0))
			Expect(result.MaintenanceCycles).To(BeNumerically(">=", 0))
			Expect(result.Healthy).To(BeTrue())
			Expect(result.LastCompactionAt).NotTo(BeZero())
			Expect(result.LastMaintenanceAt).NotTo(BeZero())
		})
	})

	Describe("Shutdown and lifecycle", func() {
		It("closes Done channel after supervisor fully stops", func() {
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: 500 * time.Millisecond,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			Eventually(result.Done, 25*time.Second).Should(BeClosed(),
				"Done channel should close after supervisor completes graceful shutdown")
		})

		It("Shutdown function triggers clean termination before duration expires", func() {
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: 30 * time.Second,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			time.Sleep(500 * time.Millisecond)
			result.Shutdown()
			Eventually(result.Done, 25*time.Second).Should(BeClosed(),
				"calling Shutdown should terminate scenario before full duration elapses")
		})
	})

	Describe("Configuration", func() {
		It("uses custom logger without affecting behavior", func() {
			result := examples.RunPersistenceScenario(ctx, examples.PersistenceRunConfig{
				Duration: 1 * time.Second,
				Logger:   zap.NewNop().Sugar(),
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done
			Expect(result.CompactionCycles).To(BeNumerically(">=", 1))
		})

		It("uses custom tick interval", func() {
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

		It("handles zero duration and runs until context cancelled", func() {
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
