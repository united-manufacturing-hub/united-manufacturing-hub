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
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/logger"
	fsmv2sentry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	fsmv2memory "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/memory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/memoryhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

const (
	memoryWorkerMaxAge = 3 * fsmv2memory.PollInterval

	memoryClientUnavailableTag = "memory::worker_client_unavailable"

	memorySupervisorNotRunningMessage = "USE_FSMV2_MEMORY_MONITOR is enabled but the fsmv2 supervisor is not running, so memory is not measured"
	memoryNoVerdictMessage            = "Memory worker produced no verdict for its last observation"
)

var containerMonitorSentryLogger = sync.OnceValue(func() deps.FSMLogger {
	hook := fsmv2sentry.NewSentryHook(5 * time.Minute)
	wrapped := logger.For(logger.ComponentContainerMonitorService).Desugar().WithOptions(zap.WrapCore(hook.Wrap))

	return deps.NewFSMLogger(wrapped.Sugar())
})

func (c *ContainerMonitorService) collectMemoryFromWorker(ctx context.Context) (*models.Memory, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	client := fsmv2client.GetClient()
	if client == nil {
		c.memoryWorkerWarnOnce.Do(func() {
			c.sentryLogger.SentryWarn(deps.FeatureSupportMemory, "", memoryClientUnavailableTag,
				deps.String("detail", memorySupervisorNotRunningMessage))
		})

		return degradedMemory(memorySupervisorNotRunningMessage), nil
	}

	workerObs, freshness, err := fsmv2client.GetFresh[simple.Status[fsmv2memory.MemoryStatus]](ctx, client, fsmv2memory.Ref, memoryWorkerMaxAge)
	if err != nil {
		return degradedMemory(fmt.Sprintf("Memory worker observation could not be read: %v", err)), nil
	}

	return judgeWorkerMemory(workerObs.Status, freshness), nil
}

func judgeWorkerMemory(status simple.Status[fsmv2memory.MemoryStatus], freshness fsmv2client.Freshness) *models.Memory {
	if freshness != fsmv2client.Fresh {
		return degradedMemory(unfreshMemoryMessage(freshness))
	}

	if status.Degraded && status.Result.Verdict.State != memoryhealth.StateDegraded {
		return degradedMemory(status.Reason)
	}

	switch status.Result.Verdict.State {
	case memoryhealth.StateHealthy:
		return measuredMemory(status.Result, models.Active)
	case memoryhealth.StateDegraded:
		return measuredMemory(status.Result, models.Degraded)
	default:
		return degradedMemory(memoryNoVerdictMessage)
	}
}

func unfreshMemoryMessage(freshness fsmv2client.Freshness) string {
	switch freshness {
	case fsmv2client.Stale:
		return fmt.Sprintf("Memory worker observation is stale (older than %s); no memory measurement to judge", memoryWorkerMaxAge)
	case fsmv2client.NotFound:
		return "Memory worker has never observed; no memory measurement to judge"
	case fsmv2client.Deleted:
		return "Memory worker was removed; no memory measurement to judge"
	default:
		return "Memory worker observation could not be classified; no memory measurement to judge"
	}
}

func measuredMemory(result fsmv2memory.MemoryStatus, category models.HealthCategory) *models.Memory {
	return &models.Memory{
		Health:           memoryHealth(result.Message, category),
		CGroupUsedBytes:  result.AvgUsedBytes,
		CGroupTotalBytes: result.TotalBytes,
		MemoryHealth:     &models.MemoryHealth{Verdict: result.Verdict, Details: result.Details},
	}
}

func degradedMemory(message string) *models.Memory {
	return &models.Memory{Health: memoryHealth(message, models.Degraded)}
}

func memoryHealth(message string, category models.HealthCategory) *models.Health {
	return &models.Health{
		Message:       message,
		ObservedState: category.String(),
		DesiredState:  models.Active.String(),
		Category:      category,
	}
}
