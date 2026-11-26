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

package benthos

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	benthos_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

var _ = Describe("IsBenthosLogsFine log window", func() {
	const startupError = `level=error msg="error getting symbol for batch notification" @service=benthos symbol=MAIN.fTemp`

	var (
		instance *BenthosInstance
		start    time.Time
	)

	BeforeEach(func() {
		cfg := config.BenthosConfig{FSMInstanceConfig: config.FSMInstanceConfig{Name: "logs-window-test"}}
		instance = NewBenthosInstanceWithService(cfg, benthos_service.NewDefaultBenthosService(cfg.Name))
		start = time.Date(2026, 11, 13, 10, 22, 18, 500_000_000, time.UTC)
	})

	setProcess := func(info s6service.ServiceInfo, logs ...s6service.LogEntry) {
		instance.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo = info
		instance.ObservedState.ServiceInfo.BenthosStatus.BenthosLogs = logs
	}

	// A plugin logs its startup error a fraction of a second after the
	// process starts. The line must stay in the window on every tick,
	// whatever the sub-second offset of the tick, and the reason must say
	// when it leaves the 10-minute window.
	DescribeTable("keeps an error logged right after the process start",
		func(sinceStart time.Duration, wantClearsIn string) {
			now := start.Add(sinceStart)
			setProcess(
				s6service.ServiceInfo{
					Status:        s6service.ServiceUp,
					LastChangedAt: start,
					Uptime:        int64(now.Sub(start).Seconds()),
				},
				s6service.LogEntry{Timestamp: start.Add(800 * time.Millisecond), Content: startupError},
			)

			fine, reason := instance.IsBenthosLogsFine(now, constants.BenthosLogWindow)
			Expect(fine).To(BeFalse())
			Expect(reason).To(HavePrefix("found error (clears in " + wantClearsIn + "): "))
		},
		Entry("tick with a sub-second offset below the log line", 18*time.Second+300*time.Millisecond, "9m43s"),
		Entry("tick with a sub-second offset above the log line", 18*time.Second+900*time.Millisecond, "9m42s"),
	)

	It("ignores an error from the previous process", func() {
		now := start.Add(20 * time.Second)
		setProcess(
			s6service.ServiceInfo{Status: s6service.ServiceUp, LastChangedAt: start, Uptime: 20},
			s6service.LogEntry{Timestamp: start.Add(-100 * time.Millisecond), Content: startupError},
		)

		fine, _ := instance.IsBenthosLogsFine(now, constants.BenthosLogWindow)
		Expect(fine).To(BeTrue())
	})

	It("ignores all logs while the process is down", func() {
		now := start.Add(20 * time.Second)
		setProcess(
			s6service.ServiceInfo{Status: s6service.ServiceDown, LastChangedAt: start},
			s6service.LogEntry{Timestamp: start.Add(-5 * time.Second), Content: startupError},
		)

		fine, _ := instance.IsBenthosLogsFine(now, constants.BenthosLogWindow)
		Expect(fine).To(BeTrue())
	})
})

var _ = Describe("IsBenthosLogsFine countdown", func() {
	const (
		brokerError   = `level=error msg="failed to connect to broker" @service=benthos`
		dispatchError = `level=error msg="failed to dispatch message" @service=benthos`
		flushError    = `level=error msg="failed to flush batch" @service=benthos`
	)

	var (
		instance *BenthosInstance
		start    time.Time
	)

	BeforeEach(func() {
		cfg := config.BenthosConfig{FSMInstanceConfig: config.FSMInstanceConfig{Name: "logs-countdown-test"}}
		instance = NewBenthosInstanceWithService(cfg, benthos_service.NewDefaultBenthosService(cfg.Name))
		start = time.Date(2026, 11, 13, 10, 22, 18, 500_000_000, time.UTC)
	})

	setProcess := func(info s6service.ServiceInfo, logs ...s6service.LogEntry) {
		instance.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo = info
		instance.ObservedState.ServiceInfo.BenthosStatus.BenthosLogs = logs
	}

	// The process has been up for longer than the log window, so the scan
	// window is the full log window and the clamp on the process lifetime
	// plays no part in the result.
	runningLongerThanTheWindow := func(now time.Time) s6service.ServiceInfo {
		return s6service.ServiceInfo{
			Status:        s6service.ServiceUp,
			LastChangedAt: start,
			Uptime:        int64(now.Sub(start).Seconds()),
		}
	}

	// A flow stays degraded until its last error has aged out of the window,
	// so the countdown belongs to the last error. The message names the
	// first one, because that one started it.
	It("counts down to the last error, and names the first", func() {
		now := start.Add(15 * time.Minute)
		setProcess(
			runningLongerThanTheWindow(now),
			s6service.LogEntry{Timestamp: start.Add(6 * time.Minute), Content: brokerError},
			s6service.LogEntry{Timestamp: start.Add(8 * time.Minute), Content: dispatchError},
			s6service.LogEntry{Timestamp: start.Add(11 * time.Minute), Content: flushError},
		)

		fine, reason := instance.IsBenthosLogsFine(now, constants.BenthosLogWindow)
		Expect(fine).To(BeFalse())
		// start+11m + 10m window - now = 6m. Counting from the first error
		// would give 1m.
		Expect(reason).To(Equal("found error (clears in 6m0s): " + brokerError))
	})

	It("counts down from the only error", func() {
		now := start.Add(15 * time.Minute)
		setProcess(
			runningLongerThanTheWindow(now),
			s6service.LogEntry{Timestamp: start.Add(8 * time.Minute), Content: brokerError},
		)

		fine, reason := instance.IsBenthosLogsFine(now, constants.BenthosLogWindow)
		Expect(fine).To(BeFalse())
		Expect(reason).To(Equal("found error (clears in 3m0s): " + brokerError))
	})

	// An error older than the window no longer counts, so it is neither
	// named nor counted down from.
	It("ignores errors that have already aged out", func() {
		now := start.Add(15 * time.Minute)
		setProcess(
			runningLongerThanTheWindow(now),
			s6service.LogEntry{Timestamp: start.Add(1 * time.Minute), Content: dispatchError},
			s6service.LogEntry{Timestamp: start.Add(7 * time.Minute), Content: brokerError},
			s6service.LogEntry{Timestamp: start.Add(9 * time.Minute), Content: flushError},
		)

		fine, reason := instance.IsBenthosLogsFine(now, constants.BenthosLogWindow)
		Expect(fine).To(BeFalse())
		Expect(reason).To(Equal("found error (clears in 4m0s): " + brokerError))
	})
})
