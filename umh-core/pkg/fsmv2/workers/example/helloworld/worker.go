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

// Package hello_world implements a minimal FSMv2 worker that demonstrates
// the WorkerBase API with typed config/status, action wrapping, and one-line registration.
//
// Naming convention: The package name uses underscore (hello_world) but the
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

// WorkerType is the registered type name for this worker.
const WorkerType = "helloworld"

// HelloworldWorker implements the FSMv2 Worker interface using the WorkerBase API.
type HelloworldWorker struct {
	deps *HelloworldDependencies
	fsmv2.WorkerBase[HelloworldConfig, HelloworldStatus]
}

// NewHelloworldWorker creates a new helloworld worker with the standard framework dependencies.
func NewHelloworldWorker(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	w := &HelloworldWorker{}
	baseDeps := w.InitBase(id, logger, sr)
	w.deps = NewHelloworldDependencies(baseDeps)

	return w, nil
}

// CollectObservedState collects and returns the current observed state.
// Uses ExtractConfig to get typed config from the desired state.
// Returns NewObservation; the collector handles CollectedAt, framework
// metrics, action history, and metric accumulation automatically.
func (w *HelloworldWorker) CollectObservedState(_ context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	cfg := fsmv2.ExtractConfig[HelloworldConfig](desired)

	status := HelloworldStatus{
		HelloSaid: w.deps.HasSaidHello(),
		Mood:      readMoodFile(cfg.MoodFilePath),
	}

	return fsmv2.NewObservation(status), nil
}

// GetDependenciesAny returns the worker's dependencies for action execution.
// Overrides WorkerBase.GetDependenciesAny to return *HelloworldDependencies
// instead of *deps.BaseDependencies.
func (w *HelloworldWorker) GetDependenciesAny() any {
	return w.deps
}

// Actions returns the available actions for this worker.
// Implements ActionProvider capability interface.
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
	register.Worker[HelloworldConfig, HelloworldStatus](WorkerType, NewHelloworldWorker)
}
