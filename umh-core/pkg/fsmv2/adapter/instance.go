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

// Package adapter bridges fsmv2 workers to the fsmv1 FSMInstance / FSMManager
// interfaces so that fsmv2-backed components can be used as drop-in
// replacements inside the existing control loop without writing boilerplate
// per worker type. The framework owns state resolution; the developer supplies
// only the Fresh-case mapping.
package adapter

import (
	"context"
	"errors"
	"sync"
	"time"

	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

const (
	// storeReadTimeout bounds how long getFreshObs blocks on a CSE store read.
	// The StateReader non-blocking contract requires a deadline-bounded context.
	storeReadTimeout = 100 * time.Millisecond

	// unregisteredStaleFallback is the staleAfter window applied when the worker
	// type has no registered observation interval. Hardcoded (rather than reading
	// the supervisor's DefaultObservationInterval) to keep the adapter cycle-free.
	unregisteredStaleFallback = 1 * time.Second

	// runningState and degradedState are the hardcoded fsmv1 state literals the
	// resolution ladder returns outside the Fresh+healthy leaf. Drawn from the
	// {running, degraded, stopped} vocabulary — there is no "starting" state.
	runningState  = "running"
	degradedState = "degraded"
)

// HealthReporter is the verdict-reading seam the adapter owns. The stored status
// type (TStatus) satisfies it structurally when it carries a health verdict
// (e.g. simple.Status[T]). The adapter imports nothing from simple: it reads the
// verdict off the stored status via a type assertion to this interface.
type HealthReporter interface {
	HealthVerdict() (degraded bool, reason string)
}

// AdaptedInstance wraps an fsmv2 child worker observed via the global
// FSMv2Client and satisfies publicfsm.FSMInstance.
//
// All lifecycle methods (CreateInstance, RemoveInstance, etc.) are no-ops: an
// fsmv2 worker manages its own lifecycle through the shared global runtime. The
// instance reads state via GetFreshObs and resolves it through a fixed
// framework-owned ladder; the developer supplies only mapFresh (the Fresh +
// healthy leaf) and mapObserved (the full ObservedState).
//
// TConfig is the domain config type flowing through the fsmv1 control loop.
// TStatus is the worker's stored status type (e.g. simple.Status[Raw]).
type AdaptedInstance[TConfig, TStatus any] struct {
	mapFresh    func(cfg TConfig, status TStatus) string
	mapObserved func(cfg TConfig, status TStatus) publicfsm.ObservedState

	cfg TConfig

	ref          dynamicchildren.Ref
	desiredState string

	// lastState caches the most recently resolved state so a single read hiccup
	// (Unknown) holds the last known state instead of flapping a healthy worker.
	// Guarded by mu because the fsmv1 control loop may call GetCurrentFSMState
	// concurrently with other reads.
	lastState string

	minRequiredTime time.Duration
	// staleAfter is the observation-age threshold for Stale classification:
	// 3 × the worker type's registered observation interval, or the 1s fallback
	// when unregistered. Framework-owned, not a developer knob.
	staleAfter time.Duration

	mu sync.Mutex

	// isDisabled is set when the worker is deliberately disabled (not running).
	// GetCurrentFSMState returns desiredState directly so the fsmv1 control loop
	// sees current==desired and does not wait for a state transition.
	isDisabled bool
}

// newAdaptedInstance builds an AdaptedInstance. ref must match the Ref used in
// Upsert/Delete so reads resolve to the correct child observation. Construction
// is unexported: the WorkerManager builds instances internally from a spec.
func newAdaptedInstance[TConfig, TStatus any](
	ref dynamicchildren.Ref,
	cfg TConfig,
	desiredState string,
	minRequiredTime time.Duration,
	mapFresh func(cfg TConfig, status TStatus) string,
	mapObserved func(cfg TConfig, status TStatus) publicfsm.ObservedState,
	isDisabled bool,
) *AdaptedInstance[TConfig, TStatus] {
	return &AdaptedInstance[TConfig, TStatus]{
		ref:             ref,
		cfg:             cfg,
		desiredState:    desiredState,
		minRequiredTime: minRequiredTime,
		staleAfter:      staleAfterFor(ref.WorkerType),
		mapFresh:        mapFresh,
		mapObserved:     mapObserved,
		isDisabled:      isDisabled,
	}
}

// staleAfterFor returns 3 × the worker type's registered observation interval,
// falling back to unregisteredStaleFallback when the type has no registered
// interval.
func staleAfterFor(workerType string) time.Duration {
	if interval, ok := fsmv2.ObservationIntervalFor(workerType); ok {
		return 3 * interval
	}

	return unregisteredStaleFallback
}

// getFreshObs reads the child observation for ref, bounded by storeReadTimeout.
// A nil client (FF-off) or any read error maps to Unknown so the caller can hold
// the last known state instead of flapping.
func (i *AdaptedInstance[TConfig, TStatus]) getFreshObs() (fsmv2.Observation[TStatus], fsmv2client.Freshness) {
	c := fsmv2client.GetClient()
	if c == nil {
		return fsmv2.Observation[TStatus]{}, fsmv2client.Unknown
	}

	ctx, cancel := context.WithTimeout(context.Background(), storeReadTimeout)
	defer cancel()

	obs, freshness, err := fsmv2client.GetFreshObs[TStatus](ctx, c, i.ref, i.staleAfter)
	if err != nil {
		return fsmv2.Observation[TStatus]{}, fsmv2client.Unknown
	}

	return obs, freshness
}

// --- publicfsm.FSMInstance implementation ---

// GetCurrentFSMState resolves the fsmv1 state via the framework-owned ladder.
// Output ∈ {running, degraded, stopped} ∪ mapFresh domain states. Every resolved
// state (except the isDisabled shortcut) is cached into lastState.
func (i *AdaptedInstance[TConfig, TStatus]) GetCurrentFSMState() string {
	// Rung 1: disabled → desired state directly, without reading the client.
	if i.isDisabled {
		return i.desiredState
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	obs, freshness := i.getFreshObs()
	resolved := i.resolve(obs, freshness)
	i.lastState = resolved

	return resolved
}

// resolve implements rungs 2–6 of the ladder. mu is held by the caller.
func (i *AdaptedInstance[TConfig, TStatus]) resolve(obs fsmv2.Observation[TStatus], freshness fsmv2client.Freshness) string {
	// Rung 2: Unknown (nil client / read error) → hold last known state.
	if freshness == fsmv2client.Unknown {
		if i.lastState == "" {
			return runningState
		}

		return i.lastState
	}

	// Rung 3: degraded verdict on the stored status → degraded.
	if hr, ok := any(obs.Status).(HealthReporter); ok {
		if degraded, _ := hr.HealthVerdict(); degraded {
			return degradedState
		}
	}

	// Rung 4: bootstrap (no observation yet) → running, not "starting".
	if freshness == fsmv2client.Unregistered || freshness == fsmv2client.NeverObserved {
		return runningState
	}

	// Rung 5: stale (missed polls) → degraded.
	if freshness == fsmv2client.Stale {
		return degradedState
	}

	// Rung 6: Fresh + not degraded → developer's Fresh-case mapping.
	return i.mapFresh(i.cfg, obs.Status)
}

// GetDesiredFSMState returns the configured desired state.
func (i *AdaptedInstance[TConfig, TStatus]) GetDesiredFSMState() string {
	return i.desiredState
}

// SetDesiredFSMState always errors: the desired state is driven by config, not
// set imperatively through the fsmv1 control loop.
func (i *AdaptedInstance[TConfig, TStatus]) SetDesiredFSMState(_ string) error {
	return errors.New("adapter: desired state is driven by config, not set imperatively")
}

// IsTransientStreakCounterMaxed always returns false: the adapter has no
// transient-streak concept.
func (i *AdaptedInstance[TConfig, TStatus]) IsTransientStreakCounterMaxed() bool {
	return false
}

// Reconcile is a no-op: the fsmv2 worker reconciles itself in its own runtime.
func (i *AdaptedInstance[TConfig, TStatus]) Reconcile(_ context.Context, _ publicfsm.SystemSnapshot, _ serviceregistry.Provider) (error, bool) {
	return nil, false
}

// Remove deletes the ref from the shared registry, despawning the fsmv2 child.
func (i *AdaptedInstance[TConfig, TStatus]) Remove(_ context.Context) error {
	if c := fsmv2client.GetClient(); c != nil {
		c.Delete(i.ref)
	}

	return nil
}

// GetLastObservedState maps the observation to the developer's ObservedState.
// It always delegates to mapObserved so the returned concrete type is the same
// on every tick: on a non-Fresh read getFreshObs yields a zero Observation, so
// mapObserved receives a zero status and produces the developer's type with
// empty content rather than a foreign framework type a consumer's type
// assertion would trip on. mapObserved must tolerate a zero status.
func (i *AdaptedInstance[TConfig, TStatus]) GetLastObservedState() publicfsm.ObservedState {
	obs, _ := i.getFreshObs()

	return i.mapObserved(i.cfg, obs.Status)
}

// GetMinimumRequiredTime returns the configured minimum required time.
func (i *AdaptedInstance[TConfig, TStatus]) GetMinimumRequiredTime() time.Duration {
	return i.minRequiredTime
}

// --- publicfsm.FSMInstanceActions no-ops (monitor-only) ---

// CreateInstance is a no-op: the fsmv2 runtime owns instance creation.
func (i *AdaptedInstance[TConfig, TStatus]) CreateInstance(_ context.Context, _ filesystem.Service) error {
	return nil
}

// RemoveInstance is a no-op: removal is actuated by Remove.
func (i *AdaptedInstance[TConfig, TStatus]) RemoveInstance(_ context.Context, _ filesystem.Service) error {
	return nil
}

// StartInstance is a no-op: the fsmv2 runtime owns lifecycle.
func (i *AdaptedInstance[TConfig, TStatus]) StartInstance(_ context.Context, _ filesystem.Service) error {
	return nil
}

// StopInstance is a no-op: the fsmv2 runtime owns lifecycle.
func (i *AdaptedInstance[TConfig, TStatus]) StopInstance(_ context.Context, _ filesystem.Service) error {
	return nil
}

// UpdateObservedStateOfInstance is a no-op: observations are read on demand via
// getFreshObs, not pushed by the control loop.
func (i *AdaptedInstance[TConfig, TStatus]) UpdateObservedStateOfInstance(_ context.Context, _ serviceregistry.Provider, _ publicfsm.SystemSnapshot) error {
	return nil
}

// CheckForCreation always returns true: the fsmv2 child is always eligible.
func (i *AdaptedInstance[TConfig, TStatus]) CheckForCreation(_ context.Context, _ filesystem.Service) bool {
	return true
}

var _ publicfsm.FSMInstance = (*AdaptedInstance[struct{}, struct{}])(nil)
