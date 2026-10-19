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

package simple

import (
	"context"
	"fmt"
	"sync"
)

const probeActionName = "probe"

var registeredProbes sync.Map

type registeredProbe interface {
	run(ctx context.Context, depsAny, cfgAny any) error
}

type typedProbe[TConfig, TDeps any] struct {
	probe      func(ctx context.Context, d TDeps, cfg TConfig)
	workerType string
}

func (p *typedProbe[TConfig, TDeps]) run(ctx context.Context, depsAny, cfgAny any) error {
	d, ok := depsAny.(TDeps)
	if !ok {
		return fmt.Errorf("probe for worker type %q: deps are %T, want %T", p.workerType, depsAny, *new(TDeps))
	}

	cfg, ok := cfgAny.(TConfig)
	if !ok {
		return fmt.Errorf("probe for worker type %q: config is %T, want %T", p.workerType, cfgAny, *new(TConfig))
	}

	p.probe(ctx, d, cfg)

	return nil
}

func registerProbe(workerType string, probe registeredProbe) {
	registeredProbes.Store(workerType, probe)
}

func lookupProbe(workerType string) (registeredProbe, bool) {
	probe, ok := registeredProbes.Load(workerType)
	if !ok {
		return nil, false
	}

	return probe.(registeredProbe), true
}

func probeActionIfRegistered[TConfig any](workerType string, cfg TConfig) any {
	if _, registered := lookupProbe(workerType); !registered {
		// An untyped nil: a typed nil pointer would reach Transition as an action.
		return nil
	}

	return &probeAction[TConfig]{workerType: workerType, cfg: cfg}
}

type probeAction[TConfig any] struct {
	cfg        TConfig
	workerType string
}

func (a *probeAction[TConfig]) Execute(ctx context.Context, depsAny any) error {
	// The executor reports every returned error to Sentry; a stopping worker cancels probes routinely.
	if ctx.Err() != nil {
		return nil
	}

	probe, registered := lookupProbe(a.workerType)
	if !registered {
		return fmt.Errorf("no probe registered for worker type %q", a.workerType)
	}

	return probe.run(ctx, depsAny, a.cfg)
}

func (a *probeAction[TConfig]) Name() string {
	return probeActionName
}
