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

package examples

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	pkgconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	fsmv2timescale "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/historian"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/historian/timescalemetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// CollectSummary leaves customer_export out, because the historian did not create it.
var historianScenarioDatabase = timescalemetrics.FakeContents{
	PostgresVersion:  "17.6",
	TimescaleVersion: "2.24.0",
	Tables: []timescalemetrics.Table{
		{
			Name:                 "value_scenario",
			IsHypertable:         true,
			OccupiedDiskBytes:    4_000_000,
			EarliestRowTimestamp: "2026-01-01T00:00:00Z",
			LatestRowTimestamp:   "2026-01-02T00:00:00Z",
		},
		{Name: "tag", OccupiedDiskBytes: 300_000, Rows: 12},
		{Name: "customer_export", OccupiedDiskBytes: 50_000_000},
	},
	JobCount:                  1,
	DatabaseOccupiedDiskBytes: 900_000_000,
}

type historianObservation = fsmv2.Observation[simple.Status[fsmv2timescale.TimescaleStatus]]

// HistorianScenario runs the historian monitor against a FakeDatabase stored under DatabaseKey.
var HistorianScenario = Scenario{
	Name:        "historian",
	Description: "Timescale monitor against a fake database: running, then unreachable, then password rejected, then running again",

	Dependencies: func() (map[string]any, func(), error) {
		deps := map[string]any{}

		var database timescalemetrics.Database = timescalemetrics.NewFakeDatabase(historianScenarioDatabase)

		config.SetDependency(deps, fsmv2timescale.DatabaseKey, database)

		return deps, nil, nil
	},

	Run: func(ctx context.Context, env Env) error {
		database, ok := config.LookupDependency(env.Dependencies, fsmv2timescale.DatabaseKey)
		if !ok {
			return errors.New("the historian scenario's dependency map holds no database under fsmv2timescale.DatabaseKey")
		}

		fake, ok := database.(*timescalemetrics.FakeDatabase)
		if !ok {
			return errors.New("the database under fsmv2timescale.DatabaseKey is not a timescalemetrics.FakeDatabase")
		}

		cfg := pkgconfig.HistorianConfig{
			Timescale: pkgconfig.TimescaleConfig{
				Host:     "timescale.scenario.invalid",
				Password: "scenario-password",
			},
		}

		waitForHistorian := func(name string, done func(obs historianObservation) bool) error {
			return env.WaitFor(ctx, name,
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[simple.Status[fsmv2timescale.TimescaleStatus]](ctx, env.Client, fsmv2timescale.Ref)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the monitor has not published an observation yet", nil
						}

						return false, "", err
					}

					result := obs.Status.Result
					seen := fmt.Sprintf("state=%s degraded=%t host=%s reachable=%t auth=%s tables=%v data_span_seconds=%d database_bytes=%d reason=%q",
						obs.State, obs.Status.Degraded, result.Host, result.Reachable, result.Auth,
						result.TableNames, result.DataSpanSeconds, result.DatabaseOccupiedDiskBytes, obs.Status.Reason)

					return done(obs), seen, nil
				})
		}

		env.Step("create the historian monitor with the database answering; it starts in running, so no state_transition line shows it entering running")

		if err := env.Client.Upsert(fsmv2timescale.Ref, cfg.ToTemplateMap()); err != nil {
			return err
		}

		if err := waitForHistorian("store shows running with valid credentials",
			func(obs historianObservation) bool {
				return obs.State == "running" &&
					!obs.Status.Degraded &&
					obs.Status.Result.Host == cfg.Timescale.Host &&
					obs.Status.Result.Reachable &&
					obs.Status.Result.Auth == models.TimescaleAuthValid
			}); err != nil {
			return err
		}

		if err := waitForHistorian("store shows the summary CollectSummary read from the database, and its size",
			func(obs historianObservation) bool {
				return slices.Equal(obs.Status.Result.TableNames, []string{"value_scenario", "tag"}) &&
					obs.Status.Result.DataSpanSeconds == 24*60*60 &&
					obs.Status.Result.DatabaseOccupiedDiskBytes == historianScenarioDatabase.DatabaseOccupiedDiskBytes
			}); err != nil {
			return err
		}

		env.Step("the database stops answering")
		fake.SetErr(timescalemetrics.ErrFakeConnectionRefused)

		if err := waitForHistorian("store shows degraded and unreachable",
			func(obs historianObservation) bool {
				return obs.State == "degraded" &&
					obs.Status.Degraded &&
					!obs.Status.Result.Reachable &&
					obs.Status.Result.Auth == models.TimescaleAuthUnknown &&
					strings.Contains(obs.Status.Reason, "connection refused")
			}); err != nil {
			return err
		}

		env.Step("the database rejects the password; the monitor stays degraded, so wait for auth=invalid in the observation")
		fake.SetErr(timescalemetrics.ErrFakePasswordRejected)

		if err := waitForHistorian("store shows degraded with credentials rejected",
			func(obs historianObservation) bool {
				return obs.State == "degraded" &&
					obs.Status.Degraded &&
					obs.Status.Result.Reachable &&
					obs.Status.Result.Auth == models.TimescaleAuthInvalid
			}); err != nil {
			return err
		}

		env.Step("the database accepts the password again; the monitor declares no Health function, so it returns to running with reason \"running (no health check)\"")
		fake.SetErr(nil)

		return waitForHistorian("store shows running again",
			func(obs historianObservation) bool {
				return obs.State == "running" &&
					!obs.Status.Degraded &&
					obs.Status.Result.Reachable &&
					obs.Status.Result.Auth == models.TimescaleAuthValid
			})
	},
}
