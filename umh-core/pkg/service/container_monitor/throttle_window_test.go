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

package container_monitor

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

func newThrottleTestService() *ContainerMonitorService {
	return NewContainerMonitorServiceWithPath(filesystem.NewMockFileSystem(), "/tmp/test")
}

var _ = Describe("updateThrottleWindow", func() {
	var svc *ContainerMonitorService

	BeforeEach(func() {
		svc = newThrottleTestService()
	})

	Context("on first read", func() {
		It("should return zero ratio and not throttled", func() {
			ratio, isThrottled := svc.updateThrottleWindow(&CPUCgroupInfo{
				NrPeriods:   1000,
				NrThrottled: 50,
			})

			Expect(ratio).To(Equal(0.0), "expected ratio 0.0 on first read")
			Expect(isThrottled).To(BeFalse(), "expected isThrottled=false on first read")
		})
	})

	Context("delta calculation", func() {
		It("should compute the correct throttle ratio from deltas", func() {
			now := time.Now()
			svc.throttleSnapshots = []cgroupSnapshot{
				{timestamp: now.Add(-1 * time.Second), nrPeriods: 1000, nrThrottled: 50},
			}

			ratio, isThrottled := svc.updateThrottleWindow(&CPUCgroupInfo{
				NrPeriods:   1010,
				NrThrottled: 55,
			})

			expectedRatio := 5.0 / 10.0
			Expect(ratio).To(Equal(expectedRatio))
			Expect(isThrottled).To(BeTrue(), "expected isThrottled=true for 50%% ratio")
		})
	})

	Context("no throttling", func() {
		It("should return zero ratio when no new throttling occurred", func() {
			now := time.Now()
			svc.throttleSnapshots = []cgroupSnapshot{
				{timestamp: now.Add(-1 * time.Second), nrPeriods: 1000, nrThrottled: 50},
			}

			ratio, isThrottled := svc.updateThrottleWindow(&CPUCgroupInfo{
				NrPeriods:   1010,
				NrThrottled: 50,
			})

			Expect(ratio).To(Equal(0.0))
			Expect(isThrottled).To(BeFalse())
		})
	})

	Context("counter reset", func() {
		It("should clear snapshots and return zero on counter reset", func() {
			now := time.Now()
			svc.throttleSnapshots = []cgroupSnapshot{
				{timestamp: now.Add(-1 * time.Second), nrPeriods: 100000, nrThrottled: 5000},
			}

			ratio, isThrottled := svc.updateThrottleWindow(&CPUCgroupInfo{
				NrPeriods:   100,
				NrThrottled: 5,
			})

			Expect(ratio).To(Equal(0.0))
			Expect(isThrottled).To(BeFalse())
			Expect(svc.throttleSnapshots).To(HaveLen(1))
		})
	})

	Context("nil cgroup info", func() {
		It("should return zero ratio for nil input", func() {
			ratio, isThrottled := svc.updateThrottleWindow(nil)

			Expect(ratio).To(Equal(0.0))
			Expect(isThrottled).To(BeFalse())
		})
	})

	Context("zero periods", func() {
		It("should return zero ratio and not store snapshot", func() {
			ratio, isThrottled := svc.updateThrottleWindow(&CPUCgroupInfo{
				NrPeriods:   0,
				NrThrottled: 0,
			})

			Expect(ratio).To(Equal(0.0))
			Expect(isThrottled).To(BeFalse())
			Expect(svc.throttleSnapshots).To(HaveLen(0))
		})
	})

	Context("pruning old entries", func() {
		It("should remove entries older than the throttle window", func() {
			now := time.Now()
			svc.throttleSnapshots = []cgroupSnapshot{
				{timestamp: now.Add(-120 * time.Second), nrPeriods: 500, nrThrottled: 25},
				{timestamp: now.Add(-90 * time.Second), nrPeriods: 600, nrThrottled: 30},
				{timestamp: now.Add(-30 * time.Second), nrPeriods: 900, nrThrottled: 45},
			}

			svc.updateThrottleWindow(&CPUCgroupInfo{
				NrPeriods:   1000,
				NrThrottled: 50,
			})

			Expect(svc.throttleSnapshots).To(HaveLen(2))
		})
	})

	Context("below threshold", func() {
		It("should not flag as throttled when ratio is below threshold", func() {
			now := time.Now()
			svc.throttleSnapshots = []cgroupSnapshot{
				{timestamp: now.Add(-1 * time.Second), nrPeriods: 1000, nrThrottled: 50},
			}

			ratio, isThrottled := svc.updateThrottleWindow(&CPUCgroupInfo{
				NrPeriods:   1100,
				NrThrottled: 51,
			})

			expectedRatio := 1.0 / 100.0
			Expect(ratio).To(Equal(expectedRatio))
			Expect(isThrottled).To(BeFalse(), "expected isThrottled=false for ratio below threshold (%f < %f)", ratio, constants.CPUThrottleRatioThreshold)
		})
	})

	Context("throttle then recover", func() {
		It("should show throttling while window contains throttled data, then recover", func() {
			now := time.Now()

			// Phase 1: Window contains throttled data from the past
			svc.throttleSnapshots = []cgroupSnapshot{
				{timestamp: now.Add(-4 * time.Second), nrPeriods: 1000, nrThrottled: 50},
				{timestamp: now.Add(-3 * time.Second), nrPeriods: 1010, nrThrottled: 58},
				{timestamp: now.Add(-2 * time.Second), nrPeriods: 1020, nrThrottled: 65},
			}

			// Phase 2: New read with no throttling
			ratio, isThrottled := svc.updateThrottleWindow(&CPUCgroupInfo{
				NrPeriods:   1030,
				NrThrottled: 65,
			})

			// Window: 30 periods, 15 throttled = 50%
			expectedRatio := 15.0 / 30.0
			Expect(ratio).To(Equal(expectedRatio))
			Expect(isThrottled).To(BeTrue(), "expected isThrottled=true while window still contains throttled data")

			// Phase 3: Old throttled entries fall out of window
			svc.throttleSnapshots = []cgroupSnapshot{
				{timestamp: now.Add(-1 * time.Second), nrPeriods: 1030, nrThrottled: 65},
			}

			ratio, isThrottled = svc.updateThrottleWindow(&CPUCgroupInfo{
				NrPeriods:   1040,
				NrThrottled: 65,
			})

			Expect(ratio).To(Equal(0.0))
			Expect(isThrottled).To(BeFalse(), "expected isThrottled=false after throttling stopped and window slid past")
		})
	})
})
