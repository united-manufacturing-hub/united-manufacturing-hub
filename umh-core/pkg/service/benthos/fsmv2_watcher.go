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

package benthos

// This file defines the FSMv2 benthos_monitor watcher seam: the interface the
// BenthosService read/lifecycle paths call behind USE_FSMV2_BENTHOS_MONITOR,
// the production default that delegates to the process-scoped fsmv2client, and
// the feature-flag cache. The monitor→worker state vocabulary translation
// (mapFrom) lands in R8 alongside its first call site. The seam is wired into
// GetHealthCheckAndMetrics by the PR3b rungs; FF-off stays byte-identical
// because every branch is guarded by fsmv2BenthosMonitorEnabled.

import (
	"context"
	"errors"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	bmworker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
)

// fsmv2BenthosMonitorEnabled gates the FF-on read + lifecycle paths. It is read
// once at package init from USE_FSMV2_BENTHOS_MONITOR (default OFF). env.GetAsBool
// reads os.Getenv directly (no config-file parsing), so init-time is correct and
// the value is fixed for the process lifetime. It is NOT threaded through
// NewBenthosManager (honors C-Inject: the CPU-target benthos runs through the
// DataFlowComponent manager, which the ctor wouldn't reach).
var fsmv2BenthosMonitorEnabled bool

func init() {
	// required=false ⇒ never errors; silently falls back to the default (OFF).
	fsmv2BenthosMonitorEnabled, _ = env.GetAsBool("USE_FSMV2_BENTHOS_MONITOR", false, false)
}

// benthosMonitorWatcher is the seam the BenthosService FF-on paths call. The
// production default (defaultBenthosMonitorWatcher) delegates to the
// process-scoped fsmv2client; tests inject a fake via WithFSMv2BenthosWatcher.
//
// GetFresh returns the worker's published BenthosMonitorStatus{Scan, Stopped}
// (NOT a bare Scan — Scan has no Stopped field, and decoding the flat
// {scan,stopped} JSON into a bare Scan yields a zero Scan for every bridge,
// breaking v6.1c). Upsert/Delete drive the watcher's child-spec lifecycle.
type benthosMonitorWatcher interface {
	GetFresh(ctx context.Context, ref dynamicchildren.Ref, maxAge time.Duration) (bmworker.BenthosMonitorStatus, fsmv2client.Freshness, error)
	Upsert(ref dynamicchildren.Ref, cfg map[string]any) error
	Delete(ref dynamicchildren.Ref)
}

// defaultBenthosMonitorWatcher is the production implementation: it reaches the
// process-scoped fsmv2client singleton published at startup (inside the
// USE_FSMV2_TRANSPORT block, cmd/main.go:278/644). USE_FSMV2_BENTHOS_MONITOR=ON
// therefore REQUIRES USE_FSMV2_TRANSPORT=ON — if the client is nil (transport FF
// off / supervisor not built), every method fails soft (GetFresh → Unknown+err,
// Upsert → err, Delete → no-op) rather than dereferencing nil.
type defaultBenthosMonitorWatcher struct{}

func (defaultBenthosMonitorWatcher) GetFresh(ctx context.Context, ref dynamicchildren.Ref, maxAge time.Duration) (bmworker.BenthosMonitorStatus, fsmv2client.Freshness, error) {
	c := fsmv2client.GetClient()
	if c == nil {
		return bmworker.BenthosMonitorStatus{}, fsmv2client.Unknown, errors.New("fsmv2 client not initialized (USE_FSMV2_TRANSPORT off?)")
	}

	return fsmv2client.GetFresh[bmworker.BenthosMonitorStatus](ctx, c, ref, maxAge)
}

func (defaultBenthosMonitorWatcher) Upsert(ref dynamicchildren.Ref, cfg map[string]any) error {
	c := fsmv2client.GetClient()
	if c == nil {
		return errors.New("fsmv2 client not initialized (USE_FSMV2_TRANSPORT off?)")
	}

	return c.Upsert(ref, cfg)
}

func (defaultBenthosMonitorWatcher) Delete(ref dynamicchildren.Ref) {
	c := fsmv2client.GetClient()
	if c == nil {
		return
	}

	c.Delete(ref)
}
