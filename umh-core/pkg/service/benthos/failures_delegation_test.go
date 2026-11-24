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

// The lookup path of IsMetricsErrorFree: with the flag on the answer comes
// from the fsmv2benthosmetrics.FailureReport lookup, with the flag off nobody asks.

package benthos

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	benthos_monitor_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	fsmv2benthosmetrics "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/benthosmetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// stubFailuresReader is a state reader that returns the observation stored
// under a Benthos S6 name, or ErrNotFound when it holds none for it. A
// non-nil err makes every read fail with it instead.
type stubFailuresReader struct {
	obs map[string]*fsmv2.Observation[simple.Status[fsmv2benthosmetrics.Scrape]]
	err error
}

func (s *stubFailuresReader) LoadObservedTyped(_ context.Context, _, id string, result any) error {
	if s.err != nil {
		return s.err
	}

	obs, ok := s.obs[id]
	if !ok {
		return persistence.ErrNotFound
	}

	out, ok := result.(*fsmv2.Observation[simple.Status[fsmv2benthosmetrics.Scrape]])
	if !ok {
		return nil
	}

	*out = *obs

	return nil
}

func (s *stubFailuresReader) LoadDesiredTyped(_ context.Context, _, _ string, _ any) error {
	return nil
}

// setBenthosMetricsFlag sets USE_FSMV2_BENTHOS_METRICS for one spec and
// restores whatever the environment held before. An empty value unsets it.
func setBenthosMetricsFlag(v string) {
	prior, present := os.LookupEnv(envUseFsmv2BenthosMetrics)
	if v == "" {
		_ = os.Unsetenv(envUseFsmv2BenthosMetrics)
	} else {
		_ = os.Setenv(envUseFsmv2BenthosMetrics, v)
	}

	DeferCleanup(func() {
		if present {
			_ = os.Setenv(envUseFsmv2BenthosMetrics, prior)
		} else {
			_ = os.Unsetenv(envUseFsmv2BenthosMetrics)
		}
	})
}

// stubStoredFailures registers a client whose store holds the observation
// with the given failures, under the given Benthos S6 name, as the last
// sample the worker stored. Setting the returned reader's err makes every
// later read fail.
func stubStoredFailures(s6Name string, failures *benthosmetrics.FailureReport) *stubFailuresReader {
	return stubStoredFailuresAt(s6Name, failures, time.Now())
}

// stubStoredFailuresAt is stubStoredFailures for a sample collected at
// collectedAt. A Failures without its own ObservedAt is stamped with
// collectedAt, as a real scrape stamps it.
func stubStoredFailuresAt(s6Name string, failures *benthosmetrics.FailureReport, collectedAt time.Time) *stubFailuresReader {
	if failures != nil && failures.ObservedAt.IsZero() {
		failures.ObservedAt = collectedAt
	}

	obs := &fsmv2.Observation[simple.Status[fsmv2benthosmetrics.Scrape]]{
		CollectedAt: collectedAt,
		Status: simple.Status[fsmv2benthosmetrics.Scrape]{
			Result: fsmv2benthosmetrics.Scrape{Sample: benthosmetrics.Sample{Failures: failures}},
		},
	}

	reader := &stubFailuresReader{obs: map[string]*fsmv2.Observation[simple.Status[fsmv2benthosmetrics.Scrape]]{
		fsmv2config.ChildID(s6Name): obs,
	}}
	fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader))
	DeferCleanup(func() { fsmv2client.SetClient(nil) })

	return reader
}

// fsmv2BackendService builds a BenthosService wired like the flag-on default:
// the FSMv2 worker manager as its monitor backend, under the given S6 name.
func fsmv2BackendService(s6Name string) *BenthosService {
	s := &BenthosService{
		s6ServiceName:         s6Name,
		benthosMonitorManager: fsmv2benthosmetrics.NewManager("delegation"),
		logger:                logger.For("benthos-failures-delegation-test"),
	}

	return s
}

// outputErrorMetrics returns FSMv1-shaped metrics whose output error total is
// n, and whose processor paths hold no error.
func outputErrorMetrics(n int64) benthos_monitor.BenthosMetrics {
	return benthos_monitor.BenthosMetrics{
		Metrics: benthos_monitor.Metrics{
			Outputs: map[string]benthos_monitor.OutputInstance{
				"root.output": {Error: n},
			},
		},
	}
}

