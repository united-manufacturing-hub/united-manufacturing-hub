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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	fsmv2sentry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// cpuWorkerMaxAge is how old the fsmv2 CPU worker's observation may be and still
// count as Fresh for the seam. It leaves enough slack that one slow or missed
// poll cannot flip the instance to degraded. The seam is the code path that
// reports CPU from the fsmv2 worker instead of the legacy sampler, selected at
// construction by USE_FSMV2_CPU.
const cpuWorkerMaxAge = 3 * fsmv2cpu.PollInterval

// collectCPUFromWorker builds the whole CPU record from the fsmv2 CPU worker's
// last observation. The legacy fields stay empty on purpose: old and new
// reporting stay cleanly separated, so nothing here re-derives a legacy-named
// number from worker data. models.CPU says which fields the worker fills and
// which the legacy path does.
//
// It errors only when the tick was cancelled: a cancelled tick measured
// nothing, so it has no verdict to report. getCPUMetrics aborts on a cancelled
// ctx the same way.
func (c *ContainerMonitorService) collectCPUFromWorker(ctx context.Context) (*models.CPU, error) {
	health, cpuHealth, err := c.readWorkerCPUHealth(ctx)
	if err != nil {
		return nil, err
	}

	return &models.CPU{Health: health, CPUHealth: cpuHealth}, nil
}

// cpuVerdict is the seam's judgement about the CPU worker's last observation:
// what to say, how to classify it, and the measurement it was drawn from.
// cpuHealth is nil when the seam sends no verdict: there was no measurement to
// judge, or the worker is degraded while its verdict is not.
type cpuVerdict struct {
	cpuHealth *models.CPUHealth
	message   string
	category  models.HealthCategory
}

// health renders the verdict as the models.Health the seam reports. It is the
// one place the seam builds that struct, so ObservedState tracking Category and
// DesiredState being Active hold by construction rather than by agreement
// between the places that build it.
func (v cpuVerdict) health() *models.Health {
	return &models.Health{
		Message:       v.message,
		ObservedState: v.category.String(),
		DesiredState:  models.Active.String(),
		Category:      v.category,
	}
}

// degradedCPU is the fail-closed verdict: degraded, with a message and no
// cpuHealth. Used wherever the seam could not measure, could not classify, or
// withholds the worker's verdict.
func degradedCPU(message string) cpuVerdict {
	return cpuVerdict{message: message, category: models.Degraded}
}

// judgeWorkerCPUReadError is the verdict for an observation the store could not
// return. GetFresh reports Unknown freshness in that case: the read failure
// prevented the observation from being classified, so it cannot be called
// healthy. Fail closed with the verbatim store error as the message.
func judgeWorkerCPUReadError(err error) cpuVerdict {
	return degradedCPU(fmt.Sprintf("CPU worker observation could not be read: %v", err))
}

// judgeWorkerCPU turns the observation fsmv2client.GetFresh returned into a
// verdict.
func judgeWorkerCPU(
	status simple.Status[fsmv2cpu.CPUStatus],
	freshness fsmv2client.Freshness,
) cpuVerdict {
	// If it is not fresh, handle these cases here.
	if freshness != fsmv2client.Fresh {
		message := "CPU worker observation could not be classified; no measurement to judge"

		switch freshness {
		case fsmv2client.Stale:
			message = fmt.Sprintf("CPU worker observation is stale (older than %s); cannot trust the verdict it carries", cpuWorkerMaxAge)
		case fsmv2client.NotFound:
			message = "CPU worker has never observed; no measurement to judge"
		case fsmv2client.Deleted:
			message = "CPU worker was removed; no measurement to judge"
		case fsmv2client.Unknown:
			// The preset message above covers it.
		}

		return degradedCPU(message)
	}

	// Degraded with a verdict that is not degraded: the poll failed, or the CPU
	// usage is not measured yet (cpuhealth.UsageMeasured). Send no verdict. A
	// healthy verdict next to a degraded category would contradict the message
	// the operator reads.
	if status.Degraded && status.Result.Verdict.State != cpuhealth.StateDegraded {
		return degradedCPU(status.Reason)
	}

	// A Fresh observation carries the developer's judgement in Result. The
	// switch maps only the two spelled-out states, so a rename of either
	// fails the seam tests too.
	switch status.Result.Verdict.State {
	case cpuhealth.StateHealthy:
		return cpuVerdict{
			cpuHealth: &models.CPUHealth{
				Verdict: status.Result.Verdict,
				Details: status.Result.Details,
			},
			message:  status.Result.Message,
			category: models.Active,
		}
	case cpuhealth.StateDegraded:
		return cpuVerdict{
			cpuHealth: &models.CPUHealth{
				Verdict: status.Result.Verdict,
				Details: status.Result.Details,
			},
			message:  status.Result.Message,
			category: models.Degraded,
		}
	default:
		// Empty result verdict AND Degraded == false is a genuine "no
		// determination" — a successful poll produced no verdict. There is no
		// second opinion to defer to, so say so rather than read it as healthy.
		return degradedCPU("CPU worker produced no verdict for its last observation")
	}
}

