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

package fsmv2benthos

import (
	"fmt"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	benthosmonitorfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

func testConfig() config.BenthosMonitorConfig {
	return config.BenthosMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{Name: "benthos-1", DesiredFSMState: "active"},
		MetricsPort:       4195,
	}
}

// sampleWithRates builds a Sample whose rates carry the given input and output
// messages per second. The rate fields sit in an unexported embedded struct, so
// a composite literal cannot set them; they are addressable, so assignment can.
func sampleWithRates(inPerSecond, outPerSecond float64) benthosmetrics.Sample {
	var s benthosmetrics.Sample
	s.Input.MessagesPerSecond = inPerSecond
	s.Output.MessagesPerSecond = outPerSecond

	return s
}

var _ = Describe("mapObserved", func() {
	// mapObserved fills every field of the FSMv1 BenthosMonitorObservedState:
	// the scan's timestamps, the throughput state, the health check answers,
	// and the observed config.
	It("builds the full nested structure", func() {
		sample := sampleWithRates(20, 10)
		sample.At = time.Now().Truncate(time.Second)
		sample.Metrics = benthosmetrics.Metrics{
			Inputs: map[string]benthosmetrics.InputMetrics{
				"root.input": {Path: "root.input", Received: 5},
			},
			Outputs: map[string]benthosmetrics.OutputMetrics{
				"root.output": {Path: "root.output", Sent: 3},
			},
		}
		sample.IsActive = true
		sample.PingAlive = true
		sample.Ready = true
		sample.Version = "1.2.3"

		observed := mapObserved(testConfig(), simple.Status[benthosmetrics.Sample]{Result: sample})
		bmState, ok := observed.(benthosmonitorfsm.BenthosMonitorObservedState)
		Expect(ok).To(BeTrue(), "mapObserved returned %T", observed)

		// Every pointer a consumer dereferences without a check must be
		// non-nil: GetHealthCheckAndMetrics (pkg/service/benthos/benthos.go)
		// reads LastScan.BenthosMetrics unchecked, and HasProcessingActivity
		// reads MetricsState.
		Expect(bmState.ServiceInfo).NotTo(BeNil())
		Expect(bmState.ServiceInfo.BenthosStatus.LastScan).NotTo(BeNil())
		Expect(bmState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics).NotTo(BeNil())

		ms := bmState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics.MetricsState
		Expect(ms).NotTo(BeNil())

		// The single conversion: MessagesPerSecond becomes MessagesPerTick,
		// scaled by the FSMv1 tick (constants.DefaultTickerTime).
		Expect(ms.Input.MessagesPerTick).To(BeNumerically("~", 20*constants.DefaultTickerTime.Seconds(), 1e-9))
		Expect(ms.Output.MessagesPerTick).To(BeNumerically("~", 10*constants.DefaultTickerTime.Seconds(), 1e-9))
		Expect(ms.Input.MessagesPerTick).NotTo(Equal(ms.Output.MessagesPerTick))

		// LastCount comes from the parsed counters, which is what the
		// Management Console throughput panels read.
		Expect(ms.Input.LastCount).To(Equal(int64(5)))
		Expect(ms.Output.LastCount).To(Equal(int64(3)))
		Expect(ms.IsActive).To(BeTrue(), "copied through, never scaled")

		// The scan's timestamp is the scrape's start.
		Expect(bmState.ServiceInfo.BenthosStatus.LastScan.LastUpdatedAt).To(Equal(sample.At))

		// HealthCheck mirrors the scrape's liveness/readiness/version.
		hc := bmState.ServiceInfo.BenthosStatus.LastScan.HealthCheck
		Expect(hc.IsLive).To(BeTrue())
		Expect(hc.IsReady).To(BeTrue())
		Expect(hc.Version).To(Equal("1.2.3"))

		// The observed config is the configured one.
		Expect(bmState.ObservedMonitorConfig).To(Equal(testConfig()))
	})

	// IsRunning is always true, so GetHealthCheckAndMetrics never answers
	// ErrBenthosMonitorNotRunning, which the Benthos FSM in idle, active or
	// degraded logs as an error and does not apply. Liveness carries a
	// failed or stale scrape instead: a sample vouches for IsLive and IsReady
	// for three scrape intervals.
	It("keeps IsRunning true and reports liveness only from a fresh sample", func() {
		now := time.Now()
		restore := swapScrapeClock(func() time.Time { return now })
		defer restore()

		for _, tc := range []struct {
			name   string
			status simple.Status[benthosmetrics.Sample]
			live   bool
		}{
			{"fresh good scrape", simple.Status[benthosmetrics.Sample]{
				Result: benthosmetrics.Sample{At: now, PingAlive: true, Ready: true},
			}, true},
			// A failed poll carries the last good sample (Poll); within the
			// bound it still vouches, as FSMv1 keeps its last scan.
			{"failed poll, last good sample 2 s old", simple.Status[benthosmetrics.Sample]{
				Result:   benthosmetrics.Sample{At: now.Add(-2 * time.Second), PingAlive: true, Ready: true},
				Degraded: true,
				Reason:   "poll error: scrape /version: connection refused",
			}, true},
			{"failed poll, last good sample past three scrapes", simple.Status[benthosmetrics.Sample]{
				Result:   benthosmetrics.Sample{At: now.Add(-StaleAfter - time.Millisecond), PingAlive: true, Ready: true},
				Degraded: true,
				Reason:   "poll error: scrape /version: connection refused",
			}, false},
			// A stale read: the worker stopped producing observations, and the
			// adapter forwards the last one.
			{"stale observation, not degraded", simple.Status[benthosmetrics.Sample]{
				Result: benthosmetrics.Sample{At: now.Add(-time.Hour), PingAlive: true, Ready: true},
			}, false},
			// What the adapter passes on an Unknown, NotFound or Deleted read.
			{"zero status", simple.Status[benthosmetrics.Sample]{}, false},
		} {
			bm, ok := mapObserved(testConfig(), tc.status).(benthosmonitorfsm.BenthosMonitorObservedState)
			Expect(ok).To(BeTrue(), "case %q", tc.name)
			Expect(bm.ServiceInfo.BenthosStatus.IsRunning).To(BeTrue(),
				"case %q: IsRunning=false makes the Benthos FSM stop updating its observed state", tc.name)
			hc := bm.ServiceInfo.BenthosStatus.LastScan.HealthCheck
			Expect(hc.IsLive).To(Equal(tc.live), "case %q: IsLive", tc.name)
			Expect(hc.IsReady).To(Equal(tc.live), "case %q: IsReady", tc.name)
		}
	})

	// mapObserved returns non-nil pointers for a zero sample, because
	// GetHealthCheckAndMetrics dereferences LastScan.BenthosMetrics without a
	// check.
	It("tolerates a zero status", func() {
		observed := mapObserved(testConfig(), simple.Status[benthosmetrics.Sample]{})
		bmState, ok := observed.(benthosmonitorfsm.BenthosMonitorObservedState)
		Expect(ok).To(BeTrue(), "mapObserved returned %T", observed)

		Expect(bmState.ServiceInfo).NotTo(BeNil())
		Expect(bmState.ServiceInfo.BenthosStatus.LastScan).NotTo(BeNil())
		Expect(bmState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics).NotTo(BeNil())
		Expect(bmState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics.MetricsState).NotTo(BeNil())
	})

	// A scrape FSMv1 trusts (IsRunning) of a Benthos that answers /ping, is
	// not ready and moves no messages must read live, not ready and idle:
	// each field comes from its own source.
	It("keeps liveness, readiness and activity apart", func() {
		live := simple.Status[benthosmetrics.Sample]{
			Result: benthosmetrics.Sample{At: time.Now(), PingAlive: true, Ready: false, IsActive: false},
		}
		bmState, ok := mapObserved(testConfig(), live).(benthosmonitorfsm.BenthosMonitorObservedState)
		Expect(ok).To(BeTrue())
		Expect(bmState.ServiceInfo.BenthosStatus.IsRunning).To(BeTrue(), "precondition: FSMv1 reads this scan")

		scan := bmState.ServiceInfo.BenthosStatus.LastScan
		Expect(scan.HealthCheck.IsLive).To(BeTrue(), "IsLive comes from /ping")
		Expect(scan.HealthCheck.IsReady).To(BeFalse(), "IsReady comes from /ready, not /ping")
		Expect(scan.BenthosMetrics.MetricsState.IsActive).To(BeFalse(), "an idle Benthos must read idle")
	})
})

