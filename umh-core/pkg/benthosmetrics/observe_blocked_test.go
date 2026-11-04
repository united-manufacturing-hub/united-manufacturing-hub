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

// When a flow's outputs count as blocked, and when they do not.

package benthosmetrics

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// outputStep is one scrape: its time after failureT, the port it came from
// (0 is testPort), and the counters it shows.
type outputStep struct {
	at              time.Duration
	port            int
	sent, errors    int64
	processorErrors int64

	// switchSibling replaces the single output with a switch output whose
	// first case sends s.sent messages while the second case, stuck at 10,
	// fails s.errors writes: one output that keeps sending among a blocked
	// sibling.
	switchSibling bool
}

func (s outputStep) metrics() Metrics {
	m := Metrics{
		Inputs:  map[string]InputMetrics{"root.input": {Received: 10, ConnectionUp: 1}},
		Outputs: map[string]OutputMetrics{"root.output": {Sent: s.sent, Error: s.errors, ConnectionUp: 1}},
		Process: ProcessMetrics{Processors: map[string]ProcessorMetrics{
			"root.pipeline.processors.0": {Error: s.processorErrors},
		}},
	}
	if s.switchSibling {
		m.Outputs = map[string]OutputMetrics{
			"root.output.cases.0": {Sent: s.sent, ConnectionUp: 1},
			"root.output.cases.1": {Sent: 10, Error: s.errors, ConnectionUp: 1},
		}
	}

	return m
}

// blockedVerdict observes each step in order, starting from a Tracker that
// has seen no scrape, and returns the Failures at the last one.
func blockedVerdict(steps []outputStep) Failures {
	tr := NewTracker(nil)
	var v Failures
	for _, s := range steps {
		port := s.port
		if port == 0 {
			port = testPort
		}
		_, v = tr.observe(port, s.metrics(), failureT.Add(s.at))
	}

	return v
}

// wantReasonAfter asserts that the Failures at the last step are not
// error-free and name want as their reason. why says what the steps show.
func wantReasonAfter(steps []outputStep, want, why string) {
	v := blockedVerdict(steps)
	ok, reason := v.ErrorFree()
	ExpectWithOffset(1, ok).To(BeFalse(), "%s: %q, want %q", why, reason, want)
	ExpectWithOffset(1, reason).To(Equal(want), why)
}

// secondsSteps builds one scrape per second for n seconds. At second i the
// output has sent sent(i) messages and failed errors(i) writes.
func secondsSteps(n int, sent, errors func(i int) int64) []outputStep {
	steps := make([]outputStep, 0, n)
	for i := 0; i < n; i++ {
		steps = append(steps, outputStep{at: time.Duration(i) * time.Second, sent: sent(i), errors: errors(i)})
	}

	return steps
}

// pausedSendSteps builds a minute of scrapes that send one message a second,
// then a pause in scrapes that ends at resumeAt, with six scrapes that each
// fail one write. A message went out during the pause, so output_sent rose
// across it.
func pausedSendSteps(resumeAt time.Duration) []outputStep {
	steps := make([]outputStep, 0, 66)
	for i := int64(0); i < 60; i++ {
		steps = append(steps, outputStep{at: time.Duration(i) * time.Second, sent: i})
	}
	for k := int64(0); k < 6; k++ {
		steps = append(steps, outputStep{at: resumeAt + time.Duration(k)*time.Second, sent: 300, errors: k + 1})
	}

	return steps
}

func stuckAt(c int64) func(int) int64 { return func(int) int64 { return c } }
func onePerSecond(i int) int64        { return int64(i) }

