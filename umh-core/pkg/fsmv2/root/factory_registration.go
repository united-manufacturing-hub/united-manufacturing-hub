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

package root

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// init registers the passthrough root worker with the factory.
// This enables automatic creation via factory.NewWorkerByType() and factory.NewSupervisorByType().
//
// Registration happens automatically when this package is imported.
// The passthrough root can create any registered worker type as children.
func init() {
	// Register PassthroughWorker factory.
	// This allows creating passthrough root workers via factory.NewWorkerByType().
	if err := factory.RegisterFactory[PassthroughObservedState, *PassthroughDesiredState](
		func(identity fsmv2.Identity) fsmv2.Worker {
			return NewPassthroughWorker(identity.ID, identity.Name)
		}); err != nil {
		panic(fmt.Sprintf("failed to register PassthroughWorker factory: %v", err))
	}

	// Register PassthroughWorker supervisor factory.
	// This allows creating supervisors for passthrough root workers via factory.NewSupervisorByType().
	if err := factory.RegisterSupervisorFactory[PassthroughObservedState, *PassthroughDesiredState](
		func(cfg interface{}) interface{} {
			supervisorCfg := cfg.(supervisor.Config)

			return supervisor.NewSupervisor[PassthroughObservedState, *PassthroughDesiredState](supervisorCfg)
		}); err != nil {
		panic(fmt.Sprintf("failed to register PassthroughWorker supervisor factory: %v", err))
	}
}