// realBenthosMetrics is a full real Benthos /metrics body, captured from a
// running Benthos on 2025-04-24 and stored gzip-hex-encoded in
// pkg/service/benthos_monitor/test_metrics.txt; this is one of that file's 17
// bodies, decoded verbatim. It keeps what Benthos actually emits: the HELP and
// TYPE lines ("Benthos Timing metric" for the latency summaries), per-path
// labels, latency sum and count lines, and a sum in the exponent form the
// Prometheus encoder prints (2.82954e+06). The captured instance's pipeline
// declared no processors, so Benthos emitted no processor series; the spec
// below this one covers processors with its own body.
const realBenthosMetrics = `# HELP input_connection_failed Benthos Counter metric
# TYPE input_connection_failed counter
input_connection_failed{label="",path="root.input"} 0
# HELP input_connection_lost Benthos Counter metric
# TYPE input_connection_lost counter
input_connection_lost{label="",path="root.input"} 0
# HELP input_connection_up Benthos Counter metric
# TYPE input_connection_up counter
input_connection_up{label="",path="root.input"} 1
# HELP input_latency_ns Benthos Timing metric
# TYPE input_latency_ns summary
input_latency_ns{label="",path="root.input",quantile="0.5"} 174833
input_latency_ns{label="",path="root.input",quantile="0.9"} 378375
input_latency_ns{label="",path="root.input",quantile="0.99"} 858666
input_latency_ns_sum{label="",path="root.input"} 2.82954e+06
input_latency_ns_count{label="",path="root.input"} 12
# HELP input_received Benthos Counter metric
# TYPE input_received counter
input_received{label="",path="root.input"} 12
# HELP output_batch_sent Benthos Counter metric
# TYPE output_batch_sent counter
output_batch_sent{label="",path="root.output"} 12
# HELP output_connection_failed Benthos Counter metric
# TYPE output_connection_failed counter
output_connection_failed{label="",path="root.output"} 0
# HELP output_connection_lost Benthos Counter metric
# TYPE output_connection_lost counter
output_connection_lost{label="",path="root.output"} 0
# HELP output_connection_up Benthos Counter metric
# TYPE output_connection_up counter
output_connection_up{label="",path="root.output"} 1
# HELP output_error Benthos Counter metric
# TYPE output_error counter
output_error{label="",path="root.output"} 0
# HELP output_latency_ns Benthos Timing metric
# TYPE output_latency_ns summary
output_latency_ns{label="",path="root.output",quantile="0.5"} 38250
output_latency_ns{label="",path="root.output",quantile="0.9"} 94709
output_latency_ns{label="",path="root.output",quantile="0.99"} 138250
output_latency_ns_sum{label="",path="root.output"} 606294
output_latency_ns_count{label="",path="root.output"} 12
# HELP output_sent Benthos Counter metric
# TYPE output_sent counter
output_sent{label="",path="root.output"} 12
`

