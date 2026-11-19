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

package configworker_test

import (
	"context"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	fsmv2timescale "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/historian"
	worker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

const historianMetricPrefix = "umh_historian_"

// exportedHistorianSeries counts the umh_historian_* series /metrics would serve.
func exportedHistorianSeries(t *testing.T) int {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather the default registry: %v", err)
	}

	count := 0

	for _, family := range families {
		if strings.HasPrefix(family.GetName(), historianMetricPrefix) {
			count += len(family.GetMetric())
		}
	}

	return count
}

func newWorkerWithoutHistorian(t *testing.T) *worker.ConfigworkerWorker {
	t.Helper()

	register.SetGlobalDeps[*dynamicchildren.Registry](workerType, dynamicchildren.NewWriter().Registry())
	t.Cleanup(func() { register.ClearGlobalDeps(workerType) })

	register.SetGlobalDeps[config.ConfigManager](worker.ConfigManagerDepsKey,
		config.NewMockConfigManager().WithConfig(config.FullConfig{}))
	t.Cleanup(func() { register.ClearGlobalDeps(worker.ConfigManagerDepsKey) })

	withClient(t)

	identity := deps.Identity{ID: workerType + "-001", WorkerType: workerType}

	w, err := worker.NewConfigworkerWorker(identity, deps.NewNopFSMLogger(), nil)
	if err != nil {
		t.Fatalf("NewConfigworkerWorker: %v", err)
	}

	return w
}

// A poll still in flight when the historian is removed records its status after
// the child is deleted; the next tick without a historian removes those series.
func TestCollectObservedStateRemovesHistorianMetricsWhenHistorianAbsent(t *testing.T) {
	t.Cleanup(fsmv2timescale.CleanupHistorianMetrics)

	w := newWorkerWithoutHistorian(t)

	fsmv2timescale.RecordHistorianStatus("timescale-001(historian-timescale)", fsmv2timescale.TimescaleStatus{
		Auth:      models.TimescaleAuthValid,
		Reachable: true,
	})

	if exportedHistorianSeries(t) == 0 {
		t.Fatalf("RecordHistorianStatus exported no %s series, so the check below proves nothing", historianMetricPrefix)
	}

	desired := &fsmv2.WrappedDesiredState[snapshot.ConfigworkerConfig]{}
	if _, err := w.CollectObservedState(context.Background(), desired); err != nil {
		t.Fatalf("CollectObservedState (historian absent): %v", err)
	}

	if got := exportedHistorianSeries(t); got != 0 {
		t.Fatalf("historian absent: %d %s series still exported after reconcile, want 0", got, historianMetricPrefix)
	}
}
