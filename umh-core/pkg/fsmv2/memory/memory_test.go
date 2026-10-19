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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/memoryhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

func pollTimes(memoryDeps *MemoryDeps, polls int) (MemoryStatus, simple.Health) {
	var status MemoryStatus

	for range polls {
		var err error

		status, err = Poll(context.Background(), memoryDeps, MemoryConfig{})
		Expect(err).ToNot(HaveOccurred())
	}

	return status, healthFromStatus(MemoryConfig{}, status)
}

var _ = Describe("the memory worker's poll", func() {
	It("reports the sample it judged", func() {
		status, _ := pollTimes(newTestDeps(fixtureFilesystem(cgroupAtPercent(50)), unreadableHostMemory), 1)

		Expect(status.Source).To(Equal(memoryhealth.SourceCgroupLimit))
		Expect(status.UsedBytes).To(Equal(decimalLimitBytes / 2))
		Expect(status.TotalBytes).To(Equal(decimalLimitBytes))
	})

	It("is degraded until the memory is measured", func() {
		status, health := pollTimes(newTestDeps(fixtureFilesystem(cgroupAtPercent(50)), unreadableHostMemory), 1)

		Expect(status.Details.Measured()).To(BeFalse())
		Expect(health.Degraded).To(BeTrue())
		Expect(health.Reason).To(Equal(status.Message))
	})

	It("is healthy once the memory is measured below the degraded level", func() {
		status, health := pollTimes(newTestDeps(fixtureFilesystem(cgroupAtPercent(50)), unreadableHostMemory), 2)

		Expect(status.Verdict.State).To(Equal(memoryhealth.StateHealthy))
		Expect(health.Degraded).To(BeFalse())
		Expect(health.Reason).To(Equal(status.Message))
	})

	It("is degraded while the verdict is degraded", func() {
		status, health := pollTimes(newTestDeps(fixtureFilesystem(cgroupAtPercent(90)), unreadableHostMemory), 2)

		Expect(status.Verdict.State).To(Equal(memoryhealth.StateDegraded))
		Expect(health.Degraded).To(BeTrue())
		Expect(health.Reason).To(Equal(status.Message))
	})

	It("fails when no memory total can be measured", func() {
		memoryDeps := newTestDeps(fixtureFilesystem(map[string]string{}), unreadableHostMemory)

		_, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

		Expect(err).To(MatchError(errHostUnreadable))
	})

	It("returns the context error on a cancelled tick", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := Poll(ctx, newTestDeps(fixtureFilesystem(cgroupAtPercent(50)), unreadableHostMemory), MemoryConfig{})

		Expect(err).To(MatchError(context.Canceled))
	})

	It("reads through the filesystem published under FilesystemDepsKey", func() {
		register.SetGlobalDeps[filesystem.Service](FilesystemDepsKey, fixtureFilesystem(cgroupAtPercent(50)))
		DeferCleanup(func() { register.ClearGlobalDeps(FilesystemDepsKey) })

		identity := deps.Identity{ID: "memory-injection", WorkerType: WorkerType}
		memoryDeps := NewDeps(identity, deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, identity), nil)

		status, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

		Expect(err).ToNot(HaveOccurred())
		Expect(status.TotalBytes).To(Equal(decimalLimitBytes))
	})
})
