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
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/scenariorunner"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	pkgconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	fsmv2benthos "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/benthos"
)

// benthosStaleAfter is the observation age the framework calls stale for this
// worker: three times its registered scrape interval, the threshold the FSMv1
// adapter reads through (adapter.staleAfterFor).
const benthosStaleAfter = 3 * benthosmetrics.ScrapeInterval

// fakeBenthosKey holds the two fake Benthos servers the scenario drives, so
// Run can change their answers, the way nmap's Run reads its mockDialer back
// out of the dependency map.
var fakeBenthosKey = config.NewDependencyKey[*benthosFakes]("benthos-metrics.fakes")

// benthosFakes is the pair of fake Benthos instances the scenario runs: the
// first monitor's target and the second's.
type benthosFakes struct {
	first, second *fakeBenthos
}

type benthosObservation = fsmv2.Observation[simple.Status[fsmv2benthos.Scrape]]

// benthosChildSpec renders the child-spec map the adapter's cfgFor produces:
// the config as YAML with the yaml-tag keys, which is what the worker
// unmarshals back into a config.BenthosMonitorConfig.
func benthosChildSpec(name string, port uint16) (map[string]any, error) {
	raw, err := yaml.Marshal(pkgconfig.BenthosMonitorConfig{
		FSMInstanceConfig: pkgconfig.FSMInstanceConfig{Name: name, DesiredFSMState: "active"},
		MetricsPort:       port,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal benthos monitor config: %w", err)
	}

	spec := map[string]any{}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("unmarshal benthos monitor config to map: %w", err)
	}

	return spec, nil
}

// benthosSeen renders the parts of an observation a wait names.
func benthosSeen(obs benthosObservation) string {
	s := obs.Status

	return fmt.Sprintf("state=%s degraded=%t reason=%q input_rate=%.2f output_rate=%.2f active=%t input=%d errors=%d version=%q",
		obs.State, s.Degraded, s.Reason,
		inputRate(s), outputRate(s),
		s.Result.Sample.IsActive,
		s.Result.Sample.Metrics.InputReceivedTotal(),
		s.Result.Sample.Metrics.Outputs["root.output"].Error,
		s.Result.Probes.Version)
}

