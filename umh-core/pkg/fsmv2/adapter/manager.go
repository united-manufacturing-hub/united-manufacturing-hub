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

package adapter

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// WorkerManagerSpec describes the domain-specific behaviour that WorkerManager
// needs to manage a fleet of fsmv2 child workers as fsmv1 FSMInstances.
//
// TDomainConfig is the domain config type that flows through the fsmv1 control
// loop (e.g. config.NmapConfig).
type WorkerManagerSpec[TDomainConfig any] struct {
	// ManagerName is used for logging and snapshot labels.
	ManagerName string

	// Log is the sugared logger for the manager.
	Log *zap.SugaredLogger

	// ExtractConfigs pulls the relevant config slice from a SystemSnapshot.
	ExtractConfigs func(snapshot publicfsm.SystemSnapshot) []TDomainConfig

	// NameOf returns the unique name for a config entry.
	NameOf func(cfg TDomainConfig) string

	// DesiredStateOf returns the desired FSM state string from a config entry.
	DesiredStateOf func(cfg TDomainConfig) string

	// ConfigEqual returns true when two configs are semantically identical.
	ConfigEqual func(a, b TDomainConfig) bool

	// NewInstance creates an FSMInstance adapter for a config entry.
	NewInstance func(cfg TDomainConfig) publicfsm.FSMInstance

	// RefFor builds the dynamicchildren.Ref for a config entry.
	RefFor func(cfg TDomainConfig) dynamicchildren.Ref

	// CfgFor builds the map[string]any payload for Upsert from a config entry.
	CfgFor func(cfg TDomainConfig) (map[string]any, error)
}

// WorkerManager is a generic fsmv1-compatible manager that drives a fleet of
// fsmv2 child workers via the global FSMv2Client (Upsert/Delete) and exposes
// each child as an FSMInstance adapter.
//
// It satisfies the superset of publicfsm.FSMManager[any] required by provider
// interfaces such as NmapManagerProvider.
type WorkerManager[TDomainConfig any] struct {
	spec          WorkerManagerSpec[TDomainConfig]
	instances     map[string]publicfsm.FSMInstance
	domainConfigs map[string]TDomainConfig
	tick          uint64
}

// NewWorkerManager creates a WorkerManager from spec.
func NewWorkerManager[TDomainConfig any](spec WorkerManagerSpec[TDomainConfig]) *WorkerManager[TDomainConfig] {
	return &WorkerManager[TDomainConfig]{
		spec:          spec,
		instances:     make(map[string]publicfsm.FSMInstance),
		domainConfigs: make(map[string]TDomainConfig),
	}
}

func (m *WorkerManager[TDomainConfig]) GetInstances() map[string]publicfsm.FSMInstance {
	return m.instances
}

func (m *WorkerManager[TDomainConfig]) GetInstance(n string) (publicfsm.FSMInstance, bool) {
	inst, ok := m.instances[n]
	return inst, ok
}

func (m *WorkerManager[TDomainConfig]) GetManagerName() string {
	return m.spec.ManagerName
}

