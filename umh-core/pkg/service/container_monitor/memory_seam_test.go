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

package container_monitor_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	fsmv2memory "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/memory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

type memoryStatus = simple.Status[fsmv2memory.MemoryStatus]

type memoryStubStateReader struct {
	observation *fsmv2.Observation[memoryStatus]
	err         error
}

func (s *memoryStubStateReader) LoadObservedTyped(_ context.Context, _, _ string, result interface{}) error {
	if s.err != nil {
		return s.err
	}

	if s.observation == nil {
		return errors.New("memoryStubStateReader: no staged observation")
	}

	out, ok := result.(*fsmv2.Observation[memoryStatus])
	if !ok {
		return errors.New("memoryStubStateReader: unexpected result type")
	}

	*out = *s.observation

	return nil
}

type recordedWarning struct {
	Feature deps.Feature
	Message string
}

type warningRecorder struct {
	deps.FSMLogger

	warnings *[]recordedWarning
}

func (r warningRecorder) SentryWarn(feature deps.Feature, _ string, msg string, _ ...deps.Field) {
	*r.warnings = append(*r.warnings, recordedWarning{Feature: feature, Message: msg})
}

var (
	healthyWorkerMemory = fsmv2memory.MemoryStatus{
		Source: fsmv2memory.SourceCgroup, UsedBytes: 500, TotalBytes: 1000, UsedPercent: 50, Message: "worker says normal",
	}
	criticalWorkerMemory = fsmv2memory.MemoryStatus{
		Source: fsmv2memory.SourceCgroup, UsedBytes: 900, TotalBytes: 1000, UsedPercent: 90, Message: "worker says critical",
	}
)

func freshObservation(status memoryStatus) *fsmv2.Observation[memoryStatus] {
	return &fsmv2.Observation[memoryStatus]{CollectedAt: time.Now(), Status: status}
}

func publishMemoryClient(stub *memoryStubStateReader, registered bool) {
	writer := dynamicchildren.NewWriter()
	if registered {
		Expect(writer.Upsert(fsmv2memory.Ref, map[string]any{})).To(Succeed())
	}

	previous := fsmv2client.GetClient()
	fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, stub))
	DeferCleanup(func() { fsmv2client.SetClient(previous) })
}

func clearMemoryClient() {
	previous := fsmv2client.GetClient()
	fsmv2client.SetClient(nil)
	DeferCleanup(func() { fsmv2client.SetClient(previous) })
}

func newFlaggedService(memoryFlag string) *container_monitor.ContainerMonitorService {
	GinkgoT().Setenv("USE_FSMV2_MEMORY_MONITOR", memoryFlag)
	GinkgoT().Setenv("USE_FSMV2_CPU", "false")

	return container_monitor.NewContainerMonitorServiceWithPath(filesystem.NewMockFileSystem(), GinkgoT().TempDir())
}

var _ = Describe("the memory seam's verdict", func() {
	It("copies a fresh healthy reading", func() {
		memory := container_monitor.JudgeWorkerMemory(memoryStatus{Result: healthyWorkerMemory}, fsmv2client.Fresh)

		Expect(memory.Health.Category).To(Equal(models.Active))
		Expect(memory.Health.Message).To(Equal(healthyWorkerMemory.Message))
		Expect(memory.CGroupUsedBytes).To(Equal(healthyWorkerMemory.UsedBytes))
		Expect(memory.CGroupTotalBytes).To(Equal(healthyWorkerMemory.TotalBytes))
	})

	It("copies a fresh degraded reading", func() {
		status := memoryStatus{Result: criticalWorkerMemory, Degraded: true, Reason: criticalWorkerMemory.Message}

		memory := container_monitor.JudgeWorkerMemory(status, fsmv2client.Fresh)

		Expect(memory.Health.Category).To(Equal(models.Degraded))
		Expect(memory.Health.Message).To(Equal(criticalWorkerMemory.Message))
		Expect(memory.CGroupTotalBytes).To(Equal(criticalWorkerMemory.TotalBytes))
	})

	It("reports a failed poll as degraded without a reading", func() {
		status := memoryStatus{Degraded: true, Reason: "poll error: cgroup and host unreadable"}

		memory := container_monitor.JudgeWorkerMemory(status, fsmv2client.Fresh)

		Expect(memory.Health.Category).To(Equal(models.Degraded))
		Expect(memory.Health.Message).To(Equal(status.Reason))
		Expect(memory.CGroupTotalBytes).To(BeZero())
	})

	DescribeTable("reports a missing reading as degraded",
		func(freshness fsmv2client.Freshness, messagePart string) {
			memory := container_monitor.JudgeWorkerMemory(memoryStatus{Result: healthyWorkerMemory}, freshness)

			Expect(memory.Health.Category).To(Equal(models.Degraded))
			Expect(memory.Health.Message).To(ContainSubstring(messagePart))
			Expect(memory.CGroupUsedBytes).To(BeZero())
			Expect(memory.CGroupTotalBytes).To(BeZero())
		},
		Entry("stale", fsmv2client.Stale, "stale"),
		Entry("never observed", fsmv2client.NeverObserved, "never observed"),
		Entry("unregistered", fsmv2client.Unregistered, "not registered"),
		Entry("unknown", fsmv2client.Unknown, "could not be classified"),
	)
})

