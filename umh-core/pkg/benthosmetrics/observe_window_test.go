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

// How long a fired error signal stays, and what its entry reports.

package benthosmetrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
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
	failures Failures
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
		_, tl.failures = tl.tracker.observe(testPort, outputErrors(tl.errorsAt(tl.next)), tl.next)
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
		Expect(reason).To(Equal("benthos reported 1 output error in the last 10 minutes"))

		Expect(tl.failures.Entries).To(HaveLen(1))
		e := tl.failures.Entries[0]
		Expect(e.Signal).To(Equal("output_error"))
		Expect(e.Label).To(Equal("output error"))
		Expect(e.Count).To(Equal(int64(1)))
		Expect(e.Since).To(Equal(failureT))
		Expect(e.LastErrorAt).To(Equal(failureT), "the entry's counter rose at T")
		Expect(e.Connection).To(BeFalse(), "an error entry is not a connection entry")
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

	// A second error at T+5m holds the entry until 10 minutes after that
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

		for i := 0; i < 5; i++ {
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
		Expect(reason).To(Equal("benthos reported 5 output errors in the last 10 minutes"))
	})

	// Scrape stores observe's Failures on the sample, so a rise between two
	// real scrapes reaches the sample as not error-free.
	It("carries the failures on Scrape", func() {
		var outputError int64

		srv, port := inlineServer(func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path != "/metrics" {
				return false
			}
			_, _ = fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output\"} 10\n")
			_, _ = fmt.Fprintf(w, "output_error{label=\"\",path=\"root.output\"} %d\n", outputError)

			return true
		})
		defer srv.Close()

		tr := NewTracker(nil)

		sample, err := tr.Scrape(context.Background(), port)
		Expect(err).NotTo(HaveOccurred(), "first Scrape")
		Expect(sample.Failures).NotTo(BeNil(), "the first Scrape must carry its Failures")
		ok, _ := sample.Failures.ErrorFree()
		Expect(ok).To(BeTrue())

		outputError = 2
		sample, err = tr.Scrape(context.Background(), port)
		Expect(err).NotTo(HaveOccurred(), "second Scrape")
		Expect(sample.Failures).NotTo(BeNil())
		ok, _ = sample.Failures.ErrorFree()
		Expect(ok).To(BeFalse(), "output_error rose 0 -> 2 between the scrapes")
	})

	// errorTable()'s error signals reach the availability the generated
	// diagnosis suite expects for each case.
	It("conforms to the table", func() {
		errorWant := map[diagnosis.Case]diagnosis.Availability{
			diagnosis.CaseLive:          diagnosis.Ready,
			diagnosis.CaseBriefOutage:   diagnosis.NoneReady,
			diagnosis.CaseLongOutage:    diagnosis.AllAbsent,
			diagnosis.CaseUnsupported:   diagnosis.Ready,
			diagnosis.CasePostOutageDip: diagnosis.NoneReady,
			diagnosis.CaseBelowFloor:    diagnosis.NoneReady,
		}
		want := map[string]map[diagnosis.Case]diagnosis.Availability{}
		for _, c := range errorCounters {
			want[c.name] = errorWant
		}

		outcomes := diagnosis.Run(errorTable(), diagnosis.NewEnvironment(), failureFeed{})
		seen := map[string]map[diagnosis.Case]bool{}
		for _, o := range outcomes {
			if seen[o.Signal] == nil {
				seen[o.Signal] = map[diagnosis.Case]bool{}
			}
			seen[o.Signal][o.Case] = true
			w, ok := want[o.Signal]
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

	// Failures.NotCleared judges each error entry against its own LastErrorAt,
	// so an entry stuck past its window counts even while another counter
	// keeps rising.
	It("counts an entry stuck past its window while another counter keeps rising", func() {
		now := failureT.Add(12 * time.Minute)
		f := Failures{
			ObservedAt: now,
			Entries: []FailureEntry{
				{Signal: "output_error", LastErrorAt: failureT},
				{Signal: "processor_error", LastErrorAt: now.Add(-30 * time.Second)},
			},
		}

		Expect(f.NotCleared(time.Minute)).To(BeTrue(),
			"the output_error entry is stuck past its window")
	})

	// An output error every minute for 15 minutes keeps one entry open since
	// T, but each new error restarts its window, so the entry does not count:
	// NotCleared counts from the last error, not from when the entry
	// appeared.
	It("does not count an entry while its errors keep arriving", func() {
		tl := newErrorTimeline(failureT.Add(-time.Second), func(at time.Time) int64 {
			if at.Before(failureT) {
				return 0
			}

			return 1 + int64(at.Sub(failureT)/time.Minute)
		})

		tl.runTo(failureT.Add(15 * time.Minute))
		Expect(tl.failures.Entries).To(HaveLen(1))
		Expect(tl.failures.Entries[0].Since).To(Equal(failureT), "the entry has been open since T")
		Expect(tl.failures.Entries[0].LastErrorAt).To(Equal(failureT.Add(15 * time.Minute)))
		Expect(tl.failures.NotCleared(time.Minute)).To(BeFalse(),
			"the last error was at this very scrape")
	})

	// An entry may clear up to margin late before it counts. NotCleared is
	// true only when the last error is more than ErrorWindow plus margin
	// behind the scrape.
	It("gives an entry its margin before it counts as not cleared", func() {
		now := failureT.Add(15 * time.Minute)
		lastErrorAgo := func(age time.Duration) Failures {
			return Failures{ObservedAt: now, Entries: []FailureEntry{{
				Signal: "output_error", Since: now.Add(-age), LastErrorAt: now.Add(-age),
			}}}
		}

		f := lastErrorAgo(ErrorWindow + 30*time.Second)
		Expect(f.NotCleared(time.Minute)).To(BeFalse(), "30s late is inside the 1m margin")
		f = lastErrorAgo(ErrorWindow + time.Minute)
		Expect(f.NotCleared(time.Minute)).To(BeFalse(), "exactly window plus margin is not more than it")
		f = lastErrorAgo(ErrorWindow + time.Minute + time.Second)
		Expect(f.NotCleared(time.Minute)).To(BeTrue(), "one second past window plus margin counts as not cleared")
	})
})

// failureFeed is the diagnosis.Run feed for errorTable(): every error
// counter readable and unchanged, or every error counter unknown.
type failureFeed struct{}

func (failureFeed) Readable(time.Time) readings {
	s := readings{
		inputReceived: diagnosis.Unknown(),
		outputSent:    diagnosis.Unknown(),
		errors:        map[string]diagnosis.Reading{},
	}
	for _, c := range errorCounters {
		s.errors[c.name] = diagnosis.Known(0)
	}

	return s
}

// Unreadable leaves every error reading out of the map; a missing key reads
// as an unknown reading.
func (failureFeed) Unreadable(time.Time) readings {
	return readings{
		inputReceived: diagnosis.Unknown(),
		outputSent:    diagnosis.Unknown(),
		errors:        map[string]diagnosis.Reading{},
	}
}
