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

package fsmv2cpu

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// stubSampler is the mock injection seam: a Poll reads through whatever Sampler
// the deps hold, so a test supplies its own instead of the real cgroup sampler.
type stubSampler struct {
	read func(ctx context.Context) (cpuhealth.Sample, error)
}

func (s stubSampler) Read(ctx context.Context) (cpuhealth.Sample, error) {
	return s.read(ctx)
}

// newDeps builds a CPUDeps with a stub sampler and a real engine + table, so a
// test can drive Poll without the startup snapshot (which reads a real cgroup).
func newDeps(s cpuhealth.Sampler, cores, quota float64) *CPUDeps {
	table := cpuhealth.Table(cores, quota)
	engine, err := diagnosis.NewEngine(table)
	Expect(err).NotTo(HaveOccurred(), "the test table must be buildable")

	return &CPUDeps{
		BaseDependencies: deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, deps.Identity{ID: "cpu-test", WorkerType: WorkerType}),
		sampler:          s,
		engine:           engine,
		table:            table,
		firstFilled:      make(map[string]bool),
	}
}

var _ = Describe("CPU monitor worker", func() {
	Describe("R1 — polls and reports", func() {
		It("registers a simple monitor worker with a declared observation interval", func() {
			iv, ok := fsmv2.ObservationIntervalFor(WorkerType)
			Expect(ok).To(BeTrue(), "init() must call simple.Register, which records the observation interval")
			Expect(iv).To(Equal(pollInterval))

			Expect(fsmv2.LookupInitialState(WorkerType)).NotTo(BeNil(),
				"Register records an initial state for the worker type")
		})

		It("keeps the sampler's baselines in per-instance deps behind a pointer, so a poll's mutations survive the tick", func() {
			d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				return cpuhealth.Sample{Timestamp: time.Now()}, nil
			}}, 4, 2)

			// Two ticks. Poll takes d by value; because TDeps is *CPUDeps, the
			// value is the pointer and the second tick sees the first's
			// mutation. If TDeps were the value CPUDeps, the non-pointer `polls`
			// field would be copied and reset to zero each tick.
			_, err := Poll(context.Background(), d, CPUConfig{})
			Expect(err).NotTo(HaveOccurred())
			Expect(d.polls).To(Equal(uint64(1)), "first tick increments the deps' poll counter")

			_, err = Poll(context.Background(), d, CPUConfig{})
			Expect(err).NotTo(HaveOccurred())
			Expect(d.polls).To(Equal(uint64(2)),
				"second tick sees the first's mutation — a value-TDeps refactor would show 1")
		})

		It("reports a verdict from Decide rather than a raw measurement", func() {
			d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				return cpuhealth.Sample{
					Timestamp: time.Now(),
					Quota:     diagnosis.Known(2),
					// All signals present and quiet: nothing fires.
					NrPeriods:   diagnosis.Known(1),
					NrThrottled: diagnosis.Known(0),
					UsageUsec:   diagnosis.Known(5000000),
					Pressure:    diagnosis.Known(0),
					Steal:       diagnosis.Known(0),
					HostBusy:    diagnosis.Known(0.5),
					Virtualized: false,
				}, nil
			}}, 4, 2)

			status, err := Poll(context.Background(), d, CPUConfig{})
			Expect(err).NotTo(HaveOccurred())
			Expect(status.Polls).To(Equal(uint64(1)))

			// The verdict is a state from Decide, not a raw Sample reading such
			// as usage cores. A quiet, present tick judges healthy.
			Expect(status.Verdict).To(Equal(string(cpuhealth.StateHealthy)))
			Expect(status.Message).NotTo(BeEmpty(), "the status carries the composed customer message")
		})

		It("reports a degraded verdict, not a healthy one, when Decide judges the cgroup degraded", func() {
			// A hostile sample — one whose signal fires above the mark — must
			// surface as a degraded verdict. Without this arm, "report a verdict
			// from Decide rather than a raw measurement" is satisfied by
			// Decide-on-nothing, because an all-absent sample also judges healthy
			// on tick 1. A degraded arm proves the reported state reflects Decide,
			// not the absence of an engine crash.
			d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				return cpuhealth.Sample{
					Timestamp: time.Now(),
					Quota:     diagnosis.Known(0),
					NrPeriods: diagnosis.Known(1),
					// Pressure fires above the mark on the FIRST sample, so a
					// no-quota tick has a degraded verdict without window warm-up.
					Pressure:    diagnosis.Known(0.9),
					HostBusy:    diagnosis.Known(0.5),
					Virtualized: true,
				}, nil
			}}, 4, 0)

			status, err := Poll(context.Background(), d, CPUConfig{})
			Expect(err).NotTo(HaveOccurred())
			Expect(status.Verdict).To(Equal(string(cpuhealth.StateDegraded)),
				"a hostile sample must judge degraded, proving the verdict reflects Decide")
			Expect(status.Message).NotTo(BeEmpty())
		})

		It("reports it could not measure when the engine failed to build, rather than panicking on a nil engine", func() {
			// NewDeps cannot fail, so a table that will not build is surfaced
			// through Poll: the engine stays nil, engineErr is set, and Poll
			// returns the error. Without this guard, Decide on a nil engine
			// panics at the supervisor (simple has no recover around Poll).
			d := &CPUDeps{
				BaseDependencies: deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, deps.Identity{ID: "cpu-test", WorkerType: WorkerType}),
				sampler: stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
					return cpuhealth.Sample{Timestamp: time.Now()}, nil
				}},
				engineErr: errors.New("cpu table will not build"),
			}

			status, err := Poll(context.Background(), d, CPUConfig{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cpu table will not build"))
			Expect(status.Verdict).To(BeEmpty(), "no verdict is fabricated from an unbuilt engine")
		})
	})
})
