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

// How long a fired error signal stays, and what it reports.

package benthosmetrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics/benthosmetricstest"
)

// failureT is the instant of the first output error in every timeline below.
var failureT = time.Unix(1_800_000_000, 0)

// outputErrors returns a scrape whose output error total is n.
func outputErrors(n int64) Metrics {
	return Metrics{
		Inputs:  map[string]InputMetrics{"root.input": {Received: 10}},
		Outputs: map[string]OutputMetrics{"root.output": {Sent: 10, Error: n}},
	}
}

// errorTimeline drives observe once per second on one Tracker. errorsAt gives
// the output error total for each scrape time.
type errorTimeline struct {
	tracker  *Tracker
	next     time.Time
	errorsAt func(time.Time) int64
	failures FailureReport
}

func newErrorTimeline(start time.Time, errorsAt func(time.Time) int64) *errorTimeline {
	return &errorTimeline{
		tracker:  NewTracker(func() time.Time { return start }),
		next:     start,
		errorsAt: errorsAt,
	}
}

// runTo observes every second up to and including until.
func (tl *errorTimeline) runTo(until time.Time) {
	for !tl.next.After(until) {
		tl.failures = *tl.tracker.observe(testPort, outputErrors(tl.errorsAt(tl.next)), tl.next).Failures
		tl.next = tl.next.Add(time.Second)
	}
}