var _ = Describe("An output whose writes fail", func() {
	// Writes fail every second and nothing is sent for more than two
	// minutes, so the flow's outputs count as blocked.
	It("is quiet under errors for two minutes", func() {
		wantReasonAfter(secondsSteps(150, stuckAt(10), onePerSecond),
			"benthos output blocked: no message sent for 2 minutes",
			"errors every second, nothing sent for 149s")
	})

	// The reason names how long nothing was sent, in whole minutes, not the
	// two minutes it takes to call the outputs blocked.
	It("names how long it has sent nothing in the reason", func() {
		wantReasonAfter(secondsSteps(200, stuckAt(10), onePerSecond),
			"benthos output blocked: no message sent for 3 minutes",
			"errors every second, nothing sent for 199s")
	})

	// The same errors while messages still go out are output errors, not
	// blocked outputs.
	It("reports its errors, not blocked, while it is still sending", func() {
		wantReasonAfter(secondsSteps(150, onePerSecond, onePerSecond),
			"benthos reported 149 output errors in the last 10 minutes",
			"errors and sends every second")
	})

	// After 90 seconds of failed writes the output has not yet gone two
	// minutes without a sent message.
	It("is not blocked under two minutes", func() {
		wantReasonAfter(secondsSteps(90, stuckAt(10), onePerSecond),
			"benthos reported 89 output errors in the last 10 minutes",
			"errors every second, nothing sent for 89s")
	})

	// The output sent nothing for over three minutes, and one write failed.
	// One error is below the three blocked outputs need.
	It("is not blocked after one error", func() {
		errors := func(i int) int64 { return min(max(int64(i)-195, 0), 1) }
		wantReasonAfter(secondsSteps(200, stuckAt(10), errors),
			"benthos reported 1 output error in the last 10 minutes",
			"nothing sent for 199s, one error at 196s")
	})

	// Two errors are below the three blocked outputs need.
	It("is not blocked after two errors", func() {
		errors := func(i int) int64 { return min(max(int64(i)-196, 0), 2) }
		wantReasonAfter(secondsSteps(200, stuckAt(10), errors),
			"benthos reported 2 output errors in the last 10 minutes",
			"nothing sent for 199s, errors at 197s and 198s")
	})

	// Five writes failed, then none for over two minutes. Nothing was sent,
	// but the output is not failing now.
	It("is not blocked when it stopped failing", func() {
		errors := func(i int) int64 { return min(int64(i), 5) }
		wantReasonAfter(secondsSteps(200, stuckAt(10), errors),
			"benthos reported 5 output errors in the last 10 minutes",
			"five errors, then none for 195s")
	})

	// Once messages go out again, the reason goes back to counting output
	// errors.
	It("recovers", func() {
		sent := func(i int) int64 {
			if i < 150 {
				return 10
			}

			return int64(i)
		}
		errors := func(i int) int64 { return min(int64(i), 150) }
		wantReasonAfter(secondsSteps(155, sent, errors),
			"benthos reported 150 output errors in the last 10 minutes",
			"5s after messages go out again")
	})

	// A blocked output is named even while a processor error counter rises
	// faster than the output error counter, and only its entry carries
	// BlockedFor.
	It("comes first, and only it carries BlockedFor", func() {
		steps := make([]outputStep, 0, 150)
		for i := int64(0); i < 150; i++ {
			steps = append(steps, outputStep{
				at:              time.Duration(i) * time.Second,
				sent:            10,
				errors:          i,
				processorErrors: 3 * i,
			})
		}

		wantReasonAfter(steps,
			"benthos output blocked: no message sent for 2 minutes",
			"output blocked while processor errors rise 3 a second")

		for _, e := range blockedVerdict(steps).Entries {
			if e.Signal != outputError {
				Expect(e.BlockedFor).To(BeZero(),
					"entry %q: only the output error entry carries BlockedFor", e.Signal)
			}
		}
	})

	// Messages went out during a three-minute pause in scrapes, so errors in
	// the seconds after it are not a blocked output.
	It("is not blocked by a send during a scrape pause", func() {
		wantReasonAfter(pausedSendSteps(240*time.Second),
			"benthos reported 6 output errors in the last 10 minutes",
			"sent rose during the pause, 6 errors in the 6s after it")
	})

	// After a Benthos restart, the two minutes without a sent message count
	// from the restart. The output fails every write after it: at 60s it is
	// not blocked, at 130s it is.
	It("counts from a restart", func() {
		steps := make([]outputStep, 0, 331)
		for i := int64(0); i < 200; i++ {
			steps = append(steps, outputStep{at: time.Duration(i) * time.Second, sent: 10})
		}
		for i := int64(200); i <= 330; i++ {
			steps = append(steps, outputStep{at: time.Duration(i) * time.Second, errors: i - 199})
		}

		wantReasonAfter(steps[:261],
			"benthos reported 61 output errors in the last 10 minutes",
			"60s after the restart")

		wantReasonAfter(steps,
			"benthos output blocked: no message sent for 2 minutes",
			"130s after the restart")
	})

	// output_error is already 100 at umh-core's first scrape. Two more
	// errors after it are below the three blocked outputs need.
	It("does not count errors before the first scrape", func() {
		errors := func(i int) int64 { return 100 + min(max(int64(i)-125, 0), 2) }
		wantReasonAfter(secondsSteps(130, stuckAt(10), errors),
			"benthos reported 2 output errors in the last 10 minutes",
			"counter at 100 before the first scrape, 2 errors after it")
	})

	// 99 writes failed while messages still went out. After the last message,
	// two more failed, below the three blocked outputs need.
	It("does not count errors while sending", func() {
		sent := func(i int) int64 { return min(int64(i), 99) }
		errors := func(i int) int64 { return min(int64(i), 99) + min(max(int64(i)-225, 0), 2) }
		wantReasonAfter(secondsSteps(230, sent, errors),
			"benthos reported 101 output errors in the last 10 minutes",
			"errors while sending, then 2 after the last send")
	})

	// The reason names a duration a reader does not have to divide in
	// their head, however long nothing was sent.
	It("names a quiet time over an hour in hours and minutes", func() {
		steps := make([]outputStep, 0, 152)
		for i := int64(0); i <= 151; i++ {
			steps = append(steps, outputStep{at: time.Duration(i) * 30 * time.Second, sent: 10, errors: i})
		}

		wantReasonAfter(steps,
			"benthos output blocked: no message sent for 1 hour 15 minutes",
			"an error every 30s, nothing sent for 75m30s")
	})

	// One case of a switch output keeps sending while the other case's
	// every write fails for 200 s. The flow's outputs are judged together,
	// so the sending case keeps the summed output_sent rising and the
	// reason counts errors, not a blocked output. One stuck output among
	// several that still send is not reported as blocked.
	It("reports errors, not blocked, when one case of a switch still sends", func() {
		steps := make([]outputStep, 0, 200)
		for i := int64(0); i < 200; i++ {
			steps = append(steps, outputStep{
				at:            time.Duration(i) * time.Second,
				sent:          i,
				errors:        i,
				switchSibling: true,
			})
		}

		wantReasonAfter(steps,
			"benthos reported 199 output errors in the last 10 minutes",
			"one case sends a message a second, the other fails a write a second")
	})
})

