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

// When an input or output counts as not connected, and when that entry
// clears.

package benthosmetrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// conn is the connection counters of one input or output.
type conn struct{ up, lost, failed int64 }

// connScrape returns a scrape with the given inputs and outputs, each keyed
// by its Benthos path label, such as root.input. Every other counter is
// fixed, so only the connection counters change between scrapes.
func connScrape(inputs, outputs map[string]conn) Metrics {
	m := Metrics{
		Inputs:  map[string]InputMetrics{},
		Outputs: map[string]OutputMetrics{},
	}
	for path, c := range inputs {
		m.Inputs[path] = InputMetrics{
			Received:         10,
			ConnectionUp:     c.up,
			ConnectionLost:   c.lost,
			ConnectionFailed: c.failed,
		}
	}
	for path, c := range outputs {
		m.Outputs[path] = OutputMetrics{
			Sent:             10,
			ConnectionUp:     c.up,
			ConnectionLost:   c.lost,
			ConnectionFailed: c.failed,
		}
	}

	return m
}

// connSteps observes one scrape per second from failureT and returns each
// scrape's ErrorFree answer.
func connSteps(scrapes ...Metrics) ([]bool, []string) {
	oks := make([]bool, 0, len(scrapes))
	reasons := make([]string, 0, len(scrapes))
	tr := NewTracker(func() time.Time { return failureT })
	for i, m := range scrapes {
		_, f := tr.observe(testPort, m, failureT.Add(time.Duration(i)*time.Second))
		ok, reason := f.ErrorFree()
		oks = append(oks, ok)
		reasons = append(reasons, reason)
	}

	return oks, reasons
}

func in(c conn) map[string]conn { return map[string]conn{"root.input": c} }