var _ = Describe("The error window", func() {
	// One output error at T keeps the flow not error-free until the scrape
	// before it leaves the 10-minute window.
	It("holds an output error for ErrorWindow", func() {
		tl := newErrorTimeline(failureT.Add(-time.Second), func(at time.Time) int64 {
			if at.Before(failureT) {
				return 0
			}

			return 1
		})

		tl.runTo(failureT.Add(time.Second))
		ok, reason := tl.failures.ErrorFree()
		Expect(ok).To(BeFalse())
		Expect(reason).To(Equal("benthos reported 1 output error in the last 10m, 1s ago"),
			"the error rose at T and this scrape is at T+1s")

		Expect(failuresIn(tl.failures)).To(Equal(1))
		e := tl.failures.OutputErrors
		Expect(e.Count).To(Equal(int64(1)))
		Expect(e.Since).To(Equal(failureT))
		Expect(e.LastErrorAt).To(Equal(failureT), "the counter rose at T")
		Expect(tl.failures.ObservedAt).To(Equal(failureT.Add(time.Second)))

		// With a span of 9m59s the T-1s scrape would already be out of the
		// window here, and the signal would release.
		tl.runTo(failureT.Add(10*time.Minute - time.Second))
		ok, _ = tl.failures.ErrorFree()
		Expect(ok).To(BeFalse(), "the T-1s scrape is still in the window")

		tl.runTo(failureT.Add(10*time.Minute + time.Second))
		ok, _ = tl.failures.ErrorFree()
		Expect(ok).To(BeTrue(), "no rise in the last 10 minutes")
	})

	// A second error at T+5m holds the failure until 10 minutes after that
	// second error.
	It("restarts the window on a second output error", func() {
		tl := newErrorTimeline(failureT.Add(-time.Second), func(at time.Time) int64 {
			switch {
			case at.Before(failureT):
				return 0
			case at.Before(failureT.Add(5 * time.Minute)):
				return 1
			default:
				return 2
			}
		})

		tl.runTo(failureT.Add(14 * time.Minute))
		ok, _ := tl.failures.ErrorFree()
		Expect(ok).To(BeFalse(), "the T+5m error is 9 minutes old")

		tl.runTo(failureT.Add(15*time.Minute + time.Second))
		ok, _ = tl.failures.ErrorFree()
		Expect(ok).To(BeTrue(), "the last error is more than 10 minutes old")
	})

	// Errors counted before the worker's first scrape are not judged, so a
	// counter already at 5 is error-free.
	It("does not backfill on the first scrape", func() {
		tl := newErrorTimeline(failureT, func(time.Time) int64 { return 5 })

		for i := range 5 {
			tl.runTo(failureT.Add(time.Duration(i) * time.Second))
			ok, _ := tl.failures.ErrorFree()
			Expect(ok).To(BeTrue(), "scrape %d with a constant output error total of 5", i)
		}
	})

	// The reason reports the increase as it stands now, not the increase at
	// the moment the signal fired.
	It("names the current increase in the reason", func() {
		tl := newErrorTimeline(failureT.Add(-time.Second), func(at time.Time) int64 {
			switch {
			case at.Before(failureT):
				return 0
			case at.Before(failureT.Add(3 * time.Second)):
				return 1
			default:
				return 5
			}
		})

		tl.runTo(failureT.Add(3 * time.Second))
		ok, reason := tl.failures.ErrorFree()
		Expect(ok).To(BeFalse())
		Expect(reason).To(Equal("benthos reported 5 output errors in the last 10m, last one just now"))
	})

	// Scrape stores observe's FailureReport on the sample, so a rise between two
	// real scrapes reaches the sample as not error-free.
	It("carries the failures on Scrape", func() {
		var outputError int64

		srv := benthosmetricstest.NewServer()
		port := int(srv.Port())

		srv.SetHandler(func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path != "/metrics" {
				return false
			}

			_, _ = fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output\"} 10\n")
			_, _ = fmt.Fprintf(w, "output_error{label=\"\",path=\"root.output\"} %d\n", outputError)

			return true
		})
		defer srv.Close()

		tr := NewTracker(time.Now)

		_, sample, err := tr.Scrape(context.Background(), port)
		Expect(err).NotTo(HaveOccurred(), "first Scrape")
		Expect(sample.Failures).NotTo(BeNil(), "the first Scrape must carry its Failures")
		ok, _ := sample.Failures.ErrorFree()
		Expect(ok).To(BeTrue())

		outputError = 2
		_, sample, err = tr.Scrape(context.Background(), port)
		Expect(err).NotTo(HaveOccurred(), "second Scrape")
		Expect(sample.Failures).NotTo(BeNil())
		ok, _ = sample.Failures.ErrorFree()
		Expect(ok).To(BeFalse(), "output_error rose 0 -> 2 between the scrapes")
	})

	// errorTable()'s error signals reach the availability the generated
	// diagnosis suite expects for each case. The sum's Reduction.Min is 1,
	// so, as diagnosis.Run documents for m == 1, one readable tick after an
	// outage is already Ready, and CaseBelowFloor drives no readable tick and
	// reads AllAbsent.
	It("conforms to the table", func() {
		errorWant := map[diagnosis.Case]diagnosis.Availability{
			diagnosis.CaseLive:          diagnosis.Ready,
			diagnosis.CaseBriefOutage:   diagnosis.NoneReady,
			diagnosis.CaseLongOutage:    diagnosis.AllAbsent,
			diagnosis.CaseUnsupported:   diagnosis.Ready,
			diagnosis.CasePostOutageDip: diagnosis.Ready,
			diagnosis.CaseBelowFloor:    diagnosis.AllAbsent,
		}

		want := map[signal]map[diagnosis.Case]diagnosis.Availability{}
		for _, c := range errorCounters {
			want[c.name] = errorWant
		}

		outcomes := diagnosis.Run(errorTable(), diagnosis.NewEnvironment(), failureFeed{})

		seen := map[signal]map[diagnosis.Case]bool{}

		for _, o := range outcomes {
			sig := signal(o.Signal)
			if seen[sig] == nil {
				seen[sig] = map[diagnosis.Case]bool{}
			}

			seen[sig][o.Case] = true
			w, ok := want[sig]
			Expect(ok).To(BeTrue(), "signal %q is not an errorCounters entry", o.Signal)
			Expect(o.Availability).To(Equal(w[o.Case]),
				"signal %q, case %v", o.Signal, o.Case)
		}

		// A signal errorTable() dropped has no outcome, so the loop above cannot
		// see it. Every error counter must reach every case.
		for name, w := range want {
			for cs := range w {
				Expect(seen[name][cs]).To(BeTrue(),
					"signal %q, case %v: no outcome, want one per signal and case", name, cs)
			}
		}
	})
})

// failureFeed is the diagnosis.Run feed for errorTable(): every error
// counter readable and not risen, or every error counter unknown.
type failureFeed struct{}

func (failureFeed) Readable(time.Time) errorReadings {
	s := errorReadings{increases: map[signal]diagnosis.Reading{}}
	for _, c := range errorCounters {
		s.increases[c.name] = diagnosis.Known(0)
	}

	return s
}

// Unreadable leaves every error reading out of the map; a missing key reads
// as an unknown reading.
func (failureFeed) Unreadable(time.Time) errorReadings {
	return errorReadings{increases: map[signal]diagnosis.Reading{}}
}
