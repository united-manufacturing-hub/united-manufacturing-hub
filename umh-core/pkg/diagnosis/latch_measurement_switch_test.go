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

// These specs pin what happens to a fired signal when the measurement that
// fired it stops being readable while a second measurement keeps answering.
//
// The clear arm of Latch.Update releases only on the mark pair the episode
// fired under, which is right: spare cores on the host and our own usage
// fraction are different numbers on different scales, and one cannot declare
// the other recovered. But a measurement can also go away for good. Then the
// pair that fired can never be read again, nothing can reach the clear arm, and
// the engine's staleness fallback never runs either, because a second
// measurement keeps the signal Ready.
//
// So the latch moves the episode onto the marks it can still read, and judges
// against those from there on. These specs drive a real Engine, because the
// hold-forever only appears through the engine's availability handling.

package diagnosis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("A fired signal is judged on a measurement it can still read", func() {
	const (
		tick       = time.Second
		armSpan    = 10 * time.Second
		demoteSpan = 30 * time.Second
	)

	type cpuSnap struct {
		headroom float64
		usage    float64
	}

	// Two answers to one question that are not comparable: spare cores on the
	// host falling is bad, our own usage fraction rising is bad.
	headroomMarks := Marks{
		Unit:     "cores",
		Fire:     Mark{At: 0},
		Clear:    Mark{At: 0.5},
		Polarity: LowerIsWorse,
		Worst:    -1,
	}
	usageMarks := Marks{
		Unit:     "fraction",
		Fire:     Mark{At: 0.70},
		Clear:    Mark{At: 0.60},
		Polarity: HigherIsWorse,
		Worst:    1.0,
	}

	// drive fires the signal on the headroom measurement, then takes that
	// measurement away for long enough that its window empties, while the usage
	// measurement reports usageAfter on every one of those ticks. It returns the
	// fired set and the readiness rows of the last tick.
	drive := func(usageAfter float64) ([]Fired, []Readiness) {
		headroomReadable := true

		sig := Signal[cpuSnap]{
			Name:       "saturation",
			DemoteSpan: demoteSpan,
			Instruments: []Instrument[cpuSnap]{
				{
					Measurement: Measurement[cpuSnap]{
						Name: "host-headroom",
						Extract: func(s cpuSnap) Reading {
							if !headroomReadable {
								return Unknown()
							}

							return Known(s.headroom)
						},
						Reduction: Last,
						Span:      armSpan,
					},
					Marks: headroomMarks,
				},
				{
					Measurement: Measurement[cpuSnap]{
						Name:      "usage-fraction",
						Extract:   func(s cpuSnap) Reading { return Known(s.usage) },
						Reduction: Last,
						Span:      armSpan,
					},
					Marks: usageMarks,
				},
			},
		}

		e, err := NewEngine(Table[cpuSnap]{Signals: []Signal[cpuSnap]{sig}, Interval: tick})
		Expect(err).ToNot(HaveOccurred())

		env := NewEnvironment()
		base := time.Unix(9_000_000, 0)
		at := func(i int) time.Time { return base.Add(time.Duration(i) * tick) }

		// Fifteen ticks at 0.2 cores short of the fire mark. The headroom
		// measurement is declared first and answers every tick, so it is the one
		// that fires, and its window covers its whole span by the end.
		var fired []Fired
		for i := range 15 {
			fired, _ = e.Observe(cpuSnap{headroom: -0.2, usage: 0.10}, env, at(i))
		}

		Expect(fired).To(HaveLen(1), "the headroom measurement fires the signal")
		Expect(fired[0].Instrument).To(Equal("host-headroom"))
		Expect(fired[0].Marks.Unit).To(Equal("cores"))

		// Now host stats stop being readable. Sixty ticks is twice the demote
		// span, so the headroom window is not merely stale, it is empty: that
		// measurement can never answer this signal again. Usage answers every one
		// of those ticks, so the signal stays Ready and the engine's staleness
		// fallback is never reached.
		headroomReadable = false

		var readiness []Readiness
		for i := 15; i < 75; i++ {
			fired, readiness = e.Observe(cpuSnap{usage: usageAfter}, env, at(i))
		}

		return fired, readiness
	}

	It("releases a signal whose live measurement reads healthy on its own marks", func() {
		fired, readiness := drive(0.40)

		Expect(readiness).To(HaveLen(1))
		Expect(readiness[0].Availability).To(Equal(Ready),
			"usage keeps answering, so the engine never reaches its staleness fallback")
		Expect(fired).To(BeEmpty(),
			"0.40 usage is past the 0.60 clear mark, and nothing else can be read, so the signal must release")
	})

	It("holds a signal whose live measurement reads inside its own hysteresis band", func() {
		fired, readiness := drive(0.65)

		Expect(readiness).To(HaveLen(1))
		Expect(readiness[0].Availability).To(Equal(Ready))
		Expect(fired).To(HaveLen(1),
			"0.65 usage is between the 0.60 clear and the 0.70 fire mark, which is where the signal holds")
	})

	It("holds a signal whose live measurement reads past its own fire mark, and blames that measurement", func() {
		fired, readiness := drive(0.85)

		Expect(readiness).To(HaveLen(1))
		Expect(readiness[0].Availability).To(Equal(Ready))
		Expect(fired).To(HaveLen(1), "0.85 usage is past the 0.70 fire mark, so the signal stays fired")
		Expect(fired[0].Instrument).To(Equal("usage-fraction"),
			"the reason a caller is shown must be a number the caller can still read")
		Expect(fired[0].Marks.Unit).To(Equal("fraction"),
			"a value in fractions scored against marks in cores is not a severity")
		Expect(fired[0].Value).To(Equal(0.85))
	})
})