var _ = Describe("The lookup path of IsMetricsErrorFree", func() {
	const name = "benthos-dataflow-delegation"

	// With USE_FSMV2_BENTHOS_METRICS on, IsMetricsErrorFree returns the
	// verdict the Failures lookup reports, even though the cumulative
	// counters hold no error.
	It("asks the Failures lookup with the flag on", func() {
		setBenthosMetricsFlag("true")

		at := time.Now()
		stubStoredFailures(name, &benthosmetrics.FailureReport{
			ObservedAt: at,
			Entries: []benthosmetrics.FailureEntry{{
				Signal: "output_error", Label: "output error", Count: 1,
				LastErrorAt: at.Add(-4 * time.Minute),
			}},
		})

		ok, reason := fsmv2BackendService(name).IsMetricsErrorFree(benthos_monitor.BenthosMetrics{})
		Expect(ok).To(BeFalse(), "the verdict comes from the lookup")
		Expect(reason).To(Equal("benthos reported 1 output error in the last 10m, 4m ago"))
	})

	// With the flag on, an error-free verdict wins over cumulative error
	// counters that hold nine lifetime output errors.
	It("lets empty failures beat lifetime counters with the flag on", func() {
		setBenthosMetricsFlag("true")
		stubStoredFailures(name, &benthosmetrics.FailureReport{})

		ok, reason := fsmv2BackendService(name).IsMetricsErrorFree(outputErrorMetrics(9))
		Expect(ok).To(BeTrue(),
			"the lookup's windowed verdict wins over the lifetime counters")
		Expect(reason).To(BeEmpty())
	})

	// With the flag off, IsMetricsErrorFree reads the cumulative counters.
	It("reads the cumulative counters with the flag off", func() {
		setBenthosMetricsFlag("")

		s := &BenthosService{}

		ok, reason := s.IsMetricsErrorFree(outputErrorMetrics(3))
		Expect(ok).To(BeFalse(), "the cumulative output errors degrade the flow")
		Expect(reason).To(Equal("benthos reported 3 output errors"))

		processorErrors := benthos_monitor.BenthosMetrics{
			Metrics: benthos_monitor.Metrics{
				Process: benthos_monitor.ProcessMetrics{
					Processors: map[string]benthos_monitor.ProcessorMetrics{
						"root.pipeline.processors.0": {Error: 2},
					},
				},
			},
		}
		ok, reason = s.IsMetricsErrorFree(processorErrors)
		Expect(ok).To(BeFalse(), "the cumulative processor errors degrade the flow")
		Expect(reason).To(Equal("benthos reported 2 processor errors"))
	})

	// A caller-supplied FSMv1 monitor backend overrides the flag-selected
	// default, so it answers from the counters and never asks the Failures
	// lookup, even with the flag on and a stuck entry stored: clean counters
	// answer error-free.
	It("does not ask the Failures lookup with an FSMv1 monitor backend", func() {
		setBenthosMetricsFlag("true")
		stubStoredFailures(name, &benthosmetrics.FailureReport{
			Entries: []benthosmetrics.FailureEntry{{
				Signal: "output_error", Label: "output error", Count: 1,
			}},
		})

		fsmv1 := &BenthosService{
			benthosMonitorManager: benthos_monitor_fsm.NewBenthosMonitorManager("delegation-fsmv1"),
		}
		ok, reason := fsmv1.IsMetricsErrorFree(benthos_monitor.BenthosMetrics{})
		Expect(ok).To(BeTrue(), "an FSMv1 backend answers from the counters")
		Expect(reason).To(BeEmpty())
	})

	// With the flag on and no answer — unknown instance, no client, no first
	// sample — IsMetricsErrorFree is not error-free with the reason
	// "no Benthos metrics yet"; it does not fall back to the cumulative
	// counters.
	It("answers not error-free, reason 'no Benthos metrics yet', when the lookup has no answer", func() {
		setBenthosMetricsFlag("true")

		fsmv2client.SetClient(nil)
		DeferCleanup(func() { fsmv2client.SetClient(nil) })

		ok, reason := fsmv2BackendService(name).IsMetricsErrorFree(benthos_monitor.BenthosMetrics{})
		Expect(ok).To(BeFalse(), "no answer fails closed")
		Expect(reason).To(Equal("no Benthos metrics yet"))
	})

	// With the flag on, a failed store read reports the same as no sample
	// at all, even right after a good read.
	It("answers 'no Benthos metrics yet' when the store read fails", func() {
		setBenthosMetricsFlag("true")

		reader := stubStoredFailures(name, &benthosmetrics.FailureReport{})
		ok, reason := fsmv2BackendService(name).IsMetricsErrorFree(benthos_monitor.BenthosMetrics{})
		Expect(ok).To(BeTrue(), "a fresh error-free sample is error-free")
		Expect(reason).To(BeEmpty())

		reader.err = context.DeadlineExceeded
		ok, reason = fsmv2BackendService(name).IsMetricsErrorFree(benthos_monitor.BenthosMetrics{})
		Expect(ok).To(BeFalse(), "a failed store read is not error-free")
		Expect(reason).To(Equal("no Benthos metrics yet"))
	})

	// With the flag on, when scrapes stop, the worker's last sample keeps
	// answering until it is more than ten seconds old. A sample two seconds
	// old still gives its error's reason. An older one gives "no fresh
	// Benthos metrics", so an outage does not keep showing an old error.
	It("answers from the last sample until it is more than ten seconds old", func() {
		setBenthosMetricsFlag("true")

		collectedAt := time.Now().Add(-2 * time.Second)
		stored := &benthosmetrics.FailureReport{
			Entries: []benthosmetrics.FailureEntry{{
				Signal: "output_error", Label: "output error", Count: 1,
				LastErrorAt: collectedAt.Add(-4 * time.Minute),
			}},
		}

		stubStoredFailuresAt(name, stored, collectedAt)
		ok, reason := fsmv2BackendService(name).IsMetricsErrorFree(benthos_monitor.BenthosMetrics{})
		Expect(ok).To(BeFalse(), "a two-second-old sample still answers with its entry")
		Expect(reason).To(Equal("benthos reported 1 output error in the last 10m, 4m ago"),
			"still inside the health freshness rule")

		aged := &benthosmetrics.FailureReport{
			Entries: []benthosmetrics.FailureEntry{{
				Signal: "output_error", Label: "output error", Count: 1,
			}},
		}
		stubStoredFailuresAt(name, aged, time.Now().Add(-2*constants.BenthosMaxMetricsAndConfigAge))
		ok, reason = fsmv2BackendService(name).IsMetricsErrorFree(benthos_monitor.BenthosMetrics{})
		Expect(ok).To(BeFalse(), "an old sample fails closed")
		Expect(reason).To(Equal("no fresh Benthos metrics"),
			"past the health age the last error's reason is gone")
	})

	// With the flag on, error-free Failures older than the freshness rule
	// also answer "no fresh Benthos metrics", not error-free.
	It("answers 'no fresh Benthos metrics' for old error-free Failures too", func() {
		setBenthosMetricsFlag("true")
		stubStoredFailuresAt(name, &benthosmetrics.FailureReport{}, time.Now().Add(-2*constants.BenthosMaxMetricsAndConfigAge))

		ok, reason := fsmv2BackendService(name).IsMetricsErrorFree(benthos_monitor.BenthosMetrics{})
		Expect(ok).To(BeFalse(), "an old sample fails closed")
		Expect(reason).To(Equal("no fresh Benthos metrics"))
	})

	// With the flag on, a sample stamped after now — the wall clock stepped
	// back since it was taken — has no known age, so it is not fresh either.
	It("answers 'no fresh Benthos metrics' for Failures from the future", func() {
		setBenthosMetricsFlag("true")
		stubStoredFailuresAt(name, &benthosmetrics.FailureReport{}, time.Now().Add(time.Hour))

		ok, reason := fsmv2BackendService(name).IsMetricsErrorFree(benthos_monitor.BenthosMetrics{})
		Expect(ok).To(BeFalse(), "a sample from the future fails closed")
		Expect(reason).To(Equal("no fresh Benthos metrics"))
	})
})
