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

package fsmv2memory

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/memoryhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const (
	hostUsedBytes  int64 = 3000000000
	hostTotalBytes int64 = 8000000000
)

type sentryEvent struct {
	Feature deps.Feature
	Message string
	Fields  map[string]any
}

type recordingLogger struct {
	deps.FSMLogger

	events *[]sentryEvent
}

func (l recordingLogger) With(fields ...deps.Field) deps.FSMLogger {
	return recordingLogger{FSMLogger: l.FSMLogger.With(fields...), events: l.events}
}

func (l recordingLogger) SentryWarn(feature deps.Feature, hierarchyPath, msg string, fields ...deps.Field) {
	values := map[string]any{}
	for _, field := range fields {
		values[field.Key] = field.Value
	}

	*l.events = append(*l.events, sentryEvent{Feature: feature, Message: msg, Fields: values})
	l.FSMLogger.SentryWarn(feature, hierarchyPath, msg, fields...)
}

func recordingDeps(fileSystem filesystem.Service) (*MemoryDeps, *[]sentryEvent) {
	return recordingDepsWithHost(fileSystem, hostMemoryOf(hostUsedBytes, hostTotalBytes))
}

func recordingDepsWithHost(fileSystem filesystem.Service, hostMemory memoryhealth.HostMemoryReader) (*MemoryDeps, *[]sentryEvent) {
	events := &[]sentryEvent{}
	logger := recordingLogger{FSMLogger: deps.NewNopFSMLogger(), events: events}
	identity := deps.Identity{ID: "memory-report", WorkerType: WorkerType}
	baseDependencies := deps.NewBaseDependencies(logger, nil, identity)

	return newMemoryDeps(baseDependencies, memoryhealth.NewSampler(fileSystem, fixtureCgroupBase, hostMemory)), events
}

func readFrom(memoryDeps *MemoryDeps, files map[string]string) {
	memoryDeps.sampler = memoryhealth.NewSampler(fixtureFilesystem(files), fixtureCgroupBase, hostMemoryOf(hostUsedBytes, hostTotalBytes))
}

func pollRepeatedly(memoryDeps *MemoryDeps, polls int) {
	for range polls {
		_, err := Poll(context.Background(), memoryDeps, MemoryConfig{})
		Expect(err).ToNot(HaveOccurred())
	}
}

func failingPollRepeatedly(memoryDeps *MemoryDeps, polls int) {
	for range polls {
		_, err := Poll(context.Background(), memoryDeps, MemoryConfig{})
		Expect(err).To(HaveOccurred())
	}
}

func messagesOf(events []sentryEvent) []string {
	messages := make([]string, 0, len(events))
	for _, event := range events {
		messages = append(messages, event.Message)
	}

	return messages
}

var (
	missingCgroup    = map[string]string{}
	unparsableCgroup = map[string]string{
		fixtureCgroupBase + "/memory.max":     "max\n",
		fixtureCgroupBase + "/memory.current": "abc\n",
	}
	onlyMemoryMax   = map[string]string{fixtureCgroupBase + "/memory.max": "max\n"}
	unlimitedCgroup = map[string]string{
		fixtureCgroupBase + "/memory.max":     "max\n",
		fixtureCgroupBase + "/memory.current": percentOfDecimalLimitText(50),
	}
)

var _ = Describe("the memory worker's Sentry reports", func() {
	It("reports a missing cgroup once across repeated polls", func() {
		memoryDeps, events := recordingDeps(fixtureFilesystem(missingCgroup))

		pollRepeatedly(memoryDeps, 3)

		Expect(*events).To(HaveLen(1))
		Expect((*events)[0].Feature).To(Equal(deps.FeatureSupportMemory))
		Expect((*events)[0].Message).To(Equal(cgroupReadFailedTag + "::missing"))
		Expect((*events)[0].Fields).To(HaveKeyWithValue("file", "memory.max"))
		Expect((*events)[0].Fields).To(HaveKey("error"))
	})

	It("reports the same outcome on a second file separately", func() {
		memoryDeps, events := recordingDeps(fixtureFilesystem(missingCgroup))
		pollRepeatedly(memoryDeps, 1)

		readFrom(memoryDeps, onlyMemoryMax)
		pollRepeatedly(memoryDeps, 2)

		Expect(*events).To(HaveLen(2))
		Expect((*events)[1].Message).To(Equal(cgroupReadFailedTag + "::missing"))
		Expect((*events)[1].Fields).To(HaveKeyWithValue("file", "memory.current"))
	})

	It("reports an unparsable cgroup separately from a missing one", func() {
		memoryDeps, events := recordingDeps(fixtureFilesystem(missingCgroup))
		pollRepeatedly(memoryDeps, 1)

		readFrom(memoryDeps, unparsableCgroup)
		pollRepeatedly(memoryDeps, 2)

		Expect(*events).To(HaveLen(2))
		Expect((*events)[1].Message).To(Equal(cgroupReadFailedTag + "::unparsable"))
		Expect((*events)[1].Fields).To(HaveKeyWithValue("file", "memory.current"))
	})

	It("reports nothing for a readable cgroup", func() {
		memoryDeps, events := recordingDeps(fixtureFilesystem(cgroupAtPercent(50)))

		pollRepeatedly(memoryDeps, 3)

		Expect(*events).To(BeEmpty())
	})

	It("reports an unreadable host total for a cgroup without a limit once", func() {
		memoryDeps, events := recordingDepsWithHost(fixtureFilesystem(unlimitedCgroup), unreadableHostMemory)

		failingPollRepeatedly(memoryDeps, 3)

		Expect(messagesOf(*events)).To(Equal([]string{hostReadFailedTag}))
		Expect((*events)[0].Feature).To(Equal(deps.FeatureSupportMemory))
	})

	It("reports both failures when the cgroup and the host are unreadable", func() {
		memoryDeps, events := recordingDepsWithHost(fixtureFilesystem(missingCgroup), unreadableHostMemory)

		failingPollRepeatedly(memoryDeps, 3)

		Expect(messagesOf(*events)).To(ConsistOf(cgroupReadFailedTag+"::missing", hostReadFailedTag))
	})

	It("reports a zero total once", func() {
		memoryDeps, events := recordingDepsWithHost(fixtureFilesystem(unlimitedCgroup), hostMemoryOf(0, 0))

		failingPollRepeatedly(memoryDeps, 3)

		Expect(messagesOf(*events)).To(Equal([]string{zeroTotalTag}))
	})

	It("reports nothing on a cancelled tick", func() {
		memoryDeps, events := recordingDepsWithHost(fixtureFilesystem(missingCgroup), unreadableHostMemory)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := Poll(ctx, memoryDeps, MemoryConfig{})

		Expect(err).To(MatchError(context.Canceled))
		Expect(*events).To(BeEmpty())
	})
})
