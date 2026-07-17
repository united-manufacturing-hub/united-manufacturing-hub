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

// Package configworker provides the config worker, the kernel child of the
// application worker.
//
// Today the worker anchors the application's children union: once the shared
// registry is published, every application state emits this kernel child
// (renderUnion in workers/application/state), so the children list can never
// become empty. It also reconciles the historian monitor child on every tick:
// CollectObservedState polls the config manager, then upserts or deletes the
// historian child in the dynamicchildren registry so live config edits apply
// without restarting umh-core. Other child specs still reach the registry via
// fsmv2client and its Writer, and the application's collector reads the
// registry directly.
//
// TODO(ENG-4400): this worker becomes the config.yaml authority. It reads
// config.yaml, validates and serializes changes to it, and materializes the
// declared children into the dynamicchildren registry - the registry then
// becomes the merged view of config-declared and runtime-requested children,
// and spec writes move from direct Writer calls into this worker's tick. The
// historian reconcile below is the first consumer of that path; later it reads
// config.yaml directly instead of polling the config manager.
// TODO(ENG-4900): dynamicchildren.WithValidate is the hook for rejecting
// invalid specs at Upsert; nothing installs a validator today.
package configworker

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	fsmv2timescale "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/historian"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/snapshot"

	// Blank-import the state subpackage so its init() registers the initial
	// state before the supervisor ticks the worker.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/state"
)

// WorkerTypeName is the canonical worker-type identifier for the config
// worker, registered in init() and used as the folder name. Exported (same
// convention as the persistence worker) so the application worker can read
// the shared registry published under this deps key.
const WorkerTypeName = "configworker"

// ConfigManagerDepsKey is the register.SetDeps key under which parent wiring
// publishes the config.ConfigManager the worker polls each tick. It is
// distinct from WorkerTypeName (which holds the shared registry) because the
// typed deps registry keys on the string alone, so the two payloads cannot
// share a key.
//
// TODO(ENG-4400): temporary. Once the worker reads config.yaml directly it owns
// the file, no longer polls the manager, and this key and its wiring in
// cmd/main.go go away.
const ConfigManagerDepsKey = WorkerTypeName + ".configmanager"

// ConfigworkerWorker implements the FSMv2 Worker interface and holds a handle
// to the shared dynamicchildren registry. See the package doc for why it does
// nothing else yet.
type ConfigworkerWorker struct {
	registry *dynamicchildren.Registry
	// configManager is the interim config source polled each tick. It is nil on
	// FF-off paths and in unit tests. TODO(ENG-4400): drops out when the worker
	// reads config.yaml directly (see package doc and ConfigManagerDepsKey).
	configManager config.ConfigManager

	fsmv2.WorkerBase[snapshot.ConfigworkerConfig, snapshot.ConfigworkerStatus, register.NoDeps]
}

// NewConfigworkerWorker creates a config worker holding the registry published
// under WorkerTypeName via register.SetDeps. It fails when no registry was published,
// surfacing the missing wiring at construction instead of at the first registry
// read (a nil *Registry would otherwise panic on a method call far from the cause).
func NewConfigworkerWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*ConfigworkerWorker, error) {
	shared := register.GetDeps[*dynamicchildren.Registry](WorkerTypeName)
	if shared == nil {
		return nil, fmt.Errorf("no registry published for worker type %q: call register.SetDeps before constructing", WorkerTypeName)
	}

	// The config manager is optional: FF-off paths and unit tests never publish
	// one, and the historian reconcile in CollectObservedState no-ops when it is
	// nil (mirroring the fsmv2client.GetClient nil guard).
	configManager := register.GetDeps[config.ConfigManager](ConfigManagerDepsKey)

	w := &ConfigworkerWorker{
		registry:      shared,
		configManager: configManager,
	}
	w.InitBase(identity, logger, stateReader)

	return w, nil
}

// Registry returns the shared registry the worker holds.
func (w *ConfigworkerWorker) Registry() *dynamicchildren.Registry {
	return w.registry
}

// GetDependenciesAny returns nil so the framework skips metrics injection for
// this no-deps worker (a boxed struct{}{} would be non-nil).
func (w *ConfigworkerWorker) GetDependenciesAny() any {
	return nil
}

// CollectObservedState reconciles the historian monitor child from the live
// config, then returns the observed state. It reads config through the config
// manager and upserts or deletes the historian child in the shared registry so
// live edits apply without a restart. Reconcile failures are logged, not
// returned: a transient config read or upsert error must not fail the tick and
// stall the worker.
func (w *ConfigworkerWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	w.reconcileHistorian(ctx)

	return fsmv2.NewObservation(snapshot.ConfigworkerStatus{}), nil
}

// reconcileHistorian reads the live config and syncs the historian monitor
// child through the process-scoped fsmv2client. It no-ops when the config
// manager was not published or the client is not yet set, and logs (rather
// than returns) read and upsert errors so a transient failure never fails the
// tick.
func (w *ConfigworkerWorker) reconcileHistorian(ctx context.Context) {
	if w.configManager == nil {
		return
	}

	client := fsmv2client.GetClient()
	if client == nil {
		return
	}

	cfg, err := w.configManager.GetConfig(ctx, 0)
	if err != nil {
		w.Logger().SentryWarn(deps.FeatureSupportHistorian, w.Identity().HierarchyPath,
			"config watch: failed to read config", deps.Err(err))

		return
	}

	if err := syncHistorian(client, cfg); err != nil {
		w.Logger().SentryWarn(deps.FeatureSupportHistorian, w.Identity().HierarchyPath,
			"historian watch: upsert failed", deps.Err(err))
	}
}

// syncHistorian upserts the historian monitor child with the current settings
// when the historian section is present, or deletes it when absent. It returns
// the upsert error so the caller can log it.
func syncHistorian(client *fsmv2client.FSMv2Client, cfg config.FullConfig) error {
	if cfg.Historian == nil {
		client.Delete(fsmv2timescale.Ref)

		return nil
	}

	return client.Upsert(fsmv2timescale.Ref, cfg.Historian.ToTemplateMap())
}

func init() {
	register.Worker[snapshot.ConfigworkerConfig, snapshot.ConfigworkerStatus, register.NoDeps](WorkerTypeName,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			return NewConfigworkerWorker(id, logger, sr)
		})
}

// ensure ConfigworkerWorker implements Worker interfaces (compile-time check).
var _ fsmv2.Worker = (*ConfigworkerWorker)(nil)
