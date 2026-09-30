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
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	fsmv2memory "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/memory"
	fsmv2sentry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

const (
	memoryWorkerMaxAge = 3 * fsmv2memory.PollInterval

	memoryClientUnavailableTag = "memory::worker_client_unavailable"
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
		message := memorySeamClientUnavailableMessage()

		c.memoryWorkerWarnOnce.Do(func() {
			c.sentryLogger.SentryWarn(deps.FeatureSupportMemory, "", memoryClientUnavailableTag, deps.String("detail", message))
		})

		return degradedMemory(message), nil
	}

	status, freshness, err := fsmv2client.GetFresh[simple.Status[fsmv2memory.MemoryStatus]](ctx, client, fsmv2memory.Ref, memoryWorkerMaxAge)
	if err != nil {
		return degradedMemory(fmt.Sprintf("Memory worker observation could not be read: %v", err)), nil
	}

	return judgeWorkerMemory(status, freshness), nil
}

func memorySeamClientUnavailableMessage() string {
	transportOn, _ := env.GetAsBool("USE_FSMV2_TRANSPORT", false, true)
	if !transportOn {
		return "USE_FSMV2_MEMORY_MONITOR is enabled but USE_FSMV2_TRANSPORT is off, so the fsmv2 supervisor never runs and no memory worker client is published; no memory measurement is available"
	}

	if os.Getenv("API_URL") == "" || os.Getenv("AUTH_TOKEN") == "" {
		return "USE_FSMV2_MEMORY_MONITOR is enabled but API_URL or AUTH_TOKEN is unset, so the fsmv2 supervisor never runs and no memory worker client is published; no memory measurement is available"
	}

	return "USE_FSMV2_MEMORY_MONITOR is enabled but no fsmv2 client is reachable yet (the fsmv2 supervisor may still be starting); no memory measurement is available"
}

func judgeWorkerMemory(status simple.Status[fsmv2memory.MemoryStatus], freshness fsmv2client.Freshness) *models.Memory {
	if freshness != fsmv2client.Fresh {
		return degradedMemory(unfreshMemoryMessage(freshness))
	}

	if !status.Result.Measured {
		return degradedMemory(status.Reason)
	}

	category := models.Active
	if status.Degraded {
		category = models.Degraded
	}

	return &models.Memory{
		Health:           memoryHealth(status.Result.Message, category),
		CGroupUsedBytes:  status.Result.UsedBytes,
		CGroupTotalBytes: status.Result.TotalBytes,
	}
}

func unfreshMemoryMessage(freshness fsmv2client.Freshness) string {
	switch freshness {
	case fsmv2client.Stale:
		return fmt.Sprintf("Memory worker observation is stale (older than %s); no memory measurement to judge", memoryWorkerMaxAge)
	case fsmv2client.NeverObserved:
		return "Memory worker has never observed; no memory measurement to judge"
	case fsmv2client.Unregistered:
		return "Memory worker is not registered with the fsmv2 runtime; no memory measurement to judge"
	default:
		return "Memory worker observation could not be classified; no memory measurement to judge"
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
