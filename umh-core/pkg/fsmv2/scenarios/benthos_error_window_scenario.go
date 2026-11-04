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
	"time"

	"github.com/benbjohnson/clock"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/scenariorunner"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	fsmv2benthos "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/benthos"
)

// benthosErrorFakeKey holds the fake Benthos the error-window scenario
// drives, the way the benthos-metrics scenario holds its pair under
// fakeBenthosKey.
var benthosErrorFakeKey = config.NewDependencyKey[*fakeBenthos]("benthos-error-window.fake")

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

// flowErrorFree is f.ErrorFree, except that a sample from a failed scrape,
// whose Failures is nil, is not error-free.
func flowErrorFree(f *benthosmetrics.Failures) (ok bool, reason string) {
	if f == nil {
		return false, "no failures judged: the scrape failed"
	}

	return f.ErrorFree()
}

// failuresSeen adds the flow's reason to what benthosSeen shows, because the
// worker's own degraded flag does not reflect the failures.
func failuresSeen(obs benthosObservation) string {
	_, reason := flowErrorFree(obs.Status.Result.Failures)

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
		fake := newFakeBenthos(clk)

		m := map[string]any{}

		var c clock.Clock = clk
		config.SetDependency(m, fsmv2benthos.ClockKey, c)
		config.SetDependency(m, benthosErrorFakeKey, fake)

		return m, func() { fake.close() }, nil
	},

	Run: func(ctx context.Context, env scenariorunner.Env) error {
		fake, ok := config.LookupDependency(env.Dependencies, benthosErrorFakeKey)
		if !ok {
			return errors.New("the benthos-error-window scenario's dependency map holds no fake under benthosErrorFakeKey")
		}

		ref := dynamicchildren.Ref{WorkerType: fsmv2benthos.WorkerType, Name: "benthos-error-window"}

		waitForSample := func(check string, pass func(obs benthosObservation) (bool, string, error)) error {
			return env.WaitFor(ctx, check, func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[simple.Status[benthosmetrics.Sample]](ctx, env.Client, ref)
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
		fake.setReadAdvance(time.Second)
		fake.setTraffic(40, 20, 0)

		spec, err := benthosChildSpec(ref.Name, fake.port())
		if err != nil {
			return err
		}

		if err := env.Client.Upsert(ref, spec); err != nil {
			return err
		}

		if err := waitForSample("the first sample seeds the flow error-free",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				ok, _ := flowErrorFree(s.Result.Failures)
				done := obs.State == "running" && !s.Degraded && ok

				return done, failuresSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("five output errors burst in between two scrapes")
		fake.setTraffic(40, 20, 5)

		if err := waitForSample("the flow degrades on the burst: one entry with the burst's count and its reason",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				e := benthosEntry(s.Result.Failures, "output_error")
				if e == nil {
					return false, failuresSeen(obs), nil
				}

				ok, reason := flowErrorFree(s.Result.Failures)
				want := fmt.Sprintf("benthos reported %d output errors in the last 10 minutes", e.Count)
				if !ok && reason != want {
					return false, "", fmt.Errorf("the entry's reason is %q, want %q", reason, want)
				}

				// The entry's count is the whole burst: the window reaches
				// back to the error-free seed, so the count is the error
				// counter the same scrape read.
				done := !ok && e.Count >= 5 && e.Count == s.Result.Metrics.Outputs["root.output"].Error

				return done, failuresSeen(obs), nil
			}); err != nil {
			return err
		}

		// The error counter stays flat from here: no further error, no
		// restart, until the window has let the burst go.
		var lastErrorAt time.Time
		var inputAtBurst int64

		env.Step("the error counter stays flat; the clock steps thirty seconds per scrape while the window runs out")
		fake.setTraffic(40, 20, 0)
		fake.setReadAdvance(30 * time.Second)

		if err := waitForSample("the entry still holds the flat counter, thirty seconds short of ten minutes after the last error",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				e := benthosEntry(s.Result.Failures, "output_error")

				if e != nil && e.LastErrorAt.After(lastErrorAt) {
					lastErrorAt = e.LastErrorAt
				}

				if inputAtBurst == 0 {
					inputAtBurst = s.Result.Metrics.InputReceivedTotal()
				}

				// A clear more than 45 s, one and a half read steps, before
				// the window ends is the bug this scenario pins, not a state
				// to wait out.
				if e == nil && fake.now().Before(lastErrorAt.Add(benthosmetrics.ErrorWindow-45*time.Second)) {
					return false, "", fmt.Errorf("the entry cleared %v before the ten-minute window ended (last error at %s, now %s)",
						benthosmetrics.ErrorWindow-fake.now().Sub(lastErrorAt), lastErrorAt, fake.now())
				}

				if e == nil {
					return false, failuresSeen(obs), nil
				}

				done := fake.now().After(lastErrorAt.Add(benthosmetrics.ErrorWindow - 45*time.Second))

				return done, fmt.Sprintf("now=%s last_error=%s %s",
					fake.now().Format(time.TimeOnly), lastErrorAt.Format(time.TimeOnly), failuresSeen(obs)), nil
			}); err != nil {
			return err
		}

		if err := waitForSample("the flow is error-free again ten minutes after its last error, with no restart",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				e := benthosEntry(s.Result.Failures, "output_error")

				// The window lets the entry go at ten minutes: still present
				// past that is the bug this scenario pins, not a state to wait
				// out.
				if e != nil && fake.now().After(lastErrorAt.Add(benthosmetrics.ErrorWindow+45*time.Second)) {
					return false, "", fmt.Errorf("the entry is still present %v after the last error, past the %v window",
						fake.now().Sub(lastErrorAt).Round(time.Second), benthosmetrics.ErrorWindow)
				}

				if e != nil {
					return false, failuresSeen(obs), nil
				}

				ok, _ := flowErrorFree(s.Result.Failures)

				// No restart intervened: the same Benthos answered every
				// scrape, and its input counter only ever rose.
				done := ok && s.Result.Version == fakeBenthosVersion &&
					s.Result.Metrics.InputReceivedTotal() >= inputAtBurst

				return done, failuresSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("output errors come back; then the Benthos restarts: its counters drop to zero and start over")
		fake.setTraffic(40, 20, 3)

		var inputBeforeRestart int64

		if err := waitForSample("the flow degrades again on the new errors",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				inputBeforeRestart = s.Result.Metrics.InputReceivedTotal()
				e := benthosEntry(s.Result.Failures, "output_error")
				ok, _ := flowErrorFree(s.Result.Failures)

				return !ok && e != nil, failuresSeen(obs), nil
			}); err != nil {
			return err
		}

		fake.setCounters(0, 0, 0)
		fake.setTraffic(40, 20, 0)

		if err := waitForSample("the restart clears the errors from before it at once, and the counters start over",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				ok, _ := flowErrorFree(s.Result.Failures)
				done := ok &&
					s.Result.Metrics.Outputs["root.output"].Error == 0 &&
					s.Result.Metrics.InputReceivedTotal() < inputBeforeRestart

				return done, failuresSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("the Benthos's input loses its connection and its traffic with it")
		fake.setInputConnected(false)
		fake.setTraffic(0, 20, 0)

		if err := waitForSample("the flow shows the input outage: a connection entry with its reason",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				e := benthosEntry(s.Result.Failures, "input_connection")
				if e == nil {
					return false, failuresSeen(obs), nil
				}

				ok, reason := flowErrorFree(s.Result.Failures)
				if !ok && reason != "benthos reports 1 input not connected" {
					return false, "", fmt.Errorf("the connection entry's reason is %q", reason)
				}

				done := !ok && e.IsConnection() && e.Count == 1

				return done, failuresSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("the input reconnects and its traffic returns")
		fake.setInputConnected(true)
		fake.setTraffic(40, 20, 0)

		return waitForSample("the flow is error-free again on the reconnected input",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				ok, _ := flowErrorFree(s.Result.Failures)

				return ok, failuresSeen(obs), nil
			})
	},
}