var _ = Describe("the memory seam's worker read", func() {
	It("reports a store read error as degraded", func() {
		publishMemoryClient(&memoryStubStateReader{err: errors.New("store offline")}, true)

		memory, err := newFlaggedService("true").CollectMemoryFromWorker(context.Background())

		Expect(err).ToNot(HaveOccurred())
		Expect(memory.Health.Category).To(Equal(models.Degraded))
		Expect(memory.Health.Message).To(ContainSubstring("store offline"))
	})

	It("returns the context error on a cancelled tick", func() {
		publishMemoryClient(&memoryStubStateReader{observation: freshObservation(memoryStatus{Result: healthyWorkerMemory})}, true)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := newFlaggedService("true").CollectMemoryFromWorker(ctx)

		Expect(err).To(MatchError(context.Canceled))
	})

	It("reports a missing client as degraded and warns Sentry once", func() {
		clearMemoryClient()

		warnings := &[]recordedWarning{}
		service := newFlaggedService("true")
		service.SetSentryLogger(warningRecorder{FSMLogger: deps.NewNopFSMLogger(), warnings: warnings})

		for range 3 {
			memory, err := service.CollectMemoryFromWorker(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(memory.Health.Category).To(Equal(models.Degraded))
		}

		Expect(*warnings).To(HaveLen(1))
		Expect((*warnings)[0].Feature).To(Equal(deps.FeatureSupportMemory))
	})
})

var _ = Describe("GetStatus with USE_FSMV2_MEMORY_MONITOR", func() {
	It("takes memory health from a degraded worker reading", func() {
		status := memoryStatus{Result: criticalWorkerMemory, Degraded: true, Reason: criticalWorkerMemory.Message}
		publishMemoryClient(&memoryStubStateReader{observation: freshObservation(status)}, true)

		info, err := newFlaggedService("true").GetStatus(context.Background())

		Expect(err).ToNot(HaveOccurred())
		Expect(info.MemoryHealth).To(Equal(models.Degraded))
		Expect(info.OverallHealth).To(Equal(models.Degraded))
		Expect(info.Memory.CGroupTotalBytes).To(Equal(criticalWorkerMemory.TotalBytes))
	})

	It("takes the worker's bytes for a healthy reading", func() {
		publishMemoryClient(&memoryStubStateReader{observation: freshObservation(memoryStatus{Result: healthyWorkerMemory})}, true)

		info, err := newFlaggedService("true").GetStatus(context.Background())

		Expect(err).ToNot(HaveOccurred())
		Expect(info.MemoryHealth).To(Equal(models.Active))
		Expect(info.Memory.CGroupUsedBytes).To(Equal(healthyWorkerMemory.UsedBytes))
	})

	It("ignores the worker when the flag is off", func() {
		status := memoryStatus{Result: criticalWorkerMemory, Degraded: true, Reason: criticalWorkerMemory.Message}
		publishMemoryClient(&memoryStubStateReader{observation: freshObservation(status)}, true)

		info, err := newFlaggedService("false").GetStatus(context.Background())

		Expect(err).ToNot(HaveOccurred())
		Expect(info.Memory.CGroupTotalBytes).ToNot(Equal(criticalWorkerMemory.TotalBytes))
	})
})

var _ = Describe("the memory gauge source", func() {
	It("publishes a record with a total", func() {
		usedBytes, totalBytes, ok := container_monitor.MemoryGaugeInputs(&models.Memory{CGroupUsedBytes: 500, CGroupTotalBytes: 1000})

		Expect(ok).To(BeTrue())
		Expect(usedBytes).To(Equal(500.0))
		Expect(totalBytes).To(Equal(1000.0))
	})

	It("publishes nothing for a record without a reading", func() {
		_, _, ok := container_monitor.MemoryGaugeInputs(&models.Memory{Health: &models.Health{Category: models.Degraded}})

		Expect(ok).To(BeFalse())
	})

	It("publishes nothing without a record", func() {
		_, _, ok := container_monitor.MemoryGaugeInputs(nil)

		Expect(ok).To(BeFalse())
	})
})
