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

// What the not-cleared count counts, and what it leaves out.

package generator

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	fsmv2benthos "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/benthos"
)

// stubNotClearedReader is a state reader that returns the observation stored
// under a Benthos S6 name, or ErrNotFound when it holds none for it.
type stubNotClearedReader struct {
	obs map[string]*fsmv2.Observation[simple.Status[benthosmetrics.Sample]]
}

func (s *stubNotClearedReader) LoadObservedTyped(_ context.Context, _, id string, result any) error {
	obs, ok := s.obs[id]
	if !ok {
		return persistence.ErrNotFound
	}

	out, ok := result.(*fsmv2.Observation[simple.Status[benthosmetrics.Sample]])
	if !ok {
		return nil
	}
	*out = *obs

	return nil
}

func (s *stubNotClearedReader) LoadDesiredTyped(_ context.Context, _, _ string, _ any) error {
	return nil
}

// setBenthosMetricsFlag sets USE_FSMV2_BENTHOS_METRICS for one spec and
// restores whatever the environment held before. An empty value unsets it.
func setBenthosMetricsFlag(v string) {
	prior, present := os.LookupEnv("USE_FSMV2_BENTHOS_METRICS")
	if v == "" {
		_ = os.Unsetenv("USE_FSMV2_BENTHOS_METRICS")
	} else {
		_ = os.Setenv("USE_FSMV2_BENTHOS_METRICS", v)
	}

	DeferCleanup(func() {
		if present {
			_ = os.Setenv("USE_FSMV2_BENTHOS_METRICS", prior)
		} else {
			_ = os.Unsetenv("USE_FSMV2_BENTHOS_METRICS")
		}
	})
}

// registerBenthosWorkers registers a client whose registry holds one
// "benthos_metrics" worker per S6 name, and whose store holds the given
// Failures as each worker's stored sample. A nil Failures stores no sample
// for that name.
func registerBenthosWorkers(failures map[string]*benthosmetrics.Failures) {
	obs := map[string]*fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{}
	for s6Name, f := range failures {
		if f == nil {
			continue
		}
		obs[fsmv2config.ChildID(s6Name)] = &fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{
			CollectedAt: time.Now(),
			Status:      simple.Status[benthosmetrics.Sample]{Result: benthosmetrics.Sample{Failures: f}},
		}
	}

	c := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), &stubNotClearedReader{obs: obs})
	for s6Name := range failures {
		Expect(c.Upsert(dynamicchildren.Ref{
			WorkerType: fsmv2benthos.WorkerType,
			Name:       s6Name,
		}, map[string]any{})).To(Succeed(), "register %s", s6Name)
	}

	fsmv2client.SetClient(c)
	DeferCleanup(func() { fsmv2client.SetClient(nil) })
}

// stuckOutputError is the stored Failures of a Benthos instance whose
// output_error entry did not clear: its last error is the error window plus
// the margin, and one more minute, behind the scrape.
func stuckOutputError(at time.Time) *benthosmetrics.Failures {
	lastError := at.Add(-benthosmetrics.ErrorWindow).Add(-notClearedMargin).Add(-time.Minute)

	return &benthosmetrics.Failures{
		ObservedAt: at,
		Entries: []benthosmetrics.FailureEntry{{
			Signal:      "output_error",
			Label:       "output error",
			Count:       1,
			Since:       lastError,
			LastErrorAt: lastError,
		}},
	}
}

// dfcBenthosName is the S6 name of the Benthos instance behind the DFC the
// specs register: "benthos-dataflow-fixture-dfc".
const dfcBenthosName = "benthos-dataflow-fixture-dfc"

var _ = Describe("The not-cleared count", func() {
	// The count names every registered Benthos worker whose failures hold an
	// error entry with no new error for more than the error window plus
	// notClearedMargin.
	It("counts each registered Benthos whose errors did not clear in time", func() {
		setBenthosMetricsFlag("true")
		now := time.Now()

		registerBenthosWorkers(map[string]*benthosmetrics.Failures{
			dfcBenthosName: stuckOutputError(now),
		})
		Expect(benthosErrorsNotClearedInstanceCount(context.Background())).To(Equal(1),
			"an entry stuck past its window counts")

		// A last error inside the window plus margin does not count: this
		// entry cleared 30 s late, within notClearedMargin.
		recentError := now.Add(-benthosmetrics.ErrorWindow - 30*time.Second)
		registerBenthosWorkers(map[string]*benthosmetrics.Failures{
			dfcBenthosName: {
				ObservedAt: now,
				Entries: []benthosmetrics.FailureEntry{{
					Signal:      "output_error",
					Label:       "output error",
					Count:       1,
					Since:       recentError,
					LastErrorAt: recentError,
				}},
			},
		})
		Expect(benthosErrorsNotClearedInstanceCount(context.Background())).To(BeZero(),
			"an entry still inside its window plus margin does not count")
	})

	// A connection entry never counts, because it releases only on a
	// reconnect.
	It("does not count a connection that is still down", func() {
		setBenthosMetricsFlag("true")
		now := time.Now()

		registerBenthosWorkers(map[string]*benthosmetrics.Failures{
			dfcBenthosName: {
				ObservedAt: now,
				Entries: []benthosmetrics.FailureEntry{{
					Signal:     "input_connection",
					Label:      "input",
					Count:      1,
					Since:      now.Add(-12 * time.Minute),
					Connection: true,
				}},
			},
		})
		Expect(benthosErrorsNotClearedInstanceCount(context.Background())).To(BeZero(),
			"a connection that is still down does not count")
	})

	// The count stays 0 when no lookup answers, the expected steady state
	// of the status message.
	It("is zero when no lookup answers", func() {
		setBenthosMetricsFlag("true")

		registerBenthosWorkers(map[string]*benthosmetrics.Failures{dfcBenthosName: nil})
		Expect(benthosErrorsNotClearedInstanceCount(context.Background())).To(BeZero(),
			"no stored sample answers nothing")
	})

	// The count is 0 with no registered workers at all.
	It("is zero with no registered Benthos workers", func() {
		setBenthosMetricsFlag("true")

		registerBenthosWorkers(map[string]*benthosmetrics.Failures{})
		Expect(benthosErrorsNotClearedInstanceCount(context.Background())).To(BeZero(),
			"a registry with no Benthos workers counts nothing")
	})

	// The red test of this count: with the flag on and a stuck instance in
	// the registry, the count is greater than 0, so a count of 0 cannot
	// silently measure nothing — and with the flag off the count stays 0.
	It("is greater than zero with the flag on and a stuck instance", func() {
		registerBenthosWorkers(map[string]*benthosmetrics.Failures{
			dfcBenthosName: stuckOutputError(time.Now()),
		})

		setBenthosMetricsFlag("")
		Expect(benthosErrorsNotClearedInstanceCount(context.Background())).To(BeZero(),
			"with the flag off nobody asks the lookup")

		setBenthosMetricsFlag("true")
		Expect(benthosErrorsNotClearedInstanceCount(context.Background())).To(BeNumerically(">", 0),
			"with the flag on and a stuck instance the count is greater than zero")
	})

	// No client: the count is 0 instead of panicking.
	It("is zero with no client", func() {
		setBenthosMetricsFlag("true")
		fsmv2client.SetClient(nil)
		DeferCleanup(func() { fsmv2client.SetClient(nil) })

		Expect(benthosErrorsNotClearedInstanceCount(context.Background())).To(BeZero())
	})
})
