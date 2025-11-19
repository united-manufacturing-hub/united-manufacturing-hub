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

package root_supervisor

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// init registers the example child worker type with the factory.
// This enables automatic creation via factory.NewWorkerByType() and factory.NewSupervisorByType().
//
// Registration happens automatically when this package is imported.
// The factories are used by reconcileChildren() in the root supervisor to create child supervisors.
//
// Note: Root worker registration is handled by the generic root package.
// This example only needs to register its custom child worker type.
func init() {
	// Register ChildWorker factory
	// This allows creating child workers via factory.NewWorkerByType()
	if err := factory.RegisterFactory[ChildObservedState, *ChildDesiredState](
		func(identity fsmv2.Identity) fsmv2.Worker {
			return NewChildWorker(identity.ID, identity.Name, nil)
		}); err != nil {
		panic(fmt.Sprintf("failed to register ChildWorker factory: %v", err))
	}

	// Register ChildWorker supervisor factory
	// This allows creating supervisors for child workers via factory.NewSupervisorByType()
	// Used by reconcileChildren() when root worker's ChildrenSpecs include child workers
	if err := factory.RegisterSupervisorFactory[ChildObservedState, *ChildDesiredState](
		func(cfg interface{}) interface{} {
			supervisorCfg := cfg.(supervisor.Config)

			return supervisor.NewSupervisor[ChildObservedState, *ChildDesiredState](supervisorCfg)
		}); err != nil {
		panic(fmt.Sprintf("failed to register ChildWorker supervisor factory: %v", err))
	}
}
