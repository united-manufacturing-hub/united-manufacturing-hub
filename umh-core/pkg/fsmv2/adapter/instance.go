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
// per worker type.
package adapter

import (
	"context"
	"time"

	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// storeReadTimeout bounds how long getFreshObs blocks on a CSE store read.
// The StateReader non-blocking contract requires a deadline-bounded context.
const storeReadTimeout = 100 * time.Millisecond

// AdaptedInstance wraps a fsmv2 child worker observed via the global FSMv2Client
// and satisfies publicfsm.FSMInstance.
//
// All lifecycle methods (CreateInstance, RemoveInstance, etc.) are no-ops
// because fsmv2 manages its own lifecycle through the shared global runtime.
// The instance reads state via GetFreshObs, distinguishing Unregistered,
// NeverObserved, Stale, and Fresh so mappers can produce clean FSMv1 states.
//
// TStatus is the worker's status type (e.g. nmap_worker.NmapStatus).
// TDomainConfig is the domain config type flowing through the fsmv1 control
// loop (e.g. config.NmapConfig).
type AdaptedInstance[TStatus any, TDomainConfig any] struct {
	// mapState maps an observation + freshness reason to an FSMv1 state string.
	// Unregistered/NeverObserved → return a "starting" equivalent.
	// Stale → return a "degraded" equivalent.
	// Fresh → inspect status fields for the actual port/process state.
	mapState func(fsmv2.Observation[TStatus], fsmv2client.Freshness, TDomainConfig) string

	// mapObservedState builds the full FSMv1 ObservedState from an observation.
	mapObservedState func(TDomainConfig, fsmv2.Observation[TStatus], fsmv2client.Freshness) publicfsm.ObservedState

	domainConfig    TDomainConfig
	ref             dynamicchildren.Ref
	desiredState    string
	minRequiredTime time.Duration
	// maxAge is the observation age threshold for Stale classification.
	// Kept separate from minRequiredTime: minRequiredTime is an fsmv1 concept
	// (how long before a state change is considered stable); maxAge is an
	// fsmv2 read concept (how old an observation can be before the child is
	// considered unreachable).
	maxAge time.Duration
}

// NewAdaptedInstance creates an AdaptedInstance. ref must match the Ref used in
// Upsert/Delete calls so reads resolve to the correct child observation.
// minRequiredTime and maxAge serve distinct purposes and should be set
// independently (see field docs above).
func NewAdaptedInstance[TStatus any, TDomainConfig any](
	ref dynamicchildren.Ref,
	cfg TDomainConfig,
	desiredState string,
	minRequiredTime time.Duration,
	maxAge time.Duration,
	mapState func(fsmv2.Observation[TStatus], fsmv2client.Freshness, TDomainConfig) string,
	mapObs func(TDomainConfig, fsmv2.Observation[TStatus], fsmv2client.Freshness) publicfsm.ObservedState,
) *AdaptedInstance[TStatus, TDomainConfig] {
	return &AdaptedInstance[TStatus, TDomainConfig]{
		ref:              ref,
		domainConfig:     cfg,
		desiredState:     desiredState,
		minRequiredTime:  minRequiredTime,
		maxAge:           maxAge,
		mapState:         mapState,
		mapObservedState: mapObs,
	}
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) getFreshObs() (fsmv2.Observation[TStatus], fsmv2client.Freshness) {
	c := fsmv2client.GetClient()
	if c == nil {
		return fsmv2.Observation[TStatus]{}, fsmv2client.Unregistered
	}

	ctx, cancel := context.WithTimeout(context.Background(), storeReadTimeout)
	defer cancel()

	obs, freshness, err := fsmv2client.GetFreshObs[TStatus](ctx, c, i.ref, i.maxAge)
	if err != nil {
		return fsmv2.Observation[TStatus]{}, fsmv2client.Unknown
	}

	return obs, freshness
}

// --- publicfsm.FSMInstance implementation ---

func (i *AdaptedInstance[TStatus, TDomainConfig]) GetCurrentFSMState() string {
	obs, freshness := i.getFreshObs()
	return i.mapState(obs, freshness, i.domainConfig)
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) GetDesiredFSMState() string {
	return i.desiredState
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) SetDesiredFSMState(s string) error {
	i.desiredState = s
	return nil
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) IsTransientStreakCounterMaxed() bool {
	return false
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) Reconcile(_ context.Context, _ publicfsm.SystemSnapshot, _ serviceregistry.Provider) (error, bool) {
	return nil, false
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) Remove(_ context.Context) error {
	if c := fsmv2client.GetClient(); c != nil {
		c.Delete(i.ref)
	}

	return nil
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) GetLastObservedState() publicfsm.ObservedState {
	obs, freshness := i.getFreshObs()
	return i.mapObservedState(i.domainConfig, obs, freshness)
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) GetMinimumRequiredTime() time.Duration {
	return i.minRequiredTime
}

// --- publicfsm.FSMInstanceActions no-ops ---

func (i *AdaptedInstance[TStatus, TDomainConfig]) CreateInstance(_ context.Context, _ filesystem.Service) error {
	return nil
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) RemoveInstance(_ context.Context, _ filesystem.Service) error {
	return nil
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) StartInstance(_ context.Context, _ filesystem.Service) error {
	return nil
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) StopInstance(_ context.Context, _ filesystem.Service) error {
	return nil
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) UpdateObservedStateOfInstance(_ context.Context, _ serviceregistry.Provider, _ publicfsm.SystemSnapshot) error {
	return nil
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) CheckForCreation(_ context.Context, _ filesystem.Service) bool {
	return true
}

var _ publicfsm.FSMInstance = (*AdaptedInstance[struct{}, struct{}])(nil)