// BenthosMetricsScenario drives the real benthos monitor against two fake
// Benthos instances that share the scrape clock the worker reads through
// ClockKey: each /metrics read advances it once, so a wait asserts on the
// rates the counters carry, not on real time.
var BenthosMetricsScenario = scenariorunner.Scenario{
	Name:        "benthos-metrics",
	Description: "Benthos metrics monitor against two fakes: traffic gives each instance its own rate, a restart folds its counters, a dead endpoint degrades the store",

	Dependencies: func() (map[string]any, func(), error) {
		clk := clock.NewMock()
		fakes := &benthosFakes{first: newFakeBenthos(clk), second: newFakeBenthos(clk)}

		m := map[string]any{}

		var c clock.Clock = clk
		config.SetDependency(m, fsmv2benthos.ClockKey, c)
		config.SetDependency(m, fakeBenthosKey, fakes)

		return m, func() {
			fakes.first.close()
			fakes.second.close()
		}, nil
	},

	Run: func(ctx context.Context, env scenariorunner.Env) error {
		fakes, ok := config.LookupDependency(env.Dependencies, fakeBenthosKey)
		if !ok {
			return errors.New("the benthos-metrics scenario's dependency map holds no fakes under fakeBenthosKey")
		}

		first, second := fakes.first, fakes.second

		refFirst := dynamicchildren.Ref{WorkerType: fsmv2benthos.WorkerType, Name: "benthos-first"}
		refSecond := dynamicchildren.Ref{WorkerType: fsmv2benthos.WorkerType, Name: "benthos-second"}

		upsert := func(ref dynamicchildren.Ref, target *fakeBenthos) error {
			spec, err := benthosChildSpec(ref.Name, target.port())
			if err != nil {
				return err
			}

			return env.Client.Upsert(ref, spec)
		}

		waitForSample := func(ref dynamicchildren.Ref, check string, pass func(obs benthosObservation) (bool, string, error)) error {
			return env.WaitFor(ctx, check, func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[simple.Status[fsmv2benthos.Scrape]](ctx, env.Client, ref)
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

		env.Step("create the first monitor on a Benthos that receives 40 messages per scrape")
		first.setTraffic(40, 20, 0)

		if err := upsert(refFirst, first); err != nil {
			return err
		}

		if err := waitForSample(refFirst, "the store shows the input rate of 4 msg/s, the Benthos active and the monitor running",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				// 4 = 40 messages per read ÷ benthosReadAdvance. Exact only
				// while no other fake is scraped: every fake's read advances
				// the shared clock.
				done := obs.State == "running" && !s.Degraded &&
					inputRate(s) == 4 && s.Result.Sample.IsActive &&
					s.Result.Probes.PingAlive && s.Result.Probes.Ready && s.Result.Probes.Version == fakeBenthosVersion

				return done, benthosSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("stop the first Benthos's input traffic while its output keeps draining; start a second Benthos on its own fake, with lighter traffic and output errors")
		first.setTraffic(0, 20, 0)
		second.setTraffic(10, 5, 1)

		if err := upsert(refSecond, second); err != nil {
			return err
		}

		if err := waitForSample(refSecond, "the second monitor reads its own rate, active, with its own error counter",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				done := inputRate(s) > 0 && s.Result.Sample.IsActive &&
					s.Result.Sample.Metrics.Outputs["root.output"].Error > 0

				return done, benthosSeen(obs), nil
			}); err != nil {
			return err
		}

		if err := waitForSample(refFirst, "the input-idle first monitor reads 0 msg/s and inactive, its output draining",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				done := inputRate(s) == 0 && !s.Result.Sample.IsActive &&
					outputRate(s) > 0

				return done, benthosSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("the second Benthos restarts: its counters drop to zero and its traffic stops")
		// Traffic stops before the counters drop, so a scrape between the two
		// cannot raise the reset counters again.
		second.setTraffic(0, 0, 0)
		second.setCounters(0, 0, 0)

		if err := waitForSample(refSecond, "the restarted Benthos reads 0 msg/s and inactive, its counters at zero and its rate never negative",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				if inputRate(s) < 0 {
					return false, "", fmt.Errorf("the rate after the counter drop is negative: %.2f msg/s",
						inputRate(s))
				}

				done := inputRate(s) == 0 && !s.Result.Sample.IsActive &&
					s.Result.Sample.Metrics.InputReceivedTotal() == 0

				return done, benthosSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("traffic returns on the restarted Benthos")
		second.setTraffic(10, 5, 1)

		if err := waitForSample(refSecond, "the rate recovers to the restarted Benthos's own traffic",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				if inputRate(s) < 0 {
					return false, "", fmt.Errorf("the rate after the restart is negative: %.2f msg/s",
						inputRate(s))
				}

				done := inputRate(s) > 0 && s.Result.Sample.IsActive

				return done, benthosSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("the first Benthos dies: its server closes")
		first.close()

		if err := waitForSample(refFirst, "the first monitor reads degraded and keeps its last good sample",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				input, _, _ := first.counters()
				done := obs.State == "degraded" && s.Degraded &&
					strings.Contains(s.Reason, "poll error") &&
					s.Result.Sample.Metrics.InputReceivedTotal() == input && s.Result.Probes.PingAlive

				return done, benthosSeen(obs), nil
			}); err != nil {
			return err
		}

		env.Step("the second Benthos's /metrics stops answering; its probes still do")
		second.setFailing("/metrics", 500)

		// A failed scrape degrades the monitor on the first missed scrape; the
		// store read stays fresh while the collector runs, because the
		// three-missed-scrapes staleness rule lives in the FSMv1 adapter,
		// which the kernel-only supervisor does not run.
		if err := env.WaitFor(ctx, "the second monitor reads degraded on the stopped metrics, holds its last good sample, and its observation stays fresh",
			func(ctx context.Context) (bool, string, error) {
				obs, fresh, err := fsmv2client.GetFresh[simple.Status[fsmv2benthos.Scrape]](ctx, env.Client, refSecond, benthosStaleAfter)
				if err != nil {
					return false, "", err
				}

				s := obs.Status
				input, _, _ := second.counters()
				done := fresh == fsmv2client.Fresh &&
					s.Degraded && strings.Contains(s.Reason, "/metrics: unexpected status 500") &&
					s.Result.Sample.Metrics.InputReceivedTotal() == input && s.Result.Probes.PingAlive

				return done, fmt.Sprintf("fresh=%s %s", fresh, benthosSeen(obs)), nil
			}); err != nil {
			return err
		}

		env.Step("delete the second monitor; its Benthos answers again")
		env.Client.Delete(refSecond)
		second.setFailing("/metrics", 0)

		if err := env.WaitFor(ctx, "the store reads the deleted monitor as deleted",
			func(ctx context.Context) (bool, string, error) {
				_, fresh, err := fsmv2client.GetFresh[simple.Status[fsmv2benthos.Scrape]](ctx, env.Client, refSecond, benthosStaleAfter)

				return fresh == fsmv2client.Deleted, fresh.String(), err
			}); err != nil {
			return err
		}

		// The supervisor reaps the deleted worker on its own ticks, which
		// nothing the client reads can name: the client answers "deleted" from
		// its own record. The surviving monitor collects only under a running
		// supervisor, so two of its scrapes stand for those ticks.
		var collectedSinceDelete time.Time

		if err := env.WaitFor(ctx, "two scrapes of the surviving monitor have passed since the delete",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[simple.Status[fsmv2benthos.Scrape]](ctx, env.Client, refFirst)
				if err != nil {
					return false, "", err
				}

				if collectedSinceDelete.IsZero() {
					collectedSinceDelete = obs.CollectedAt
				}

				done := obs.CollectedAt.Sub(collectedSinceDelete) >= 2*benthosmetrics.ScrapeInterval

				return done, fmt.Sprintf("collected_at=%s", obs.CollectedAt.Format(time.TimeOnly)), nil
			}); err != nil {
			return err
		}

		env.Step("recreate the monitor under the same name on the same Benthos")
		if err := upsert(refSecond, second); err != nil {
			return err
		}

		if err := waitForSample(refSecond, "the recreated monitor's first sample carries no rate",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				_, known := s.Result.Sample.InputMessagesPerSecond.Get()
				done := !known && !s.Result.Sample.IsActive

				return done, benthosSeen(obs), nil
			}); err != nil {
			return err
		}

		return waitForSample(refSecond, "the recreated monitor reads its own rate of 1 msg/s, active and running",
			func(obs benthosObservation) (bool, string, error) {
				s := obs.Status
				// 1 = 10 messages per read ÷ benthosReadAdvance, exact because
				// the first fake is closed and no longer advances the clock.
				done := obs.State == "running" && !s.Degraded &&
					inputRate(s) == 1 && s.Result.Sample.IsActive

				return done, benthosSeen(obs), nil
			})
	},
}

// inputRate reads the stored input rate, 0 while the window cannot answer.
func inputRate(s simple.Status[fsmv2benthos.Scrape]) float64 {
	v, _ := s.Result.Sample.InputMessagesPerSecond.Get()

	return v
}

// outputRate reads the stored output rate, 0 while the window cannot answer.
func outputRate(s simple.Status[fsmv2benthos.Scrape]) float64 {
	v, _ := s.Result.Sample.OutputMessagesPerSecond.Get()

	return v
}
