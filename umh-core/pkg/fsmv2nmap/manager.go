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
// The shared global fsmv2 runtime owns child execution. This manager is a thin
// control-loop adapter: it diffs desired configs, calls Upsert/Delete on the
// global FSMv2Client, and exposes each child as an FSMv1 FSMInstance.
package fsmv2nmap

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/adapter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

// NewFsmv2NmapManager creates a new FSMv2-backed nmap manager.
// Child execution is owned by the shared global fsmv2 runtime (wired in
// cmd/main.go via fsmv2client.SetClient). This manager only reconciles config
// changes into Upsert/Delete calls on that runtime.
func NewFsmv2NmapManager(name string) *adapter.WorkerManager[config.NmapConfig] {
	managerName := fmt.Sprintf("%s_%s", logger.ComponentNmapManager, name)
	metrics.InitErrorCounter(metrics.ComponentNmapManager, name)
	log := logger.For(managerName)

	return adapter.NewWorkerManager(adapter.WorkerManagerSpec[config.NmapConfig]{
		ManagerName: managerName,
		Log:         log,
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
			return newNmapInstance(cfg)
		},
		RefFor: func(cfg config.NmapConfig) dynamicchildren.Ref {
			return dynamicchildren.Ref{WorkerType: "nmap", Name: cfg.Name}
		},
		CfgFor: func(cfg config.NmapConfig) (map[string]any, error) {
			return map[string]any{
				"target": cfg.NmapServiceConfig.Target,
				"port":   cfg.NmapServiceConfig.Port,
			}, nil
		},
	})
}
