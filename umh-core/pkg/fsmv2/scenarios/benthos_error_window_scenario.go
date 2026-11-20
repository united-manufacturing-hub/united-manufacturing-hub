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

package scenarios

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/scenariorunner"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics/benthosmetricstest"
	fsmv2benthosmetrics "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/benthosmetrics"
)

// benthosErrorFakeKey holds the fake Benthos the error-window scenario
// drives, the way the benthos-metrics scenario holds its pair under
// fakeBenthosKey.
var benthosErrorFakeKey = config.NewDependencyKey[*errorWindowFake]("benthos-error-window.fake")

// errorWindowFake is a fake Benthos whose every /metrics read advances the
// shared scrape clock by a step the scenario sets with setReadAdvance.
type errorWindowFake struct {
	*benthosmetricstest.Server

	advance atomic.Int64
}

// newErrorWindowFake starts a fake whose /metrics reads advance clk by one
// second each until setReadAdvance changes the step.
func newErrorWindowFake(clk *clock.Mock) *errorWindowFake {
	f := &errorWindowFake{Server: benthosmetricstest.NewServer()}
	f.setReadAdvance(time.Second)
	f.OnMetricsRead(func() { clk.Add(time.Duration(f.advance.Load())) })

	return f
}

// setReadAdvance sets how much scrape time one /metrics read advances the
// shared clock. Waiting out the ten-minute window in bigger strides costs one
// real second per stride instead of one per scrape second.
func (f *errorWindowFake) setReadAdvance(d time.Duration) {
	f.advance.Store(int64(d))
}

// benthosEntry returns f's entry named signal, or nil when f holds none.
func benthosEntry(f *benthosmetrics.Failures, signal string) *benthosmetrics.FailureEntry {
	if f == nil {
		return nil
	}

	for i := range f.Entries {
		if f.Entries[i].Signal == signal {
			return &f.Entries[i]
		}
	}

	return nil
}

// flowErrorFree is f.ErrorFree, except that a zero Sample, stored before
// the first successful scrape and whose Failures is nil, is not error-free.
func flowErrorFree(f *benthosmetrics.Failures) (ok bool, reason string) {
	if f == nil {
		return false, "no failures judged: no scrape succeeded yet"
	}

	return f.ErrorFree()
}

// failuresSeen adds the flow's reason to what benthosSeen shows, because the
// worker's own degraded flag does not reflect the failures.
func failuresSeen(obs benthosObservation) string {
	_, reason := flowErrorFree(obs.Status.Result.Sample.Failures)

	return fmt.Sprintf("flow_reason=%q %s", reason, benthosSeen(obs))
}

