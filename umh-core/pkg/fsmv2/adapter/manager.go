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
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// stoppedState is the desired-state literal that marks a config as disabled and
// serves as the default desired state's opposite: a config whose GetState()
// returns this is not Upserted into the fsmv2 runtime.
const stoppedState = "stopped"

// stateAccessor is the duck-typed desired-state seam. A domain config that
// carries a desired state satisfies it structurally, letting the manager derive
// each instance's desired state and the default enabled/disabled decision
// without importing the config package.
type stateAccessor interface {
	GetState() string
}

// WorkerManagerSpec describes the domain-specific behaviour WorkerManager needs
// to manage a fleet of fsmv2 child workers behind the fsmv1 FSMManager
// interface. The framework owns ref derivation, instance construction, and the
// Upsert/Delete diffing; the developer supplies the required mapping functions
// and may override the defaulted ones.
//
// TConfig is the domain config type flowing through the fsmv1 control loop.
// TStatus is the worker's stored status type (e.g. simple.Status[Raw]).
type WorkerManagerSpec[TConfig, TStatus any] struct {
	// ExtractConfigs pulls the relevant config slice from a SystemSnapshot.
	// Required.
	ExtractConfigs func(snapshot publicfsm.SystemSnapshot) []TConfig

	// NameOf returns the unique name for a config entry; it also becomes the
	// ref Name. Required.
	NameOf func(cfg TConfig) string

	// MapFresh maps a Fresh, healthy observation to a domain state string.
	// Required.
	MapFresh func(cfg TConfig, status TStatus) string

	// MapObserved maps an observation to the developer's ObservedState.
	// Required.
	MapObserved func(cfg TConfig, status TStatus) publicfsm.ObservedState

	// ConfigEqual reports whether two configs are semantically identical.
	// Optional; defaults to reflect.DeepEqual.
	ConfigEqual func(a, b TConfig) bool

	// CfgFor builds the map[string]any payload for Upsert from a config entry.
	// Optional; defaults to a JSON round-trip (TConfig → map).
	CfgFor func(cfg TConfig) (map[string]any, error)

	// IsEnabled reports whether a config entry should run in the fsmv2 runtime.
	// Disabled entries stay in the fsmv1 instance map (so their state is
	// visible) but are not Upserted; existing registrations are Deleted.
	// Optional; defaults to duck-typing the config's desired state
	// (GetState()=="stopped" => disabled, otherwise enabled).
	IsEnabled func(cfg TConfig) bool

	// Log is the FSMLogger; optional, defaults to a no-op logger.
	Log deps.FSMLogger

	// WorkerType builds the ref and names the manager. Required.
	WorkerType string

	// MinRequiredTime is passed to each built instance. Optional; defaults to 0.
	MinRequiredTime time.Duration
}

// WorkerManager is a generic fsmv1-compatible manager that drives a fleet of
// fsmv2 child workers via the global FSMv2Client (Upsert/Delete) and exposes
// each child as an AdaptedInstance. It satisfies publicfsm.FSMManager[TConfig].
type WorkerManager[TConfig, TStatus any] struct {
	instances     map[string]publicfsm.FSMInstance
	domainConfigs map[string]TConfig
	spec          WorkerManagerSpec[TConfig, TStatus]
	// mu guards instances, domainConfigs, and tick: Reconcile (control loop) and
	// CreateSnapshot / the getters (status reporting) can run on different
	// goroutines.
	mu   sync.RWMutex
	tick uint64
}

// NewWorkerManager builds a WorkerManager, filling every optional spec field
// with its default: ConfigEqual=reflect.DeepEqual, CfgFor=JSON round-trip,
// IsEnabled=desired-state duck-typing (GetState()=="stopped" => disabled), and
// Log=a no-op logger.
func NewWorkerManager[TConfig, TStatus any](spec WorkerManagerSpec[TConfig, TStatus]) *WorkerManager[TConfig, TStatus] {
	if spec.ConfigEqual == nil {
		spec.ConfigEqual = func(a, b TConfig) bool { return reflect.DeepEqual(a, b) }
	}

	if spec.CfgFor == nil {
		spec.CfgFor = defaultCfgFor[TConfig]
	}

	if spec.IsEnabled == nil {
		spec.IsEnabled = defaultIsEnabled[TConfig]
	}

	if spec.Log == nil {
		spec.Log = deps.NewNopFSMLogger()
	}

	return &WorkerManager[TConfig, TStatus]{
		spec:          spec,
		instances:     make(map[string]publicfsm.FSMInstance),
		domainConfigs: make(map[string]TConfig),
	}
}

