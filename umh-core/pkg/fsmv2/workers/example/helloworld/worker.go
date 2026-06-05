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

// Package hello_world provides a minimal FSMv2 worker example.
//
// This is the simplest possible worker implementation demonstrating:
//   - The 3 required Worker interface methods
//   - Factory registration pattern
//   - State machine transitions
//   - Action execution
//
// Use this as a template when creating new workers. See README.md for details.
//
// NAMING CONVENTION: The package name uses underscore (hello_world) but the
// folder name is "helloworld". The type prefix must be "Helloworld" (one capital)
// to derive correctly as worker type "helloworld".
package hello_world

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// HelloworldWorker implements the FSMv2 Worker interface.
//
// Workers are the core building blocks of FSMv2. Each worker:
//   - Collects its current observed state (CollectObservedState)
//   - Derives what state it should be in (DeriveDesiredState)
//   - Provides an initial state (GetInitialState)
//
// The supervisor drives the state machine by calling these methods each tick.
type HelloworldWorker struct {
	fsmv2.WorkerBase[HelloworldConfig, HelloworldStatus, *HelloworldDependencies]
}

// NewHelloworldWorker creates a new helloworld worker.
func NewHelloworldWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*HelloworldWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	w := &HelloworldWorker{}
	bd := w.InitBase(identity, logger, stateReader)
	workerDeps := NewHelloworldDependencies(bd)
	w.BindDeps(workerDeps)

	return w, nil
}

// GetDependencies returns the typed HelloworldDependencies.
// Panics with a clear message if BindDeps was not called before this worker is used.
func (w *HelloworldWorker) GetDependencies() *HelloworldDependencies {
	raw := w.GetDependenciesAny()

	d, ok := raw.(*HelloworldDependencies)
	if !ok || d == nil {
		panic("HelloworldWorker: GetDependencies called before BindDeps")
	}

	return d
}

// CollectObservedState returns the current observed state.
//
// Uses fsmv2.NewObservation which signals the supervisor's collector to perform
// post-COS wrapping (CollectedAt, framework metrics, action history).
func (w *HelloworldWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	cfg := fsmv2.ExtractConfig[HelloworldConfig](desired)

	status := HelloworldStatus{
		HelloSaid: w.GetDependencies().HasSaidHello(),
		Mood:      readMoodFile(cfg.MoodFilePath),
	}

	return fsmv2.NewObservation(status), nil
}

// Actions returns the available actions for this worker.
// Implements fsmv2.ActionProvider.
func (w *HelloworldWorker) Actions() map[string]fsmv2.Action[any] {
	return map[string]fsmv2.Action[any]{
		SayHelloActionName: fsmv2.SimpleAction[*HelloworldDependencies](SayHelloActionName, SayHello),
	}
}

// readMoodFile reads the mood from a file path. Returns empty string on error or empty path.
func readMoodFile(path string) string {
	if path == "" {
		return ""
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

func init() {
	register.Worker[HelloworldConfig, HelloworldStatus, *HelloworldDependencies]("helloworld",
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			return NewHelloworldWorker(id, logger, sr)
		})
}

// ensure HelloworldWorker implements Worker interfaces (compile-time check).
var _ fsmv2.Worker = (*HelloworldWorker)(nil)
