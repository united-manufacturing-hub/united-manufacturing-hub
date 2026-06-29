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

// Package fsmv2nmap provides an FSMManager implementation for nmap connection
// scanning backed by the fsmv2 framework instead of S6 or suture.
//
// It is a drop-in replacement for both nmap.NmapManager and
// suturenmap.SutureNmapManager, toggled via the NMAP_BACKEND="fsmv2"
// environment variable.
//
// One application supervisor manages all nmap child workers. Target additions
// and removals update the supervisor's user spec; the supervisor reconciles
// children on the next tick without per-target goroutine overhead.
package fsmv2nmap

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	cseStorage "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/adapter"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	nmap_worker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// nmapChildrenConfig is the YAML envelope the ApplicationWorker parses in
// DeriveDesiredState. Each entry becomes a child nmap worker.
type nmapChildrenConfig struct {
	Children []fsmv2config.ChildSpec `yaml:"children"`
}

// NewFsmv2NmapManager creates a new FSMv2-backed nmap manager.
// A single application supervisor owns all nmap children; target set changes
// are reflected via UpdateUserSpec rather than spawning per-target supervisors.
func NewFsmv2NmapManager(name string) *adapter.WorkerManager[config.NmapConfig] {
	managerName := fmt.Sprintf("%s_%s", logger.ComponentNmapManager, name)

	ctx, cancel := context.WithCancel(context.Background())

	metrics.InitErrorCounter(metrics.ComponentNmapManager, name)

	fsmLog := deps.NewFSMLogger(zap.L().Sugar().Named(managerName))
	store := cseStorage.NewTriangularStore(memory.NewInMemoryStore(), fsmLog)

	sup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		Store:        store,
		Logger:       fsmLog,
		ID:           managerName,
		Name:         managerName,
		TickInterval: 100 * time.Millisecond,
	})
	if err != nil {
		// NewApplicationSupervisor only fails on identity collision, which
		// cannot happen with a fresh store. Panic so mis-wiring surfaces
		// immediately rather than silently dropping scans.
		panic(fmt.Sprintf("fsmv2nmap: create application supervisor: %v", err))
	}

	_ = sup.Start(ctx)

	log := logger.For(managerName)

	return adapter.NewWorkerManager(adapter.WorkerManagerSpec[config.NmapConfig]{
		ManagerName: managerName,
		Log:         log,
		UpdateSpec:  sup.UpdateUserSpec,
		ExtractConfigs: func(snapshot publicfsm.SystemSnapshot) []config.NmapConfig {
			return snapshot.CurrentConfig.Internal.Nmap
		},
		NameOf:         func(cfg config.NmapConfig) string { return cfg.Name },
		DesiredStateOf: func(cfg config.NmapConfig) string { return cfg.DesiredFSMState },
		ConfigEqual: func(a, b config.NmapConfig) bool {
			return a.FSMInstanceConfig == b.FSMInstanceConfig &&
				a.NmapServiceConfig.Equal(b.NmapServiceConfig)
		},
		NewInstance: func(cfg config.NmapConfig) publicfsm.FSMInstance {
			return newNmapInstance(cfg, store)
		},
		BuildSpec: buildNmapChildrenSpec,
	}, cancel)
}

// buildNmapChildrenSpec marshals the current domain config map into the YAML
// envelope that ApplicationWorker.DeriveDesiredState expects. Names are sorted
// so the YAML is deterministic and does not produce spurious UpdateUserSpec calls.
func buildNmapChildrenSpec(configs map[string]config.NmapConfig) fsmv2config.UserSpec {
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}

	sort.Strings(names)

	children := make([]fsmv2config.ChildSpec, 0, len(names))

	for _, name := range names {
		cfg := configs[name]

		spec, err := fsmv2config.NewChildSpec[nmap_worker.NmapConfig](
			name,
			"nmap",
			nmap_worker.NmapConfig{
				Target: cfg.NmapServiceConfig.Target,
				Port:   cfg.NmapServiceConfig.Port,
			},
			true,
		)
		if err != nil {
			zap.L().Sugar().Warnf("fsmv2nmap: failed to build child spec for %s: %v", name, err)

			continue
		}

		children = append(children, spec)
	}

	data, err := yaml.Marshal(nmapChildrenConfig{Children: children})
	if err != nil {
		zap.L().Sugar().Warnf("fsmv2nmap: failed to marshal nmap children spec: %v", err)

		return fsmv2config.UserSpec{}
	}

	return fsmv2config.UserSpec{Config: string(data)}
}