var _ = Describe("mapObserved's Metrics copy", func() {
	// mapObserved's field-by-field copy of the library's Metrics equals
	// FSMv1's own parser run on the same bytes: a real Benthos /metrics
	// body, parsed once by pkg/benthosmetrics and once by
	// pkg/service/benthos_monitor, must yield the same counters.
	It("copies Metrics equal to FSMv1's parse of a real Benthos body", func() {
		body := []byte(realBenthosMetrics)

		libMetrics, err := benthosmetrics.ParseMetricsFromBytes(body)
		Expect(err).NotTo(HaveOccurred(), "the library parser on the real body")
		fsmv1Metrics, err := benthosmonitorservice.ParseMetricsFromBytes(body)
		Expect(err).NotTo(HaveOccurred(), "FSMv1's parser on the real body")

		// Fixture check: the body must carry real counters, or the equality
		// below would hold vacuously.
		Expect(libMetrics.InputReceivedTotal()).To(BeNumerically(">", 0), "the real body's input counters")

		sample := benthosmetrics.Sample{At: time.Now(), Metrics: libMetrics}
		observed := mapObserved(testConfig(), simple.Status[benthosmetrics.Sample]{Result: sample})
		bmState, ok := observed.(benthosmonitorfsm.BenthosMonitorObservedState)
		Expect(ok).To(BeTrue(), "mapObserved returned %T", observed)

		converted := bmState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics.Metrics
		Expect(converted).To(Equal(fsmv1Metrics),
			"mapObserved's copy of the library's parse must equal FSMv1's own parse of the same bytes")
	})

	// This body sets every field FSMv1's Metrics has to a distinct non-zero
	// value, on two input paths and two output paths, so a copy that keeps
	// only the first path cannot pass; bridge health (IsMetricsErrorFree)
	// reads the error counters.
	It("copies every counter equal to FSMv1's parse, errors included", func() {
		const body = `input_connection_failed{label="in",path="root.input"} 11
input_connection_failed{label="in",path="root.input.broker.1"} 51
input_connection_lost{label="in",path="root.input"} 12
input_connection_lost{label="in",path="root.input.broker.1"} 52
input_connection_up{label="in",path="root.input"} 13
input_connection_up{label="in",path="root.input.broker.1"} 53
input_received{label="in",path="root.input"} 14
input_received{label="in",path="root.input.broker.1"} 54
input_latency_ns{label="in",path="root.input",quantile="0.5"} 15
input_latency_ns{label="in",path="root.input",quantile="0.9"} 16
input_latency_ns{label="in",path="root.input",quantile="0.99"} 17
input_latency_ns{label="in",path="root.input.broker.1",quantile="0.5"} 55
input_latency_ns{label="in",path="root.input.broker.1",quantile="0.9"} 56
input_latency_ns{label="in",path="root.input.broker.1",quantile="0.99"} 57
input_latency_ns_sum{label="in",path="root.input"} 18
input_latency_ns_sum{label="in",path="root.input.broker.1"} 58
input_latency_ns_count{label="in",path="root.input"} 19
input_latency_ns_count{label="in",path="root.input.broker.1"} 59
output_batch_sent{label="out",path="root.output"} 21
output_batch_sent{label="out",path="root.output.fallback.1"} 61
output_connection_failed{label="out",path="root.output"} 22
output_connection_failed{label="out",path="root.output.fallback.1"} 62
output_connection_lost{label="out",path="root.output"} 23
output_connection_lost{label="out",path="root.output.fallback.1"} 63
output_connection_up{label="out",path="root.output"} 24
output_connection_up{label="out",path="root.output.fallback.1"} 64
output_error{label="out",path="root.output"} 25
output_error{label="out",path="root.output.fallback.1"} 65
output_sent{label="out",path="root.output"} 26
output_sent{label="out",path="root.output.fallback.1"} 66
output_latency_ns{label="out",path="root.output",quantile="0.5"} 27
output_latency_ns{label="out",path="root.output",quantile="0.9"} 28
output_latency_ns{label="out",path="root.output",quantile="0.99"} 29
output_latency_ns{label="out",path="root.output.fallback.1",quantile="0.5"} 67
output_latency_ns{label="out",path="root.output.fallback.1",quantile="0.9"} 68
output_latency_ns{label="out",path="root.output.fallback.1",quantile="0.99"} 69
output_latency_ns_sum{label="out",path="root.output"} 30
output_latency_ns_sum{label="out",path="root.output.fallback.1"} 70
output_latency_ns_count{label="out",path="root.output"} 31
output_latency_ns_count{label="out",path="root.output.fallback.1"} 71
processor_received{label="proc",path="root.pipeline.processors.0"} 41
processor_batch_received{label="proc",path="root.pipeline.processors.0"} 42
processor_sent{label="proc",path="root.pipeline.processors.0"} 43
processor_batch_sent{label="proc",path="root.pipeline.processors.0"} 44
processor_error{label="proc",path="root.pipeline.processors.0"} 45
processor_latency_ns{label="proc",path="root.pipeline.processors.0",quantile="0.5"} 46
processor_latency_ns{label="proc",path="root.pipeline.processors.0",quantile="0.9"} 47
processor_latency_ns{label="proc",path="root.pipeline.processors.0",quantile="0.99"} 48
processor_latency_ns_sum{label="proc",path="root.pipeline.processors.0"} 49
processor_latency_ns_count{label="proc",path="root.pipeline.processors.0"} 50
`
		libMetrics, err := benthosmetrics.ParseMetricsFromBytes([]byte(body))
		Expect(err).NotTo(HaveOccurred())
		fsmv1Metrics, err := benthosmonitorservice.ParseMetricsFromBytes([]byte(body))
		Expect(err).NotTo(HaveOccurred())

		// Fixture check: two input paths and two output paths, one processor,
		// and no field left at its zero value, or the equality below would
		// hold vacuously for it.
		Expect(fsmv1Metrics.Inputs).To(HaveLen(2))
		Expect(fsmv1Metrics.Outputs).To(HaveLen(2))
		Expect(fsmv1Metrics.Process.Processors).To(HaveLen(1))
		Expect(zeroFields(reflect.ValueOf(fsmv1Metrics), "Metrics")).To(BeEmpty())

		sample := benthosmetrics.Sample{At: time.Now(), Metrics: libMetrics}
		bmState, ok := mapObserved(testConfig(), simple.Status[benthosmetrics.Sample]{Result: sample}).(benthosmonitorfsm.BenthosMonitorObservedState)
		Expect(ok).To(BeTrue())
		Expect(bmState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics.Metrics).To(Equal(fsmv1Metrics))
	})
})

// zeroFields lists, by path, the leaf fields of v that hold their zero value,
// descending into structs and map values.
func zeroFields(v reflect.Value, path string) []string {
	var out []string

	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			out = append(out, zeroFields(v.Field(i), path+"."+v.Type().Field(i).Name)...)
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			out = append(out, zeroFields(iter.Value(), fmt.Sprintf("%s[%v]", path, iter.Key()))...)
		}
	default:
		if v.IsZero() {
			out = append(out, path)
		}
	}

	return out
}
