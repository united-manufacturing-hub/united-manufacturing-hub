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

// Every error counter the window judges, and what its entry says.

package benthosmetrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

// errorLabels repeats each counter's label so the specs check the wording, not
// copy it from errorCounters. The first spec keeps the two lists in step.
var errorLabels = map[string]string{
	"output_error":    "output error",
	"processor_error": "processor error",
}

// verdictAfterRise scrapes before at T-1s, then after at T, and returns the
// failures at T.
func verdictAfterRise(before, after map[string]int64) Failures {
	tr := NewTracker(func() time.Time { return failureT })
	tr.observe(testPort, failureScrape(before), failureT.Add(-time.Second))
	_, f := tr.observe(testPort, failureScrape(after), failureT)

	return f
}

// entryOf returns the entry with the given signal out of f, failing the spec
// when there is none.
func entryOf(f Failures, signal string) FailureEntry {
	for _, e := range f.Entries {
		if e.Signal == signal {
			return e
		}
	}

	Fail(fmt.Sprintf("no %s entry in %+v", signal, f.Entries))

	return FailureEntry{}
}

var _ = Describe("The error counters", func() {
	// errorLabels names exactly the counters in errorCounters, so the loops
	// below cover every counter.
	It("covers every counter with a label", func() {
		Expect(errorLabels).To(HaveLen(len(errorCounters)))
		for _, c := range errorCounters {
			Expect(errorLabels[c.name]).To(Equal(c.label),
				"counter %q", c.name)
		}
	})

	// Each counter in errorLabels, raised alone by 1, makes the flow not
	// error-free with its own label.
	It("degrades on every error counter", func() {
		for name, label := range errorLabels {
			f := verdictAfterRise(nil, map[string]int64{name: 1})
			ok, reason := f.ErrorFree()
			Expect(ok).To(BeFalse(), "%s 0 -> 1", name)
			Expect(reason).To(Equal(fmt.Sprintf("benthos reported 1 %s in the last 10 minutes", label)))
			Expect(f.Entries).To(HaveLen(1), "%s raised alone", name)
		}
	})

	// One processor error is named before two inputs that are not connected,
	// although 2 is larger than 1, and both entries are present.
	It("names the error before more inputs down", func() {
		inputs := func(failed int64) map[string]conn {
			return map[string]conn{"root.input.broker.0": {failed: failed}, "root.input.broker.1": {failed: failed}}
		}

		tr := NewTracker(func() time.Time { return failureT })
		before := connScrape(inputs(0), nil)
		before.Process = ProcessMetrics{Processors: map[string]ProcessorMetrics{
			"root.pipeline.processors.0": {},
		}}
		tr.observe(testPort, before, failureT.Add(-time.Second))

		after := connScrape(inputs(1), nil)
		after.Process = ProcessMetrics{Processors: map[string]ProcessorMetrics{
			"root.pipeline.processors.0": {Error: 1},
		}}
		_, f := tr.observe(testPort, after, failureT)

		Expect(f.Entries).To(HaveLen(2))
		Expect(f.Entries[1].Count).To(Equal(int64(2)), "the connection entry holds two inputs down")
		ok, reason := f.ErrorFree()
		Expect(ok).To(BeFalse())
		Expect(reason).To(Equal("benthos reported 1 processor error in the last 10 minutes"))
	})

	// Of two error counters, the one with the larger current increase is
	// named first.
	It("names the larger increase first", func() {
		f := verdictAfterRise(nil, map[string]int64{"output_error": 1, "processor_error": 3})
		ok, reason := f.ErrorFree()
		Expect(ok).To(BeFalse())
		Expect(reason).To(Equal("benthos reported 3 processor errors in the last 10 minutes"))
	})

	// A Benthos with no input, output or processor paths reports every
	// counter as 0 and nothing fires.
	It("reads empty metrics as error-free", func() {
		tr := NewTracker(func() time.Time { return failureT })
		for i := 0; i < 3; i++ {
			_, f := tr.observe(testPort, Metrics{}, failureT.Add(time.Duration(i)*time.Second))
			ok, _ := f.ErrorFree()
			Expect(ok).To(BeTrue(), "scrape %d of empty Metrics", i)
		}
	})

	// For each counter, fire it, then drop only that counter. The drop is a
	// restart, so the next verdict is error-free.
	It("reads a drop in any error counter as a restart", func() {
		for name := range errorLabels {
			tr := NewTracker(func() time.Time { return failureT })
			_, f := tr.observe(testPort, failureScrape(map[string]int64{name: 3}), failureT.Add(-time.Second))
			ok, _ := f.ErrorFree()
			Expect(ok).To(BeTrue(), "the first scrape does not backfill %s", name)

			_, f = tr.observe(testPort, failureScrape(map[string]int64{name: 4}), failureT)
			ok, _ = f.ErrorFree()
			Expect(ok).To(BeFalse(), "setup: %s 3 -> 4 did not fire", name)

			_, f = tr.observe(testPort, failureScrape(map[string]int64{name: 0}), failureT.Add(time.Second))
			ok, _ = f.ErrorFree()
			Expect(ok).To(BeTrue(), "%s 4 -> 0 is a restart", name)
		}
	})

	// Each error counter, raised in a real /metrics body between two scrapes,
	// reaches the verdict with its label. This covers the parser's name for
	// each counter.
	It("judges every error counter from the wire", func() {
		lines := map[string]string{
			"output_error":    `output_error{label="",path="root.output"} %d`,
			"processor_error": `processor_error{label="",path="root.pipeline.processors.0"} %d`,
		}

		for name, label := range errorLabels {
			var value int64

			srv, port := inlineServer(func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != "/metrics" {
					return false
				}
				_, _ = fmt.Fprintf(w, lines[name]+"\n", value)

				return true
			})
			DeferCleanup(srv.Close)

			tr := NewTracker(nil)

			_, err := tr.Scrape(context.Background(), port)
			Expect(err).NotTo(HaveOccurred(), "first Scrape")

			value = 2
			sample, err := tr.Scrape(context.Background(), port)
			Expect(err).NotTo(HaveOccurred(), "second Scrape")

			Expect(sample.Failures).NotTo(BeNil(), "%s on the wire: the sample carries no Failures", name)
			ok, reason := sample.Failures.ErrorFree()
			Expect(ok).To(BeFalse(), "%s 0 -> 2 on the wire", name)
			Expect(reason).To(Equal(fmt.Sprintf("benthos reported 2 %ss in the last 10 minutes", label)))
		}
	})

	// Each error entry carries the scrape its counter last rose in
	// LastErrorAt; a connection entry's is zero.
	It("sets each error entry's LastErrorAt to its counter's last rise", func() {
		tr := NewTracker(func() time.Time { return failureT })
		scrape := func(at time.Time, m Metrics) Failures {
			_, f := tr.observe(testPort, m, at)

			return f
		}

		scrape(failureT.Add(-time.Second), failureScrape(nil))
		scrape(failureT, failureScrape(map[string]int64{"output_error": 1}))
		second := scrape(failureT.Add(30*time.Second), failureScrape(map[string]int64{
			"output_error":    1,
			"processor_error": 2,
		}))

		Expect(entryOf(second, "output_error").LastErrorAt).To(Equal(failureT),
			"output_error rose at T")
		Expect(entryOf(second, "processor_error").LastErrorAt).To(Equal(failureT.Add(30*time.Second)),
			"processor_error rose at T+30s")

		// output_error rises again while both signals are fired.
		third := scrape(failureT.Add(time.Minute), failureScrape(map[string]int64{
			"output_error":    2,
			"processor_error": 2,
		}))
		Expect(entryOf(third, "output_error").LastErrorAt).To(Equal(failureT.Add(time.Minute)),
			"output_error rose again at T+1m")
		Expect(entryOf(third, "processor_error").LastErrorAt).To(Equal(failureT.Add(30*time.Second)),
			"processor_error did not rise again")

		// A scrape with no rise keeps each entry's last rise.
		fourth := scrape(failureT.Add(2*time.Minute), failureScrape(map[string]int64{
			"output_error":    2,
			"processor_error": 2,
		}))
		Expect(entryOf(fourth, "output_error").LastErrorAt).To(Equal(failureT.Add(time.Minute)))
		Expect(entryOf(fourth, "processor_error").LastErrorAt).To(Equal(failureT.Add(30 * time.Second)))
	})
})