// Reconcile diffs the desired configs from the snapshot against the current
// instance map, calling client.Upsert for new/changed entries and client.Delete
// for removed entries. When the global client is nil (fsmv2 runtime not yet
// started) the instance map is still updated but no Upsert/Delete calls are
// made.
func (m *WorkerManager[TDomainConfig]) Reconcile(ctx context.Context, snapshot publicfsm.SystemSnapshot, _ serviceregistry.Provider) (error, bool) {
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	m.tick++
	client := fsmv2client.GetClient()

	desired := m.spec.ExtractConfigs(snapshot)
	changed := false

	desiredNames := make(map[string]struct{}, len(desired))
	for _, cfg := range desired {
		desiredNames[m.spec.NameOf(cfg)] = struct{}{}
	}

	// Remove workers no longer in config.
	for name, existing := range m.domainConfigs {
		if _, exists := desiredNames[name]; !exists {
			m.spec.Log.Infof("Removing worker %s (no longer in config)", name)
			if client != nil {
				client.Delete(m.spec.RefFor(existing))
			}
			delete(m.instances, name)
			delete(m.domainConfigs, name)
			changed = true
		}
	}

	// Add new workers and update changed ones.
	for _, cfg := range desired {
		name := m.spec.NameOf(cfg)
		existing, exists := m.domainConfigs[name]
		enabled := m.spec.DesiredStateOf(cfg) != "stopped"

		if !exists {
			if client != nil {
				if enabled {
					cfgMap, err := m.spec.CfgFor(cfg)
					if err != nil {
						m.spec.Log.Warnf("fsmv2: build spec for %s: %v", name, err)
					} else if err := client.Upsert(m.spec.RefFor(cfg), cfgMap); err != nil {
						m.spec.Log.Warnf("fsmv2: upsert %s: %v", name, err)
					}
				}
				// stopped configs are never registered in the fsmv2 runtime
			}
			m.instances[name] = m.spec.NewInstance(cfg)
			m.domainConfigs[name] = cfg
			m.spec.Log.Infof("Created worker %s (enabled=%v)", name, enabled)
			changed = true
			continue
		}

		if !m.spec.ConfigEqual(existing, cfg) {
			if client != nil {
				if enabled {
					cfgMap, err := m.spec.CfgFor(cfg)
					if err != nil {
						m.spec.Log.Warnf("fsmv2: build spec for %s: %v", name, err)
					} else if err := client.Upsert(m.spec.RefFor(cfg), cfgMap); err != nil {
						m.spec.Log.Warnf("fsmv2: upsert %s: %v", name, err)
					}
				} else {
					client.Delete(m.spec.RefFor(cfg))
				}
			}
			m.instances[name] = m.spec.NewInstance(cfg)
			m.domainConfigs[name] = cfg
			changed = true
		}
	}

	return nil, changed
}

func (m *WorkerManager[TDomainConfig]) GetLastObservedState(serviceName string) (publicfsm.ObservedState, error) {
	if inst, exists := m.instances[serviceName]; exists {
		return inst.GetLastObservedState(), nil
	}
	return nil, fmt.Errorf("instance %s not found", serviceName)
}

func (m *WorkerManager[TDomainConfig]) GetCurrentFSMState(serviceName string) (string, error) {
	if inst, exists := m.instances[serviceName]; exists {
		return inst.GetCurrentFSMState(), nil
	}
	return "", fmt.Errorf("instance %s not found", serviceName)
}

// CreateSnapshot builds a ManagerSnapshot from current instance states.
func (m *WorkerManager[TDomainConfig]) CreateSnapshot() publicfsm.ManagerSnapshot {
	snap := &workerManagerSnapshot{
		name:      m.spec.ManagerName,
		instances: make(map[string]*publicfsm.FSMInstanceSnapshot, len(m.instances)),
		snapTime:  time.Now(),
		tick:      m.tick,
	}
	for name, inst := range m.instances {
		snap.instances[name] = &publicfsm.FSMInstanceSnapshot{
			ID:           name,
			CurrentState: inst.GetCurrentFSMState(),
			DesiredState: inst.GetDesiredFSMState(),
		}
	}
	return snap
}

// Stop is a no-op: the fsmv2 global runtime is managed externally.
func (m *WorkerManager[TDomainConfig]) Stop() {}

// workerManagerSnapshot implements publicfsm.ManagerSnapshot.
type workerManagerSnapshot struct {
	name      string
	instances map[string]*publicfsm.FSMInstanceSnapshot
	tick      uint64
	snapTime  time.Time
}

func (s *workerManagerSnapshot) GetName() string { return s.name }
func (s *workerManagerSnapshot) GetInstances() map[string]*publicfsm.FSMInstanceSnapshot {
	return s.instances
}
func (s *workerManagerSnapshot) GetSnapshotTime() time.Time { return s.snapTime }
func (s *workerManagerSnapshot) GetManagerTick() uint64     { return s.tick }
func (s *workerManagerSnapshot) IsObservedStateSnapshot()   {}
