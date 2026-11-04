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

// The error window: an error burst degrades the flow, and the flow
// clears ErrorWindow after the last error, without a restart.

package fsmv2benthos

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/factory"
)

var _ = Describe("The error window", func() {
	// One Benthos runs through the real framework, its output error counter
	// rises, and the flow's observed state turns degraded at once and clears
	// again benthosmetrics.ErrorWindow after the last error, with no restart.
	It("degrades on an error burst and clears ten minutes after the last error", func() {
		s := newFakeBenthos()
		defer s.close()
		s.metricsBody = metricsBodyWith(100, 80, 0)

		spec := userSpecFor("error-window", s.port())

		// The scrape clock steps one second per observation, so the ten-minute
		// window runs in seconds, not in sleeps.
		t0 := time.Now().Truncate(time.Second)
		clockNow := t0
		defer swapScrapeClock(func() time.Time { return clockNow })()

		worker, err := factory.NewWorkerByType(
			WorkerType,
			deps.Identity{ID: "error-window", Name: "error-window", WorkerType: WorkerType},
			deps.NewNopFSMLogger(),
			nil,
			nil,
		)
		Expect(err).NotTo(HaveOccurred(), "factory.NewWorkerByType")

		// The first scrape seeds the window with an error-free counter.
		clockNow = t0
		seeded := observeOneCycle(worker, spec)
		ok, _ := seeded.Result.Failures.ErrorFree()
		Expect(ok).To(BeTrue(), "the first scrape seeds an error-free window")

		// The burst: five output errors arrive between the scrapes.
		s.metricsBody = metricsBodyWith(100, 80, 5)
		clockNow = t0.Add(time.Second)
		fired := observeOneCycle(worker, spec)
		ok, _ = fired.Result.Failures.ErrorFree()
		Expect(ok).To(BeFalse(), "the error burst degrades the flow at once")
		Expect(fired.Result.Failures.Entries).To(HaveLen(1))
		Expect(fired.Result.Failures.Entries[0].Count).To(Equal(int64(5)))

		// The counter stays flat. One scrape per second, up to the last
		// scrape before the ten-minute window lets the burst go: still
		// degraded.
		for t := t0.Add(2 * time.Second); !t.After(t0.Add(10*time.Minute - time.Second)); t = t.Add(time.Second) {
			clockNow = t
			status := observeOneCycle(worker, spec)
			ok, _ = status.Result.Failures.ErrorFree()
			Expect(ok).To(BeFalse(),
				"at T+%v the burst is still inside the window", t.Sub(t0))
		}

		// The next scrape is past the window: the flow cleared without a
		// restart.
		clockNow = t0.Add(10*time.Minute + time.Second)
		cleared := observeOneCycle(worker, spec)
		ok, _ = cleared.Result.Failures.ErrorFree()
		Expect(ok).To(BeTrue(),
			"the flow clears ten minutes after the last error, with no restart")
	})
})