// BenthosErrorWindowScenario drives the real benthos monitor against one fake
// Benthos that shares the scrape clock the worker reads through ClockKey: a
// burst of output errors degrades the flow until ten minutes of scrape time
// pass without another error, a restart clears the errors from before it at
// once, and an input outage shows until Benthos reconnects. Each /metrics
// read advances the clock by a step the scenario sets, so the ten-minute
// window costs seconds of real time, not ten minutes.
var BenthosErrorWindowScenario = scenariorunner.Scenario{
	Name:        "benthos-error-window",
	Description: "Benthos error window: an error burst degrades a flow until ten minutes after the last error, a restart clears the errors from before it, and an input outage shows until Benthos reconnects",

	Dependencies: func() (map[string]any, func(), error) {
		clk := clock.NewMock()
		fake := newErrorWindowFake(clk)

		m := map[string]any{}

		var c clock.Clock = clk
		config.SetDependency(m, fsmv2benthosmetrics.ClockKey, c)
		config.SetDependency(m, benthosErrorFakeKey, fake)

		return m, fake.Close, nil
	},

	Run: func(ctx context.Context, env scenariorunner.Env) error {
		fake, ok := config.LookupDependency(env.Dependencies, benthosErrorFakeKey)
		if !ok {
			return errors.New("the benthos-error-window scenario's dependency map holds no fake under benthosErrorFakeKey")
		}

		ref := dynamicchildren.Ref{WorkerType: fsmv2benthosmetrics.WorkerType, Name: "benthos-error-window"}

		waitForSample := func(check string, pass func(obs benthosObservation) (bool, string, error)) error {
			return env.WaitFor(ctx, check, func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[simple.Status[fsmv2benthosmetrics.Scrape]](ctx, env.Client, ref)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotFound) {
						return false, "the monitor has not published an observation yet", nil
					}

					if errors.Is(err, fsmv2client.ErrWorkerDeleted) {
						return false, "the monitor is deleted", nil
					}

					return false, "", err
				}

				done, seen, err := pass(obs)
				if err != nil {
					return false, "", err
				}

				return done, seen, nil
			})
		}

		env.Step("create the monitor on a Benthos whose input flows without an error")
		fake.SetTraffic(40, 20, 0)

		if err := env.Client.Upsert(ref, benthosChildSpec(fake.Port())); err != nil {
			return err
		}

		if err := waitForSample("the first sample seeds the flow error-free",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				ok, _ := flowErrorFree(s.Result.Sample.Failures)
				done := obs.State == "running" && !s.Degraded && ok

				return done, failuresSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("five output errors burst in between two scrapes")
		fake.SetTraffic(40, 20, 5)

		if err := waitForSample("the flow degrades on the burst: one entry with the burst's count and its reason",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status

				e := benthosEntry(s.Result.Sample.Failures, "output_error")
				if e == nil {
					return false, failuresSeen(obs), nil
				}

				// The elapsed tail ("just now", "4m ago") moves with the
				// scrape this check happens to see, so pin everything up to
				// it: the count, the plural, the window as the user reads it
				// and the "last one" clause. pkg/benthosmetrics pins the
				// elapsed wording itself on a controlled clock.
				ok, reason := flowErrorFree(s.Result.Sample.Failures)

				want := fmt.Sprintf("benthos reported %d output errors in the last 10m, last one ", e.Count)
				if !ok && !strings.HasPrefix(reason, want) {
					return false, "", fmt.Errorf("the entry's reason is %q, want it to start %q", reason, want)
				}

				// The entry's count is the whole burst: the window reaches
				// back to the error-free seed, so the count is the error
				// counter the same scrape read.
				done := !ok && e.Count >= 5 && e.Count == s.Result.Sample.Metrics.Outputs["root.output"].Error

				return done, failuresSeen(obs), nil
			}); err != nil {
			return err
		}

		// The error counter stays flat from here: no further error, no
		// restart, until the window has let the burst go.
		var (
			lastErrorAt  time.Time
			inputAtBurst int64
		)

		env.Step("the error counter stays flat; the clock steps thirty seconds per scrape while the window runs out")
		fake.SetTraffic(40, 20, 0)
		fake.setReadAdvance(30 * time.Second)

		if err := waitForSample("the entry still holds the flat counter, thirty seconds short of ten minutes after the last error",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				e := benthosEntry(s.Result.Sample.Failures, "output_error")

				if e != nil && e.LastErrorAt.After(lastErrorAt) {
					lastErrorAt = e.LastErrorAt
				}

				if inputAtBurst == 0 {
					inputAtBurst = s.Result.Sample.Metrics.InputReceivedTotal()
				}

				// Judge the entry against its own scrape, not the clock as it
				// reads now: this callback can be handed a sample taken before
				// later scrapes advanced the clock.
				seenAt := s.Result.Sample.At

				// A clear more than 45 s, one and a half read steps, before
				// the window ends is the bug this scenario pins, not a state
				// to wait out.
				if e == nil && seenAt.Before(lastErrorAt.Add(benthosmetrics.ErrorWindow-45*time.Second)) {
					return false, "", fmt.Errorf("the entry cleared %v before the ten-minute window ended (last error at %s, scrape at %s)",
						benthosmetrics.ErrorWindow-seenAt.Sub(lastErrorAt), lastErrorAt, seenAt)
				}

				if e == nil {
					return false, failuresSeen(obs), nil
				}

				done := seenAt.After(lastErrorAt.Add(benthosmetrics.ErrorWindow - 45*time.Second))

				return done, fmt.Sprintf("scrape=%s last_error=%s %s",
					seenAt.Format(time.TimeOnly), lastErrorAt.Format(time.TimeOnly), failuresSeen(obs)), nil
			}); err != nil {
			return err
		}

		if err := waitForSample("the flow is error-free again ten minutes after its last error, with no restart",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				e := benthosEntry(s.Result.Sample.Failures, "output_error")

				// The window lets the entry go at ten minutes: still present
				// past that is the bug this scenario pins, not a state to wait
				// out. Judged against the entry's own scrape, as above.
				seenAt := s.Result.Sample.At
				if e != nil && seenAt.After(lastErrorAt.Add(benthosmetrics.ErrorWindow+45*time.Second)) {
					return false, "", fmt.Errorf("the entry is still present %v after the last error, past the %v window",
						seenAt.Sub(lastErrorAt).Round(time.Second), benthosmetrics.ErrorWindow)
				}

				if e != nil {
					return false, failuresSeen(obs), nil
				}

				ok, _ := flowErrorFree(s.Result.Sample.Failures)

				// No restart intervened: the same Benthos answered every
				// scrape, and its input counter only ever rose.
				done := ok && s.Result.Probes.Version == benthosmetricstest.Version &&
					s.Result.Sample.Metrics.InputReceivedTotal() >= inputAtBurst

				return done, failuresSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("output errors come back; then the Benthos restarts: its counters drop to zero and start over")
		fake.SetTraffic(40, 20, 3)

		var inputBeforeRestart int64

		if err := waitForSample("the flow degrades again on the new errors",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				inputBeforeRestart = s.Result.Sample.Metrics.InputReceivedTotal()
				e := benthosEntry(s.Result.Sample.Failures, "output_error")
				ok, _ := flowErrorFree(s.Result.Sample.Failures)

				return !ok && e != nil, failuresSeen(obs), nil
			}); err != nil {
			return err
		}

		// Traffic stops before the counters reset: a read that raced the
		// restart would otherwise add the still-configured error increment
		// to the reset counter.
		fake.SetTraffic(40, 20, 0)
		fake.SetCounters(0, 0, 0)

		if err := waitForSample("the restart clears the errors from before it at once, and the counters start over",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				ok, _ := flowErrorFree(s.Result.Sample.Failures)
				done := ok &&
					s.Result.Sample.Metrics.Outputs["root.output"].Error == 0 &&
					s.Result.Sample.Metrics.InputReceivedTotal() < inputBeforeRestart

				return done, failuresSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("the Benthos's input loses its connection and its traffic with it")
		fake.SetInputConnected(false)
		fake.SetTraffic(0, 20, 0)

		if err := waitForSample("the flow shows the input outage: a connection entry with its reason",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status

				e := benthosEntry(s.Result.Sample.Failures, "input_connection")
				if e == nil {
					return false, failuresSeen(obs), nil
				}

				ok, reason := flowErrorFree(s.Result.Sample.Failures)
				if !ok && reason != "benthos reports 1 input not connected" {
					return false, "", fmt.Errorf("the connection entry's reason is %q", reason)
				}

				done := !ok && e.Connection && e.Count == 1

				return done, failuresSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("the input reconnects and its traffic returns")
		fake.SetInputConnected(true)
		fake.SetTraffic(40, 20, 0)

		return waitForSample("the flow is error-free again on the reconnected input",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				ok, _ := flowErrorFree(s.Result.Sample.Failures)

				return ok, failuresSeen(obs), nil
			})
	},
}
