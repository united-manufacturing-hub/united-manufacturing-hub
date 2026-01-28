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

package helpers

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// BaseWorker provides type-safe dependency access for all workers.
// Workers embed this struct to avoid boilerplate and enable direct typed access.
//
// Example:
//
//	type MyWorker struct {
//	    helpers.BaseWorker[MyWorkerDeps]
//	}
//
//	func NewMyWorker(deps MyWorkerDeps) *MyWorker {
//	    return &MyWorker{BaseWorker: helpers.NewBaseWorker(deps)}
//	}
//
// See workers/example/example-child/worker.go for complete usage.
type BaseWorker[D deps.Dependencies] struct {
	dependencies D
}

// NewBaseWorker creates a new BaseWorker with the given dependencies.
func NewBaseWorker[D deps.Dependencies](dependencies D) *BaseWorker[D] {
	return &BaseWorker[D]{dependencies: dependencies}
}

// GetDependencies returns the dependencies for this worker.
func (w *BaseWorker[D]) GetDependencies() D {
	return w.dependencies
}

// GetDependenciesAny returns the dependencies as any for the DependencyProvider interface.
// The supervisor's ActionExecutor to pass dependencies to actions.
func (w *BaseWorker[D]) GetDependenciesAny() any {
	return w.dependencies
}