var _ = Describe("The connection checks", func() {
	// An input that never connected and failed to connect is not connected,
	// from the first scrape that shows it — read here off a real /metrics
	// body, so the parser's names for the connection counters are covered
	// too.
	It("fires at once on a failed connect", func() {
		var failed int64

		srv, port := inlineServer(func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path != "/metrics" {
				return false
			}
			_, _ = fmt.Fprintf(w, "input_received{label=\"\",path=\"root.input\"} 10\n")
			_, _ = fmt.Fprintf(w, "input_connection_failed{label=\"\",path=\"root.input\"} %d\n", failed)

			return true
		})
		DeferCleanup(srv.Close)

		tr := NewTracker(nil)

		_, err := tr.Scrape(context.Background(), port)
		Expect(err).NotTo(HaveOccurred(), "first Scrape")

		// input_connection_failed rose from 0 to 2 between the scrapes.
		failed = 2
		sample, err := tr.Scrape(context.Background(), port)
		Expect(err).NotTo(HaveOccurred(), "second Scrape")
		Expect(sample.Failures).NotTo(BeNil())
		ok, reason := sample.Failures.ErrorFree()
		Expect(ok).To(BeFalse(), "input failed twice on the wire")
		Expect(reason).To(Equal("benthos reports 1 input not connected"))
	})

	// Once Benthos reconnects, the verdict is error-free within two scrapes,
	// not one failure window later. Real scrapes are not exactly one second
	// apart, and a scrape pause during the outage does not change the answer.
	It("releases within two scrapes on a reconnect", func() {
		down := connScrape(in(conn{failed: 3}), nil)
		up := connScrape(in(conn{up: 1, failed: 3}), nil)
		oks, reasons := connSteps(connScrape(in(conn{}), nil), down, down, up, up)
		Expect(oks[2]).To(BeFalse(), "setup: still down on the third scrape")
		Expect(oks[4]).To(BeTrue(), "two scrapes after the reconnect: %q", reasons[4])

		// Scrapes alternating 993 ms and 1007 ms apart: error-free within
		// three scrapes of the reconnect.
		tr := NewTracker(func() time.Time { return failureT })
		at := failureT
		gap := 993 * time.Millisecond
		jittered := func(m Metrics) bool {
			at = at.Add(gap)
			if gap == 993*time.Millisecond {
				gap = 1007 * time.Millisecond
			} else {
				gap = 993 * time.Millisecond
			}
			_, f := tr.observe(testPort, m, at)
			ok, _ := f.ErrorFree()

			return ok
		}

		jittered(connScrape(in(conn{}), nil))
		for i := 1; i <= 20; i++ {
			Expect(jittered(connScrape(in(conn{failed: int64(i)}), nil))).To(BeFalse(),
				"setup: scrape %d while down reads error-free", i)
		}

		var ok bool
		for i := 21; i <= 23; i++ {
			ok = jittered(connScrape(in(conn{up: 1, failed: 20}), nil))
		}
		Expect(ok).To(BeTrue(), "three jittered scrapes after the reconnect")

		// A 30 s scrape pause during the outage does not change the answer:
		// error-free within 11 scrapes of the reconnect.
		paused := NewTracker(func() time.Time { return failureT })
		at2 := failureT
		withGap := func(gap time.Duration, m Metrics) bool {
			at2 = at2.Add(gap)
			_, f := paused.observe(testPort, m, at2)
			ok, _ := f.ErrorFree()

			return ok
		}

		withGap(0, connScrape(in(conn{}), nil))
		for i := 1; i <= 5; i++ {
			withGap(time.Second, connScrape(in(conn{failed: int64(i)}), nil))
		}
		Expect(withGap(30*time.Second, connScrape(in(conn{failed: 6}), nil))).To(BeFalse(),
			"setup: still down after the pause")

		resumed := false
		for i := 0; i < 11 && !resumed; i++ {
			resumed = withGap(time.Second, connScrape(in(conn{up: 1, failed: 6}), nil))
		}
		Expect(resumed).To(BeTrue(), "11 scrapes after the reconnect")
	})

	// A connection that was up and was lost is not connected until
	// connection_up rises again; an input that drops again soon after the
	// reconnect is reported at once, and a second outage is reported from its
	// own start, not the first one's.
	It("fires on a lost connection and releases on a reconnect", func() {
		oks, reasons := connSteps(
			connScrape(in(conn{up: 1}), nil),
			connScrape(in(conn{up: 1, lost: 1}), nil),
			connScrape(in(conn{up: 2, lost: 1}), nil),
			connScrape(in(conn{up: 2, lost: 1}), nil),
		)
		Expect(oks[0]).To(BeTrue(), "connected input: %q", reasons[0])
		Expect(oks[1]).To(BeFalse(), "input lost")
		Expect(reasons[1]).To(Equal("benthos reports 1 input not connected"))
		Expect(oks[3]).To(BeTrue(), "after the reconnect: %q", reasons[3])

		tr := NewTracker(func() time.Time { return failureT })
		at := failureT
		obs := func(m Metrics) Failures {
			at = at.Add(time.Second)
			_, f := tr.observe(testPort, m, at)

			return f
		}

		// A drop four seconds after a reconnect is reported at once.
		obs(connScrape(in(conn{up: 1}), nil))
		for i := 0; i < 15; i++ {
			obs(connScrape(in(conn{up: 1, lost: 1}), nil))
		}
		for i := 0; i < 3; i++ {
			f := obs(connScrape(in(conn{up: 2, lost: 1}), nil))
			if i == 2 {
				ok, _ := f.ErrorFree()
				Expect(ok).To(BeTrue(), "setup: three scrapes after the reconnect")
			}
		}
		f := obs(connScrape(in(conn{up: 2, lost: 2}), nil))
		Expect(f.Entries).To(HaveLen(1))
		Expect(f.Entries[0].Count).To(Equal(int64(1)))
		ok, reason := f.ErrorFree()
		Expect(ok).To(BeFalse(), "the input dropped again 4 s after the reconnect")
		Expect(reason).To(Equal("benthos reports 1 input not connected"))

		// The second outage's entry starts at the second drop, not the first.
		for i := 0; i < 20; i++ {
			obs(connScrape(in(conn{up: 3, lost: 2}), nil))
		}
		second := obs(connScrape(in(conn{up: 3, lost: 3}), nil))
		secondDrop := at
		Expect(second.Entries).To(HaveLen(1))
		Expect(second.Entries[0].Since).To(Equal(secondDrop),
			"the second outage is reported from its own start")

		// A Benthos restart while the input is down starts the entry over: the
		// scrapes before the restart came from another process. The restarted
		// scrape's received counter sits below the old one's.
		restarted := func(c conn, received int64) Metrics {
			m := connScrape(in(c), nil)
			inst := m.Inputs["root.input"]
			inst.Received = received
			m.Inputs["root.input"] = inst

			return m
		}

		fresh := NewTracker(func() time.Time { return failureT })
		at2 := failureT
		after := func(m Metrics) Failures {
			at2 = at2.Add(time.Second)
			_, f := fresh.observe(testPort, m, at2)

			return f
		}

		after(connScrape(in(conn{}), nil))
		for k := 1; k <= 3; k++ {
			after(connScrape(in(conn{failed: int64(k)}), nil))
		}

		after(restarted(conn{failed: 1}, 1))
		restartAt := at2
		downSince := after(restarted(conn{failed: 2}, 2))
		Expect(downSince.Entries).To(HaveLen(1), "still down after the restart")
		Expect(downSince.Entries[0].Since).To(Equal(restartAt),
			"the entry starts at the first scrape after the restart")
	})

	// An input still making its first connect, with no failed attempt yet, is
	// not reported.
	It("does not count a slow first connect as down", func() {
		startup := connScrape(in(conn{}), nil)
		oks, reasons := connSteps(startup, startup, startup, startup)
		for i, ok := range oks {
			Expect(ok).To(BeTrue(), "scrape %d of a first connect in progress: %q", i, reasons[i])
		}
	})

	// One connected path does not hide another that is down, and the reason
	// counts the paths that are down.
	It("judges the connection per path", func() {
		oks, reasons := connSteps(
			connScrape(map[string]conn{"root.input.broker.0": {up: 1}, "root.input.broker.1": {}}, nil),
			connScrape(map[string]conn{"root.input.broker.0": {up: 1}, "root.input.broker.1": {failed: 2}}, nil),
		)
		Expect(oks[1]).To(BeFalse(), "one of two input paths down")
		Expect(reasons[1]).To(Equal("benthos reports 1 input not connected"))

		_, reasons = connSteps(
			connScrape(map[string]conn{"root.input.broker.0": {}, "root.input.broker.1": {}}, nil),
			connScrape(map[string]conn{"root.input.broker.0": {failed: 1}, "root.input.broker.1": {failed: 1}}, nil),
		)
		Expect(reasons[1]).To(Equal("benthos reports 2 inputs not connected"))
	})

	// The output side follows the same rule and is named as an output.
	It("checks the output connection", func() {
		out := func(c conn) map[string]conn { return map[string]conn{"root.output": c} }
		oks, reasons := connSteps(
			connScrape(nil, out(conn{up: 1})),
			connScrape(nil, out(conn{up: 1, lost: 1})),
		)
		Expect(oks[1]).To(BeFalse(), "output lost")
		Expect(reasons[1]).To(Equal("benthos reports 1 output not connected"))
	})
})