var _ = Describe("An output whose writes fail, at its edges", func() {
	It("is blocked when quiet for exactly two minutes", func() {
		wantReasonAfter(secondsSteps(121, stuckAt(10), onePerSecond),
			"benthos output blocked: no message sent for 2 minutes",
			"errors every second, nothing sent for 120s")
	})

	It("is not blocked one second short of two minutes", func() {
		wantReasonAfter(secondsSteps(120, stuckAt(10), onePerSecond),
			"benthos reported 119 output errors in the last 10 minutes",
			"errors every second, nothing sent for 119s")
	})

	// The last failed write happened after the 2s scrape. Two minutes after
	// that scrape the output is no longer failing; one second earlier it is.
	It("is not blocked when the last failed write is two minutes back", func() {
		errors := func(i int) int64 { return min(int64(i), 3) }
		wantReasonAfter(secondsSteps(123, stuckAt(10), errors),
			"benthos reported 3 output errors in the last 10 minutes",
			"errors at 1s, 2s and 3s, scrape at 122s")

		wantReasonAfter(secondsSteps(122, stuckAt(10), errors),
			"benthos output blocked: no message sent for 2 minutes",
			"errors at 1s, 2s and 3s, scrape at 121s")
	})

	// The boundary: quiet for two minutes with exactly three failed writes
	// since the last sent message is blocked.
	It("is blocked with exactly three failed writes since the last sent message", func() {
		errors := func(i int) int64 { return min(max(int64(i)-121, 0), 3) }
		wantReasonAfter(secondsSteps(125, stuckAt(10), errors),
			"benthos output blocked: no message sent for 2 minutes",
			"quiet for 124s with exactly 3 failed writes since the last sent message")
	})

	// The counters start at 0 when Benthos restarts, so errors already on
	// them at the first scrape after it happened after the restart.
	It("counts errors already present at the first scrape after a restart", func() {
		steps := make([]outputStep, 0, 326)
		for i := int64(0); i < 200; i++ {
			steps = append(steps, outputStep{at: time.Duration(i) * time.Second, sent: 10})
		}
		for i := int64(200); i <= 325; i++ {
			e := int64(2)
			if i >= 310 {
				e = 3
			}
			steps = append(steps, outputStep{at: time.Duration(i) * time.Second, errors: e})
		}

		wantReasonAfter(steps,
			"benthos output blocked: no message sent for 2 minutes",
			"2 errors at the restart scrape, a third at 310s, scrape at 325s")
	})

	// A restart seen only as a new port, where the new Benthos has already
	// sent more than the old one, still counts errors from 0.
	It("counts errors from 0 after a new port whose sent count is higher", func() {
		steps := make([]outputStep, 0, 326)
		for i := int64(0); i < 200; i++ {
			steps = append(steps, outputStep{at: time.Duration(i) * time.Second, sent: 10})
		}
		for i := int64(200); i <= 325; i++ {
			e := int64(3)
			if i >= 310 {
				e = 4
			}
			steps = append(steps, outputStep{at: time.Duration(i) * time.Second, port: testPort + 1, sent: 50, errors: e})
		}

		wantReasonAfter(steps,
			"benthos output blocked: no message sent for 2 minutes",
			"new port at 200s with 3 errors, a fourth at 310s, scrape at 325s")
	})

	// A pause in scrapes longer than ErrorWindow rebuilds the windows, and a
	// message sent during it still counts as a send.
	It("is not blocked by a send during a scrape pause longer than the error window", func() {
		v := blockedVerdict(pausedSendSteps(900 * time.Second))
		Expect(v.Entries).NotTo(BeEmpty(), "6 errors after the pause")
		for _, e := range v.Entries {
			Expect(e.BlockedFor).To(BeZero(), "entry %q", e.Signal)
		}

		wantReasonAfter(pausedSendSteps(900*time.Second),
			"benthos reported 6 output errors in the last 10 minutes",
			"sent rose during the pause, 6 errors in the 6s after it")
	})
})