// defaultCfgFor marshals a config to JSON and unmarshals it into a
// map[string]any, the default Upsert payload builder.
func defaultCfgFor[TConfig any](cfg TConfig) (map[string]any, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unmarshal config into map: %w", err)
	}

	return out, nil
}

// defaultIsEnabled duck-types the config's desired state: a config whose
// GetState() returns "stopped" is disabled, otherwise it is enabled. A config
// without a GetState() accessor is always enabled.
func defaultIsEnabled[TConfig any](cfg TConfig) bool {
	if sa, ok := any(cfg).(stateAccessor); ok {
		return sa.GetState() != stoppedState
	}

	return true
}

// desiredStateOf derives the instance desired state from the config's GetState()
// accessor, falling back to runningState when the config has no accessor or
// returns an empty string.
func desiredStateOf[TConfig any](cfg TConfig) string {
	if sa, ok := any(cfg).(stateAccessor); ok {
		if s := sa.GetState(); s != "" {
			return s
		}
	}

	return defaultDesiredState
}

// refFor builds the ref for a config entry from the worker type and its name.
func (m *WorkerManager[TConfig, TStatus]) refFor(cfg TConfig) dynamicchildren.Ref {
	return dynamicchildren.Ref{WorkerType: m.spec.WorkerType, Name: m.spec.NameOf(cfg)}
}

// buildInstance constructs an AdaptedInstance for a config entry.
func (m *WorkerManager[TConfig, TStatus]) buildInstance(cfg TConfig, enabled bool) publicfsm.FSMInstance {
	return newAdaptedInstance(
		m.refFor(cfg),
		cfg,
		desiredStateOf(cfg),
		m.spec.MinRequiredTime,
		m.spec.MapFresh,
		m.spec.MapObserved,
		!enabled,
		m.spec.Log,
	)
}

// GetInstances returns a snapshot copy of the managed instances. A copy (not the
// live map) so a caller can iterate it while Reconcile mutates the manager.
func (m *WorkerManager[TConfig, TStatus]) GetInstances() map[string]publicfsm.FSMInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]publicfsm.FSMInstance, len(m.instances))
	for name, inst := range m.instances {
		out[name] = inst
	}

	return out
}

// GetInstance returns an instance by name.
func (m *WorkerManager[TConfig, TStatus]) GetInstance(name string) (publicfsm.FSMInstance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	inst, ok := m.instances[name]

	return inst, ok
}

// GetManagerName returns the manager's worker type.
func (m *WorkerManager[TConfig, TStatus]) GetManagerName() string {
	return m.spec.WorkerType
}

// Reconcile diffs the desired configs from the snapshot against the current
// instance map: removed names are Deleted and dropped, new enabled names are
// Upserted, and changed names are re-Upserted (or Deleted when disabled). When
// the global client is nil (fsmv2 runtime not yet started) the instance map is
// still updated but no Upsert/Delete calls are made.
//
// On a CfgFor/Upsert failure while enabled, the manager logs a warning and skips
// advancing its stored config/instance for that name so the next Reconcile
// retries — in both the new-worker and config-change branches.
func (m *WorkerManager[TConfig, TStatus]) Reconcile(ctx context.Context, snapshot publicfsm.SystemSnapshot, _ serviceregistry.Provider) (error, bool) {
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.tick++
	client := fsmv2client.GetClient()

	desired := m.spec.ExtractConfigs(snapshot)
	changed := false

	m.spec.Log.Debug("reconcile", deps.Int("desired", len(desired)), deps.Int("instances", len(m.instances)), deps.Bool("clientReady", client != nil))

	desiredNames := make(map[string]struct{}, len(desired))
	for _, cfg := range desired {
		desiredNames[m.spec.NameOf(cfg)] = struct{}{}
	}

	// Remove workers no longer in config.
	for name, existing := range m.domainConfigs {
		if _, exists := desiredNames[name]; exists {
			continue
		}

		m.spec.Log.Debug("removing worker no longer in config", deps.String("worker", name))

		if client != nil {
			client.Delete(m.refFor(existing))
		}

		delete(m.instances, name)
		delete(m.domainConfigs, name)

		changed = true
	}

	// Add new workers and update changed ones.
	for _, cfg := range desired {
		name := m.spec.NameOf(cfg)
		existing, exists := m.domainConfigs[name]
		enabled := m.spec.IsEnabled(cfg)

		if !exists {
			if !m.applyDesired(client, cfg, name, enabled) {
				continue
			}

			m.instances[name] = m.buildInstance(cfg, enabled)
			m.domainConfigs[name] = cfg
			m.spec.Log.Debug("created worker", deps.String("worker", name), deps.Bool("enabled", enabled))

			changed = true

			continue
		}

		if m.spec.ConfigEqual(existing, cfg) {
			continue
		}

		if !m.applyDesired(client, cfg, name, enabled) {
			continue
		}

		m.instances[name] = m.buildInstance(cfg, enabled)
		m.domainConfigs[name] = cfg
		m.spec.Log.Debug("updated worker", deps.String("worker", name), deps.Bool("enabled", enabled))

		changed = true
	}

	return nil, changed
}

