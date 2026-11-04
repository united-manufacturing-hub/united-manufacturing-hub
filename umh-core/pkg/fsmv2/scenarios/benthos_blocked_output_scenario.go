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

// benthosBlockedFakeKey holds the fake Benthos the blocked-output scenario
// drives.
var benthosBlockedFakeKey = config.NewDependencyKey[*fakeBenthos]("benthos-blocked-output.fake")

// blockedReasonPrefix is how ErrorFree's reason starts for a blocked output.
const blockedReasonPrefix = "benthos output blocked: no message sent for "

// outputBlockedFor is how long the output-error entry says the outputs have
// been blocked, 0 when there is no entry.
func outputBlockedFor(obs benthosObservation) time.Duration {
	if e := benthosEntry(obs.Status.Result.Failures, "output_error"); e != nil {
		return e.BlockedFor
	}

	return 0
}

// blockedSeen adds the output-error entry's blocked time to what
// failuresSeen shows.
func blockedSeen(obs benthosObservation) string {
	return fmt.Sprintf("blocked_for=%s %s", outputBlockedFor(obs), failuresSeen(obs))
}

// BenthosBlockedOutputScenario drives the real benthos monitor against one
// fake Benthos that shares the scrape clock the worker reads through
// ClockKey, ten seconds per /metrics read: an output that sends nothing while
// its writes keep failing reads blocked after two minutes, it unblocks once
// sends resume, and an idle output with no failing writes never reads
// blocked.
var BenthosBlockedOutputScenario = scenariorunner.Scenario{
	Name:        "benthos-blocked-output",
	Description: "Benthos blocked output: an output that sends nothing while its writes fail reads blocked after two minutes, unblocks when sends resume, and an idle output never reads blocked",

	Dependencies: func() (map[string]any, func(), error) {
		clk := clock.NewMock()
		fake := newFakeBenthos(clk)

		m := map[string]any{}

		var c clock.Clock = clk
		config.SetDependency(m, fsmv2benthos.ClockKey, c)
		config.SetDependency(m, benthosBlockedFakeKey, fake)

		return m, func() { fake.close() }, nil
	},

	Run: func(ctx context.Context, env scenariorunner.Env) error {
		fake, ok := config.LookupDependency(env.Dependencies, benthosBlockedFakeKey)
		if !ok {
			return errors.New("the benthos-blocked-output scenario's dependency map holds no fake under benthosBlockedFakeKey")
		}

		ref := dynamicchildren.Ref{WorkerType: fsmv2benthos.WorkerType, Name: "benthos-blocked-output"}

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

				return pass(obs)
			})
		}

		env.Step("create the monitor on a Benthos whose output sends everything it receives")
		fake.setTraffic(40, 40, 0)

		spec, err := benthosChildSpec(ref.Name, fake.port())
		if err != nil {
			return err
		}

		if err := env.Client.Upsert(ref, spec); err != nil {
			return err
		}

		if err := waitForSample("the flow is running and error-free",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				ok, _ := flowErrorFree(s.Result.Failures)

				return obs.State == "running" && ok && s.Result.Output.MessagesPerSecond > 0, blockedSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("the output stops sending while every write fails")
		quietFrom := fake.now()
		fake.setTraffic(40, 0, 1)

		if err := waitForSample("the output reads blocked, with the time since its last send in the reason",
			func(obs benthosObservation) (bool, string, error) {
				blocked := outputBlockedFor(obs)
				if blocked > 0 && blocked < benthosmetrics.BlockedAfter {
					return false, "", fmt.Errorf("the output reads blocked after %s, before %s of quiet", blocked, benthosmetrics.BlockedAfter)
				}

				// Three read steps of slack: the worker counts the quiet from
				// the last scrape whose sent counter rose, at or before
				// quietFrom.
				quiet := fake.now().Sub(quietFrom)
				if blocked == 0 && quiet > benthosmetrics.BlockedAfter+3*benthosReadAdvance {
					return false, "", fmt.Errorf("the output still reads not blocked after %s of quiet with failing writes, past %s", quiet, benthosmetrics.BlockedAfter)
				}

				ok, reason := flowErrorFree(obs.Status.Result.Failures)
				done := blocked >= benthosmetrics.BlockedAfter && !ok && strings.HasPrefix(reason, blockedReasonPrefix)

				return done, blockedSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("sends resume and the writes stop failing")
		fake.setTraffic(40, 40, 0)

		if err := waitForSample("the output unblocks; the flow still shows the recent errors, not a blocked output",
			func(obs benthosObservation) (bool, string, error) {
				ok, reason := flowErrorFree(obs.Status.Result.Failures)
				done := outputBlockedFor(obs) == 0 && !ok &&
					strings.HasSuffix(reason, "output errors in the last 10 minutes")

				return done, blockedSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("the output goes idle with no failing write: nothing to send, nothing failing")
		idleFrom := fake.now()
		fake.setTraffic(0, 0, 0)
		// Thirty seconds per read keeps the four-minute wait to eight scrapes.
		fake.setReadAdvance(30 * time.Second)

		return waitForSample("the idle output never reads blocked, well past two minutes of quiet",
			func(obs benthosObservation) (bool, string, error) {
				if blocked := outputBlockedFor(obs); blocked > 0 {
					return false, "", fmt.Errorf("an idle output with no failing write reads blocked for %s", blocked)
				}

				done := fake.now().Sub(idleFrom) >= 2*benthosmetrics.BlockedAfter

				return done, fmt.Sprintf("idle_for=%s %s", fake.now().Sub(idleFrom), blockedSeen(obs)), nil
			})
	},
}
