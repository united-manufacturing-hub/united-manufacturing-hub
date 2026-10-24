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

// Package scenariorunner runs an FSM v2 scenario: it builds the scenario's mocks,
// starts a root supervisor, hands the scenario an Env, and checks the run's
// logs and stored states afterwards.
//
// The package imports no worker. The caller supplies the root supervisor
// through RunConfig.Root and the valid states per worker type through
// RunConfig.ValidWorkerStates. Package scenarios holds umh-core's scenario
// list and supplies both.
package scenariorunner

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
)

// RunConfig configures how a scenario is executed.
type RunConfig struct {
	Store        storage.TriangularStoreInterface
	Logger       deps.FSMLogger
	Scenario     Scenario      // Required: Run rejects a Scenario with no Run func or no Name
	Duration     time.Duration // 0 means run forever (until context cancelled)
	TickInterval time.Duration
	// GracefulShutdownTimeout is the per-level drain base propagated to the
	// supervisor subtree. Zero falls back to the supervisor default (5s). A
	// tiny value forces a degraded drain, which is how tests exercise the
	// ShutdownClean=false path deterministically.
	GracefulShutdownTimeout time.Duration
	EnableTraceLogging      bool
	// DumpStore prints the store's changes and final state to stdout after the run.
	DumpStore bool

	// Root builds the supervisor tree the run drives. Required.
	Root Root

	// ValidWorkerStates lists the state names each worker type may report.
	// After the run, a stored state outside its type's list fails the run.
	// A worker type the map does not list is not checked.
	ValidWorkerStates map[string]map[string]bool
}

// SupervisorConfig is what the runner passes to Root.NewSupervisor.
type SupervisorConfig struct {
	Store                   storage.TriangularStoreInterface
	Logger                  deps.FSMLogger
	Dependencies            map[string]any
	ID                      string
	Name                    string
	TickInterval            time.Duration
	GracefulShutdownTimeout time.Duration
	EnableTraceLogging      bool
}

// Supervisor is the part of the root supervisor the runner uses.
type Supervisor interface {
	Start(ctx context.Context) <-chan struct{}
	Shutdown()
	DrainOutcomeClean() bool
}

// Root is the worker tree a run drives, and the registry through which the
// scenario's client adds workers to it. Runs that share a Root must not
// overlap within a process.
type Root interface {
	// CheckIdle returns an error when an earlier run in this process still
	// holds the root.
	CheckIdle(scenarioName string) error

	// Publish makes a new child registry visible to the root's workers and
	// returns the client that writes to it and reads store.
	Publish(store storage.TriangularStoreInterface) *fsmv2client.FSMv2Client

	// NewSupervisor builds the root supervisor.
	NewSupervisor(cfg SupervisorConfig) (Supervisor, error)

	// Clear withdraws what Publish made visible.
	Clear()
}
