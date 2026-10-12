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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// throughput is what one scrape's counters say about message flow.
type throughput struct {
	Input    ComponentThroughput
	Output   ComponentThroughput
	IsActive bool
}

// observation is one observed scrape: when, from which port, the raw counters,
// and the result observe returned for it.
type observation struct {
	at            time.Time
	port          int
	inputReceived int
	outputSent    int
	result        throughput
}

// observe records one successful scrape of the benthos at port, taken at time
// at, and returns the throughput over the window that ends at the newest
// observed scrape.
//
// A scrape whose at is not after the previous one's is not observed, and the
// previous result is returned. The engine's windows must receive points in time
// order.
//
// The engine is rebuilt when the port changed or when a counter dropped. A new
// port is a different benthos, and a dropped counter means benthos restarted,
// so the old points belong to a different counter series. The rebuilt engine
// holds one point, so the first rate after a rebuild is 0. Without the rebuild,
// the rate would be the slope from the old count to the new one, a negative
// number.
func observe(d *benthosMonitorDeps, port int, m benthosmonitorservice.Metrics, at time.Time) throughput {
	cur := observation{
		at:            at,
		port:          port,
		inputReceived: int(m.InputReceivedTotal()),
		outputSent:    int(m.OutputSentTotal()),
	}

	prev := d.prev
	switch {
	case prev == nil:
		d.engine = newEngine()
	case !at.After(prev.at):
		return prev.result
	case restarted(prev, &cur):
		d.engine = newEngine()
	}

	d.engine.Observe(sample{
		inputReceived: diagnosis.Known(float64(cur.inputReceived)),
		outputSent:    diagnosis.Known(float64(cur.outputSent)),
	}, diagnosis.Environment{}, at)

	cur.result = throughput{
		Input: ComponentThroughput{
			MessagesPerSecond: rate(d.engine.Measurement(inputRate)),
			LastCount:         cur.inputReceived,
		},
		Output: ComponentThroughput{
			MessagesPerSecond: rate(d.engine.Measurement(outputRate)),
			LastCount:         cur.outputSent,
		},
	}
	cur.result.IsActive = cur.result.Input.MessagesPerSecond > 0
	d.prev = &cur

	return cur.result
}

// restarted reports whether cur comes from a different counter series than prev:
// another port, or any counter lower than before. A drop in the output counter
// alone counts too: a reconfigured or restarted benthos can leave the input
// counter where it was.
func restarted(prev, cur *observation) bool {
	return cur.port != prev.port ||
		cur.inputReceived < prev.inputReceived ||
		cur.outputSent < prev.outputSent
}

// rate is a measurement's slope, or 0 while the engine marks it untrusted or
// absent, as with fewer than two points.
func rate(r diagnosis.Reduced) float64 {
	v, state := r.Get()
	if state != diagnosis.StateValue {
		return 0
	}

	return v
}
