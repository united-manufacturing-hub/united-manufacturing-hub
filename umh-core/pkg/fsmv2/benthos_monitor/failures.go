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

package fsmv2benthosmonitor

import (
	"fmt"
	"sort"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// failureCounter is one cumulative error counter the worker judges.
type failureCounter struct {
	// name is the counter's name everywhere: the table's signal, the
	// sample's key, and the verdict's entry.
	name string
	// label is the wording the entry's reason shows.
	label string
	// tier is the counter's rank class in the verdict's order, lower first.
	tier int
	// read sums the counter off one scrape's Metrics.
	read func(benthosmonitorservice.Metrics) int64
}

// failureCounters lists every counter the failure signals are built from.
var failureCounters = []failureCounter{
	{
		name:  "output_error",
		label: "output errors",
		tier:  0,
		read:  benthosmonitorservice.Metrics.OutputErrorTotal,
	},
}

// increase is the reduction the failure signals judge by: how much a counter
// rose between the window's oldest and newest sample. Min 2, because a single
// sample has no rise to read.
var increase = mustIncrease()

func mustIncrease() diagnosis.Reduction {
	r, err := diagnosis.NewReduction("increase", 2, func(p []diagnosis.Point) float64 {
		return p[len(p)-1].Value - p[0].Value
	})
	if err != nil {
		panic(fmt.Sprintf("benthos_monitor: invalid increase reduction: %v", err))
	}

	return r
}

func readFailures(m benthosmonitorservice.Metrics) map[string]int64 {
	out := make(map[string]int64, len(failureCounters))
	for _, c := range failureCounters {
		out[c.name] = c.read(m)
	}

	return out
}

// sampleFrom turns one observation into the sample the engine reads: the
// throughput counters and one reading per failure counter.
func sampleFrom(cur observation) sample {
	s := sample{
		inputReceived: diagnosis.Known(float64(cur.inputReceived)),
		outputSent:    diagnosis.Known(float64(cur.outputSent)),
		failures:      make(map[string]diagnosis.Reading, len(cur.failures)),
	}
	for name, v := range cur.failures {
		s.failures[name] = diagnosis.Known(float64(v))
	}

	return s
}

// counterByName finds the failureCounter a signal name was built from. Every
// signal in the table is built from a failureCounters entry, so the not-found
// arm has no reachable caller.
func counterByName(name string) (failureCounter, bool) {
	for _, c := range failureCounters {
		if c.name == name {
			return c, true
		}
	}

	return failureCounter{}, false
}

// failureVerdict turns the fired set of one Observe call into the verdict Poll
// reports, sorted by tier, then by the current increase, largest first; ties
// keep the fired order. A window with fewer readings than the increase
// reduction needs reports 0.
func failureVerdict(e *diagnosis.Engine[sample], fired []diagnosis.Fired, at time.Time) benthosmonitorservice.FailureVerdict {
	v := benthosmonitorservice.FailureVerdict{ObservedAt: at}
	for _, f := range fired {
		c, ok := counterByName(f.Signal)
		if !ok {
			continue
		}

		increaseNow, _ := e.Reduction(c.name, c.name).Get()
		v.Fired = append(v.Fired, benthosmonitorservice.FailureEntry{
			Signal:   c.name,
			Label:    c.label,
			Increase: increaseNow,
			Since:    f.Since,
			Window:   constants.BenthosLogWindow,
		})
	}

	sort.SliceStable(v.Fired, func(i, j int) bool {
		a, b := v.Fired[i], v.Fired[j]
		ac, _ := counterByName(a.Signal)
		bc, _ := counterByName(b.Signal)
		if ac.tier != bc.tier {
			return ac.tier < bc.tier
		}

		return a.Increase > b.Increase
	})

	return v
}
