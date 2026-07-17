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

package generator

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	fsmv2historian "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/historian"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// historianMaxAge bounds how old an observation may be before it is treated as
// stale. The monitor polls once per second; an older read means the worker is
// wedged or gone, so the endpoint is reported degraded.
const historianMaxAge = 10 * time.Second

// HistorianFromFSMv2 reads the historian monitor child's observed state from the
// fsmv2 store and maps it to models.Historian. It returns nil (omitting the
// section) when no historian is configured, the client is unavailable, no
// observation exists yet, or the read fails.
func HistorianFromFSMv2(ctx context.Context, log *zap.SugaredLogger) *models.Historian {
	client := fsmv2client.GetClient()
	if client == nil {
		return nil
	}

	status, freshness, err := fsmv2client.GetFresh[simple.Status[fsmv2historian.TimescaleStatus]](ctx, client, fsmv2historian.Ref, historianMaxAge)
	if err != nil {
		log.Warnw("historian status: failed to read observed state", "error", err)

		return nil
	}

	// Unregistered (no historian configured) and NeverObserved (registered but
	// not yet polled) both mean "nothing to report" — omit the section.
	if freshness != fsmv2client.Fresh && freshness != fsmv2client.Stale {
		return nil
	}

	result := status.Result

	// A stale observation is degraded regardless of its last-seen verdict: the
	// monitor stopped reporting, so the endpoint's reachability is unknown.
	degraded := status.Degraded || freshness == fsmv2client.Stale

	healthCat := models.Active
	observedState := "active"
	message := status.Reason

	if degraded {
		healthCat = models.Degraded
		observedState = "degraded"

		if freshness == fsmv2client.Stale {
			message = "historian monitor observation is stale"
		}
	}

	return &models.Historian{
		Timescale: models.Timescale{
			Health: &models.Health{
				Message:       message,
				ObservedState: observedState,
				DesiredState:  "active",
				Category:      healthCat,
			},
			Host:      result.Host,
			Latency:   result.LatencyMs,
			Port:      result.Port,
			Reachable: result.Reachable,
			AuthValid: result.AuthValid,
		},
	}
}
