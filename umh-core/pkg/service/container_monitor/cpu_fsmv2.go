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

package container_monitor

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/env"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// cpuWorkerMaxAge is how old the fsmv2 CPU worker's observation may be and still
// count as Fresh for the seam. It is 3x the worker's 1s poll interval
// (pollInterval in pkg/fsmv2/cpu), so one slow or missed poll cannot flip the
// instance to degraded.
const cpuWorkerMaxAge = 3 * time.Second

// collectCPUFromWorker builds the whole CPU record from the fsmv2 CPU worker's
// last observation. The legacy fields stay empty on purpose: old and new
// reporting stay cleanly separated, so nothing here re-derives a legacy-named
// number from worker data. models.CPU says what each generation carries.
func (c *ContainerMonitorService) collectCPUFromWorker(ctx context.Context) *models.CPU {
	health, cpuHealth := c.readWorkerCPUHealth(ctx)

	return &models.CPU{Health: health, CPUHealth: cpuHealth}
}

// readWorkerCPUHealth reads the fsmv2 CPU worker's observation and maps it to a
// models.Health. It always returns a health, and the protocol converter's
// IsResourceLimited reads that message as its bridge-block reason.
func (c *ContainerMonitorService) readWorkerCPUHealth(ctx context.Context) (health *models.Health, cpuHealth *models.CPUHealth) {
	client := fsmv2client.GetClient()
	// Fallback for a misconfiguration: USE_FSMV2_CPU is on but nothing published
	// a client, so the fsmv2 supervisor never started (or has not yet).
	if client == nil {
		message := c.cpuSeamClientUnavailableMessage()

		c.cpuWorkerWarnOnce.Do(func() {
			c.logger.Warn(message)
		})

		return &models.Health{
			Message:       message,
			ObservedState: models.Degraded.String(),
			DesiredState:  models.Active.String(),
			Category:      models.Degraded,
		}, nil
	}

	// Get the latest poll result from the worker.
	workerStatus, freshness, err := fsmv2client.GetFresh[simple.Status[fsmv2cpu.CPUStatus]](ctx, client, fsmv2cpu.Ref, cpuWorkerMaxAge)
	if err != nil {
		// GetFresh returns Unknown here: the read failure prevented it from being
		// classified, so it cannot be called healthy. Fail closed with the
		// verbatim store error as the message.
		return &models.Health{
			Message:       fmt.Sprintf("CPU worker observation could not be read: %v", err),
			ObservedState: models.Degraded.String(),
			DesiredState:  models.Active.String(),
			Category:      models.Degraded,
		}, nil
	}

	// If it is not fresh, handle these cases here.
	if freshness != fsmv2client.Fresh {
		message := "CPU worker observation could not be classified; no measurement to judge"

		switch freshness {
		case fsmv2client.Stale:
			message = fmt.Sprintf("CPU worker observation is stale (older than %s); cannot trust the verdict it carries", cpuWorkerMaxAge)
		case fsmv2client.NeverObserved:
			message = "CPU worker has never observed; no measurement to judge"
		case fsmv2client.Unregistered:
			message = "CPU worker is not registered with the fsmv2 runtime; no measurement to judge"
		}

		return &models.Health{
			Message:       message,
			ObservedState: models.Degraded.String(),
			DesiredState:  models.Active.String(),
			Category:      models.Degraded,
		}, nil
	}

	// Degraded without a degraded verdict means the poll failed, not that the box
	// is degraded.
	if workerStatus.Degraded && workerStatus.Result.Verdict.State != cpuhealth.StateDegraded {
		return &models.Health{
			Message:       workerStatus.Reason,
			ObservedState: models.Degraded.String(),
			DesiredState:  models.Active.String(),
			Category:      models.Degraded,
		}, nil
	}

	// A Fresh observation carries the developer's judgement in Result. The
	// switch maps only the two spelled-out states, so a rename of either
	// fails the seam tests too.
	switch workerStatus.Result.Verdict.State {
	case cpuhealth.StateHealthy:
		return &models.Health{
				Message:       workerStatus.Result.Message,
				ObservedState: models.Active.String(),
				DesiredState:  models.Active.String(),
				Category:      models.Active,
			}, &models.CPUHealth{
				Verdict: workerStatus.Result.Verdict,
				Details: workerStatus.Result.Details,
			}
	case cpuhealth.StateDegraded:
		return &models.Health{
				Message:       workerStatus.Result.Message,
				ObservedState: models.Degraded.String(),
				DesiredState:  models.Active.String(),
				Category:      models.Degraded,
			}, &models.CPUHealth{
				Verdict: workerStatus.Result.Verdict,
				Details: workerStatus.Result.Details,
			}
	default:
		// Empty result verdict AND Degraded == false is a genuine "no
		// determination" — a successful poll produced no verdict. There is no
		// second opinion to defer to, so say so rather than read it as healthy.
		return &models.Health{
			Message:       "CPU worker produced no verdict for its last observation",
			ObservedState: models.Degraded.String(),
			DesiredState:  models.Active.String(),
			Category:      models.Degraded,
		}, nil
	}
}

// cpuSeamClientUnavailableMessage says which prerequisite is missing when
// USE_FSMV2_CPU is on but no CPU worker client was published, or that the
// supervisor may still be starting.
func (c *ContainerMonitorService) cpuSeamClientUnavailableMessage() string {
	transportOn, _ := env.GetAsBool("USE_FSMV2_TRANSPORT", false, true)
	if !transportOn {
		return "USE_FSMV2_CPU is enabled but USE_FSMV2_TRANSPORT is off, so the fsmv2 supervisor never runs and no CPU worker client is published; no CPU measurement is available"
	}

	if os.Getenv("API_URL") == "" || os.Getenv("AUTH_TOKEN") == "" {
		return "USE_FSMV2_CPU is enabled but API_URL or AUTH_TOKEN is unset, so the fsmv2 supervisor never runs and no CPU worker client is published; no CPU measurement is available"
	}

	return "USE_FSMV2_CPU is enabled but no fsmv2 client is reachable yet (the fsmv2 supervisor may still be starting); no CPU measurement is available"
}
