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
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

const (
	transportWorkerPath = "app(application)/transport-1-001(transport)"
	pushWorkerPath      = "app(application)/transport-1-001(transport)/push-001(push)"
	pullWorkerPath      = "app(application)/transport-1-001(transport)/pull-001(pull)"
)

func transitionFields(from, to string) []deps.Field {
	return []deps.Field{deps.String("from_state", from), deps.String("to_state", to)}
}

func TestPushLoadCountersProbe(t *testing.T) {
	counters := newPushLoadCounters()
	root := newPushLoadCountingLogger(deps.NewNopFSMLogger(), counters)

	// The worker path arrives via With, the way the supervisor attaches it.
	push := root.With(deps.String("worker", pushWorkerPath))
	pull := root.With(deps.String("worker", pullWorkerPath))
	transport := root.With(deps.String("worker", transportWorkerPath))

	// One push degrade, logged twice: the de-duplication must fold it to one.
	push.Info("state_transition", transitionFields("Running", "Degraded")...)
	push.Info("state_transition", transitionFields("Running", "Degraded")...)

	// A second, real push degrade needs a different transition in between.
	push.Info("state_transition", transitionFields("Degraded", "Running")...)
	push.Info("state_transition", transitionFields("Running", "Degraded")...)

	// The pull child's Degraded is byte-identical and must not be counted.
	pull.Info("state_transition", transitionFields("Running", "Degraded")...)

	transport.Info("state_transition", transitionFields("Running", "Degraded")...)

	deadline := errors.New("push failed: " + context.DeadlineExceeded.Error())
	shutdown := errors.New("context canceled during retry: context canceled")

	push.SentryError(deps.FeatureForWorker("push"), pushWorkerPath, deadline, "action_failed",
		deps.ActionName("push"))
	push.SentryError(deps.FeatureForWorker("push"), pushWorkerPath, shutdown, "action_failed",
		deps.ActionName("push"))
	pull.SentryError(deps.FeatureForWorker("pull"), pullWorkerPath, deadline, "action_failed",
		deps.ActionName("pull"))

	push.Info("push_reset_cleared", deps.Int("pending_dropped", 7))
	push.Info("push_reset_cleared", deps.Int("pending_dropped", 3))

	counters.sampleQueue(100, 100)
	counters.sampleQueue(100, 100)
	counters.sampleQueue(50, 100)

	got := counters.snapshot()
	want := pushLoadTotals{
		transportDegrades: 1,
		pushDegrades:      2,
		budgetExpiries:    1,
		resets:            2,
		maxPendingDropped: 7,
		queueSamples:      3,
		queueFullSamples:  2,
	}

	t.Logf("got  %+v", got)
	t.Logf("want %+v", want)

	if got != want {
		t.Fatalf("counters mismatch")
	}

	t.Logf("queue_full_duty_pct=%v", percent(got.queueFullSamples, got.queueSamples))
	t.Logf("budget_expiries_per_min over 30s=%v", perMinute(got.budgetExpiries, 0.5))
}
