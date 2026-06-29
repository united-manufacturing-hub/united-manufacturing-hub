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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// AdaptedInstance wraps a fsmv2 child worker observed via a shared StateReader
// and satisfies publicfsm.FSMInstance.
//
// All lifecycle methods (CreateInstance, RemoveInstance, etc.) are no-ops
// because fsmv2 manages its own lifecycle through the ApplicationSupervisor.
// Callers supply MapState and MapObservedState to translate fsmv2 observations
// into the legacy state vocabulary the rest of the system expects.
//
// TStatus is the worker's status type (e.g. simple.Status[NmapStatus]).
// TDomainConfig is the domain config type that flows through the fsmv1 control
// loop (e.g. config.NmapConfig).
type AdaptedInstance[TStatus any, TDomainConfig any] struct {
	sr               deps.StateReader
	mapState         func(fsmv2.Observation[TStatus], TDomainConfig) string
	mapObservedState func(TDomainConfig, fsmv2.Observation[TStatus]) publicfsm.ObservedState
	domainConfig     TDomainConfig
	workerType       string
	childID          string
	desiredState     string
	minRequiredTime  time.Duration
}

// NewAdaptedInstance creates an AdaptedInstance. workerType and childID must
// match the values used when building the ApplicationSupervisor's children spec.
func NewAdaptedInstance[TStatus any, TDomainConfig any](
	sr deps.StateReader,
	workerType, childID string,
	cfg TDomainConfig,
	desiredState string,
	minTime time.Duration,
	mapState func(fsmv2.Observation[TStatus], TDomainConfig) string,
	mapObs func(TDomainConfig, fsmv2.Observation[TStatus]) publicfsm.ObservedState,
) *AdaptedInstance[TStatus, TDomainConfig] {
	return &AdaptedInstance[TStatus, TDomainConfig]{
		sr:               sr,
		workerType:       workerType,
		childID:          childID,
		domainConfig:     cfg,
		desiredState:     desiredState,
		minRequiredTime:  minTime,
		mapState:         mapState,
		mapObservedState: mapObs,
	}
}

// UpdateDomainConfig replaces the stored domain config without touching the
// childID or mappers. Call this when config values change but the identity
// (name/workerType) stays the same.
func (i *AdaptedInstance[TStatus, TDomainConfig]) UpdateDomainConfig(cfg TDomainConfig, desiredState string) {
	i.domainConfig = cfg
	i.desiredState = desiredState
}

// --- publicfsm.FSMInstance implementation ---

func (i *AdaptedInstance[TStatus, TDomainConfig]) GetCurrentFSMState() string {
	var obs fsmv2.Observation[TStatus]

	_ = i.sr.LoadObservedTyped(context.Background(), i.workerType, i.childID, &obs)

	return i.mapState(obs, i.domainConfig)
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
	return nil
}

func (i *AdaptedInstance[TStatus, TDomainConfig]) GetLastObservedState() publicfsm.ObservedState {
	var obs fsmv2.Observation[TStatus]

	_ = i.sr.LoadObservedTyped(context.Background(), i.workerType, i.childID, &obs)

	return i.mapObservedState(i.domainConfig, obs)
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
