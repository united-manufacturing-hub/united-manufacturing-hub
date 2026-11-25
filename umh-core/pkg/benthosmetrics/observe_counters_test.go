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

// Every error counter the window judges, and what it reports.

package benthosmetrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics/benthosmetricstest"
)

// failureScrape returns a scrape built from v: the error counters, and the
// failed and lost connection counts of one input and one output, keyed as
// below. A key missing from v is 0.
func failureScrape(v map[string]int64) Metrics {
	return Metrics{
		Inputs: map[string]InputMetrics{"root.input": {
			Received:         10,
			ConnectionFailed: v["input_connection_failed"],
			ConnectionLost:   v["input_connection_lost"],
		}},
		Outputs: map[string]OutputMetrics{"root.output": {
			Sent:             10,
			Error:            v["output_error"],
			ConnectionFailed: v["output_connection_failed"],
			ConnectionLost:   v["output_connection_lost"],
		}},
		Process: ProcessMetrics{Processors: map[string]ProcessorMetrics{
			"root.pipeline.processors.0": {Error: v["processor_error"]},
		}},
	}
}

// verdictAfterRise scrapes before at T-1s, then after at T, and returns the
// failures at T.
func verdictAfterRise(before, after map[string]int64) FailureReport {
	tr := NewTracker(func() time.Time { return failureT })
	tr.observe(testPort, failureScrape(before), failureT.Add(-time.Second))
	f := *tr.observe(testPort, failureScrape(after), failureT).Failures

	return f
}

var _ = Describe("The error counters", func() {
	// Each counter, raised alone by 1, makes the flow not error-free and
	// fills only its own field. failures_test.go checks the wording.
	It("degrades on every error counter", func() {
		for _, c := range errorCounters {
			f := verdictAfterRise(nil, map[string]int64{string(c.name): 1})
			ok, _ := f.ErrorFree()
			Expect(ok).To(BeFalse(), "%s 0 -> 1", c.name)
			Expect(*c.field(&f)).NotTo(BeNil(), "%s raised alone", c.name)
			Expect(failuresIn(f)).To(Equal(1), "%s raised alone", c.name)
		}
	})

	// A Benthos with no input, output or processor paths reports every
	// counter as 0 and nothing fires.
	It("reads empty metrics as error-free", func() {
		tr := NewTracker(func() time.Time { return failureT })
		for i := range 3 {
			f := *tr.observe(testPort, Metrics{}, failureT.Add(time.Duration(i)*time.Second)).Failures
			ok, _ := f.ErrorFree()
			Expect(ok).To(BeTrue(), "scrape %d of empty Metrics", i)
		}
	})

	// For each counter, fire it, then drop only that counter. The drop is a
	// restart, so the next verdict is error-free.
	It("reads a drop in any error counter as a restart", func() {
		for _, c := range errorCounters {
			name := c.name
			tr := NewTracker(func() time.Time { return failureT })
			f := *tr.observe(testPort, failureScrape(map[string]int64{string(name): 3}), failureT.Add(-time.Second)).Failures
			ok, _ := f.ErrorFree()
			Expect(ok).To(BeTrue(), "the first scrape does not backfill %s", name)

			f = *tr.observe(testPort, failureScrape(map[string]int64{string(name): 4}), failureT).Failures
			ok, _ = f.ErrorFree()
			Expect(ok).To(BeFalse(), "setup: %s 3 -> 4 did not fire", name)

			f = *tr.observe(testPort, failureScrape(map[string]int64{string(name): 0}), failureT.Add(time.Second)).Failures
			ok, _ = f.ErrorFree()
			Expect(ok).To(BeTrue(), "%s 4 -> 0 is a restart", name)
		}
	})

	// Each error counter, raised in a real /metrics body between two scrapes,
	// reaches its own field in the report. This covers the parser's name for
	// each counter.
	It("judges every error counter from the wire", func() {
		lines := map[signal]string{
			signalOutputError:    `output_error{label="",path="root.output"} %d`,
			signalProcessorError: `processor_error{label="",path="root.pipeline.processors.0"} %d`,
		}

		for _, c := range errorCounters {
			name := c.name

			var value int64

			srv := benthosmetricstest.NewServer()
			port := int(srv.Port())
			srv.SetHandler(func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != "/metrics" {
					return false
				}

				_, _ = fmt.Fprintf(w, lines[name]+"\n", value)

				return true
			})
			DeferCleanup(srv.Close)

			tr := NewTracker(time.Now)

			_, _, err := tr.Scrape(context.Background(), port)
			Expect(err).NotTo(HaveOccurred(), "first Scrape")

			value = 2
			_, sample, err := tr.Scrape(context.Background(), port)
			Expect(err).NotTo(HaveOccurred(), "second Scrape")

			Expect(sample.Failures).NotTo(BeNil(), "%s on the wire: the sample carries no Failures", name)
			ok, _ := sample.Failures.ErrorFree()
			Expect(ok).To(BeFalse(), "%s 0 -> 2 on the wire", name)

			errs := *c.field(sample.Failures)
			Expect(errs).NotTo(BeNil(), "%s on the wire", name)
			Expect(errs.Count).To(Equal(int64(2)), "%s 0 -> 2 on the wire", name)
		}
	})

	// Each counter's errors carry the scrape it last rose in LastErrorAt.
	It("sets each counter's LastErrorAt to its counter's last rise", func() {
		tr := NewTracker(func() time.Time { return failureT })
		scrape := func(at time.Time, m Metrics) FailureReport {
			f := *tr.observe(testPort, m, at).Failures

			return f
		}

		scrape(failureT.Add(-time.Second), failureScrape(nil))
		scrape(failureT, failureScrape(map[string]int64{"output_error": 1}))
		second := scrape(failureT.Add(30*time.Second), failureScrape(map[string]int64{
			"output_error":    1,
			"processor_error": 2,
		}))

		Expect(second.OutputErrors.LastErrorAt).To(Equal(failureT),
			"output_error rose at T")
		Expect(second.ProcessorErrors.LastErrorAt).To(Equal(failureT.Add(30*time.Second)),
			"processor_error rose at T+30s")

		// output_error rises again while both signals are fired.
		third := scrape(failureT.Add(time.Minute), failureScrape(map[string]int64{
			"output_error":    2,
			"processor_error": 2,
		}))
		Expect(third.OutputErrors.LastErrorAt).To(Equal(failureT.Add(time.Minute)),
			"output_error rose again at T+1m")
		Expect(third.ProcessorErrors.LastErrorAt).To(Equal(failureT.Add(30*time.Second)),
			"processor_error did not rise again")

		// A scrape with no rise keeps each counter's last rise.
		fourth := scrape(failureT.Add(2*time.Minute), failureScrape(map[string]int64{
			"output_error":    2,
			"processor_error": 2,
		}))
		Expect(fourth.OutputErrors.LastErrorAt).To(Equal(failureT.Add(time.Minute)))
		Expect(fourth.ProcessorErrors.LastErrorAt).To(Equal(failureT.Add(30 * time.Second)))
	})
})