// readWorkerCPUHealth reads the fsmv2 CPU worker's observation and maps it to a
// models.Health. Every outcome it can judge produces one, and the protocol
// converter's BridgeMustWait reads that message as the reason a bridge waits.
// A cancelled tick is the one case with nothing to judge: it returns the ctx
// error and no health.
func (c *ContainerMonitorService) readWorkerCPUHealth(ctx context.Context) (*models.Health, *models.CPUHealth, error) {
	// A cancelled tick is not a degraded box. This is the first thing the
	// function does, so nothing below it can publish a verdict nothing
	// measured -- including the missing-client case, which reports an absent
	// prerequisite that a cancelled tick has not established. getCPUMetrics
	// likewise aborts on a cancelled ctx rather than reporting.
	//
	// It catches a ctx already cancelled on entry, not one cancelled during the
	// store read below. That window is deliberately uncovered: no store in this
	// repo fails a read on a cancelled ctx (persistence/memory's validateContext
	// rejects only a nil ctx), so a second check after GetFresh would never fire.
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}

	client := fsmv2client.GetClient()
	if client == nil {
		warnFSMv2SupervisorNotRunning()

		return degradedCPU(fsmv2SupervisorNotRunningMessage).health(), nil, nil
	}

	// Get the latest poll result from the worker.
	workerObs, freshness, err := fsmv2client.GetFresh[simple.Status[fsmv2cpu.CPUStatus]](ctx, client, fsmv2cpu.Ref, cpuWorkerMaxAge)

	if err != nil {
		v := judgeWorkerCPUReadError(err)

		return v.health(), v.cpuHealth, nil
	}

	v := judgeWorkerCPU(workerObs.Status, freshness)

	return v.health(), v.cpuHealth, nil
}

// fsmv2SupervisorNotRunningMessage is the diagnosis when USE_FSMV2_CPU is on
// but no fsmv2 client is published. cmd/main.go publishes the client before
// the supervisor runs. It clears it only when the supervisor failed to build
// or has shut down. Sentry groups the warning by this text, so changing it
// starts a new Sentry issue.
const fsmv2SupervisorNotRunningMessage = "USE_FSMV2_CPU is enabled but the fsmv2 supervisor is not running, so CPU is not measured"

// cpuSeamHierarchyPath is parsed by sentry.ParseHierarchyPath as an FSMv1
// dotted path, so the event is tagged fsm_version=v1 and
// worker_type=ContainerMonitor.
const cpuSeamHierarchyPath = "fsmv1.ContainerMonitor"

// fsmv2SupervisorNotRunningOnce limits warnFSMv2SupervisorNotRunning to one
// event per process.
var fsmv2SupervisorNotRunningOnce sync.Once

// warnFSMv2SupervisorNotRunning logs fsmv2SupervisorNotRunningMessage on the
// component logger and sends it to Sentry, once per process. The component
// logger has no Sentry hook, so it adds one for this call only.
func warnFSMv2SupervisorNotRunning() {
	fsmv2SupervisorNotRunningOnce.Do(func() {
		hook := fsmv2sentry.NewSentryHook(time.Minute)
		// Stop ends the debouncer's cleanup goroutine. It loses no event:
		// SentryHook.Write (pkg/fsmv2/sentry/hook.go) captures synchronously,
		// so the event reaches the Sentry client before SentryWarn returns.
		defer hook.Stop()

		base := logger.For(logger.ComponentContainerMonitorService)
		hooked := base.Desugar().WithOptions(zap.WrapCore(hook.Wrap)).Sugar()

		deps.NewFSMLogger(hooked).SentryWarn(deps.FeatureSupportCPU, cpuSeamHierarchyPath, fsmv2SupervisorNotRunningMessage)
	})
}
