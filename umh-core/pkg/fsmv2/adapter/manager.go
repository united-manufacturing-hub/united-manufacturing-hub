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
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
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

	// UpdateSpec pushes a new children spec to the ApplicationSupervisor.
	// Typically: func(spec) { sup.UpdateUserSpec(spec) }
	UpdateSpec func(fsmv2config.UserSpec)

	// ExtractConfigs pulls the relevant config slice from a SystemSnapshot.
	ExtractConfigs func(snapshot publicfsm.SystemSnapshot) []TDomainConfig

	// NameOf returns the unique name for a config entry.
	NameOf func(cfg TDomainConfig) string

	// DesiredStateOf returns the desired FSM state string from a config entry.
	DesiredStateOf func(cfg TDomainConfig) string

	// ConfigEqual returns true when two configs are semantically identical.
	ConfigEqual func(a, b TDomainConfig) bool

	// NewInstance creates an FSMInstance adapter for a config entry.
	// The implementation typically calls adapter.NewAdaptedInstance with the
	// domain-specific mapper functions.
	NewInstance func(cfg TDomainConfig) publicfsm.FSMInstance

	// BuildSpec marshals the current domain config map into the YAML envelope
	// that ApplicationWorker.DeriveDesiredState expects.
	BuildSpec func(configs map[string]TDomainConfig) fsmv2config.UserSpec
}

// WorkerManager is a generic fsmv1-compatible manager that owns a fleet of
// fsmv2 child workers, each wrapped in an FSMInstance adapter.
//
// It satisfies the superset of publicfsm.FSMManager[any] required by provider
// interfaces such as NmapManagerProvider.
type WorkerManager[TDomainConfig any] struct {
	spec          WorkerManagerSpec[TDomainConfig]
	instances     map[string]publicfsm.FSMInstance
	domainConfigs map[string]TDomainConfig
	cancel        context.CancelFunc
}

// NewWorkerManager creates a WorkerManager from spec.
// cancel is called by Stop() to cancel the supervisor's context.
func NewWorkerManager[TDomainConfig any](
	spec WorkerManagerSpec[TDomainConfig],
	cancel context.CancelFunc,
) *WorkerManager[TDomainConfig] {
	return &WorkerManager[TDomainConfig]{
		spec:          spec,
		instances:     make(map[string]publicfsm.FSMInstance),
		domainConfigs: make(map[string]TDomainConfig),
		cancel:        cancel,
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

// Reconcile syncs the instance map to the desired configs from the snapshot,
// then pushes an updated children spec to the supervisor when anything changed.
func (m *WorkerManager[TDomainConfig]) Reconcile(ctx context.Context, snapshot publicfsm.SystemSnapshot, _ serviceregistry.Provider) (error, bool) {
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	desired := m.spec.ExtractConfigs(snapshot)
	changed := false

	desiredNames := make(map[string]struct{}, len(desired))
	for _, cfg := range desired {
		desiredNames[m.spec.NameOf(cfg)] = struct{}{}
	}

	// Remove workers no longer in config.
	for name := range m.instances {
		if _, exists := desiredNames[name]; !exists {
			m.spec.Log.Infof("Removing worker %s (no longer in config)", name)
			delete(m.instances, name)
			delete(m.domainConfigs, name)

			changed = true
		}
	}

	// Add new workers and update changed ones.
	for _, cfg := range desired {
		name := m.spec.NameOf(cfg)
		existing, exists := m.domainConfigs[name]

		if !exists {
			m.instances[name] = m.spec.NewInstance(cfg)
			m.domainConfigs[name] = cfg
			m.spec.Log.Infof("Created worker %s", name)

			changed = true

			continue
		}

		if !m.spec.ConfigEqual(existing, cfg) {
			m.instances[name] = m.spec.NewInstance(cfg)
			m.domainConfigs[name] = cfg

			changed = true
		}
	}

	if changed {
		m.spec.UpdateSpec(m.spec.BuildSpec(m.domainConfigs))
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

// Stop cancels the supervisor's context.
func (m *WorkerManager[TDomainConfig]) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

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