// applyDesired actuates the desired state for one config into the fsmv2 runtime:
// Upsert when enabled, Delete when disabled. It reports whether the caller may
// advance its stored config/instance for that name. A CfgFor/Upsert failure
// while enabled returns false (skip and retry next tick); a nil client, or any
// disabled/success path, returns true. The instance map is authoritative even
// when the client is nil.
func (m *WorkerManager[TConfig, TStatus]) applyDesired(client *fsmv2client.FSMv2Client, cfg TConfig, name string, enabled bool) bool {
	if client == nil {
		// A disabled entry needs no Upsert, so recording it now is correct even
		// without a client. An enabled entry must be Upserted once the client
		// appears; return false so it is retried every tick instead of being
		// recorded as done and orphaned when the client shows up later.
		return !enabled
	}

	if !enabled {
		client.Delete(m.refFor(cfg))

		return true
	}

	cfgMap, err := m.spec.CfgFor(cfg)
	if err != nil {
		m.spec.Log.SentryWarn(deps.FeatureForWorker(m.spec.WorkerType), name, "build spec failed, will retry next tick", deps.String("worker", name), deps.Err(err))

		return false
	}

	if err := client.Upsert(m.refFor(cfg), cfgMap); err != nil {
		m.spec.Log.SentryWarn(deps.FeatureForWorker(m.spec.WorkerType), name, "upsert failed, will retry next tick", deps.String("worker", name), deps.Err(err))

		return false
	}

	return true
}

// GetLastObservedState delegates to the named instance; a missing name errors.
func (m *WorkerManager[TConfig, TStatus]) GetLastObservedState(serviceName string) (publicfsm.ObservedState, error) {
	m.mu.RLock()
	inst, exists := m.instances[serviceName]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("instance %s not found", serviceName)
	}

	return inst.GetLastObservedState(), nil
}

// GetCurrentFSMState delegates to the named instance; a missing name errors.
func (m *WorkerManager[TConfig, TStatus]) GetCurrentFSMState(serviceName string) (string, error) {
	m.mu.RLock()
	inst, exists := m.instances[serviceName]
	m.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("instance %s not found", serviceName)
	}

	return inst.GetCurrentFSMState(), nil
}

// CreateSnapshot builds a ManagerSnapshot from the current instance states.
func (m *WorkerManager[TConfig, TStatus]) CreateSnapshot() publicfsm.ManagerSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snap := &workerManagerSnapshot{
		name:      m.spec.WorkerType,
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
func (m *WorkerManager[TConfig, TStatus]) Stop() {}

// workerManagerSnapshot implements publicfsm.ManagerSnapshot.
type workerManagerSnapshot struct {
	instances map[string]*publicfsm.FSMInstanceSnapshot
	snapTime  time.Time
	name      string
	tick      uint64
}

func (s *workerManagerSnapshot) GetName() string { return s.name }

func (s *workerManagerSnapshot) GetInstances() map[string]*publicfsm.FSMInstanceSnapshot {
	return s.instances
}

func (s *workerManagerSnapshot) GetSnapshotTime() time.Time { return s.snapTime }

func (s *workerManagerSnapshot) GetManagerTick() uint64 { return s.tick }

func (s *workerManagerSnapshot) IsObservedStateSnapshot() {}
