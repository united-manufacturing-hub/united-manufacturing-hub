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

package scenarios

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/scenariorunner"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"

	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
)

// Run executes a scenario on an application supervisor that starts with one
// child, the config worker (see scenariorunner.Run). The scenario adds workers
// through env.Client into the dynamicchildren registry, and the application
// worker spawns each one as its own child beside the config worker. Run
// replaces cfg.Root and cfg.ValidWorkerStates.
func Run(ctx context.Context, cfg scenariorunner.RunConfig) (*scenariorunner.RunResult, error) {
	cfg.Root = applicationRoot{}
	cfg.ValidWorkerStates = validWorkerStates

	return scenariorunner.Run(ctx, cfg)
}

// applicationRoot publishes the dynamicchildren registry under the
// process-global configworker deps key, which the application worker reads
// every tick. Clearing the key while the supervisor still runs flips the
// application worker's RegistryConfigured observation.
type applicationRoot struct{}

func (applicationRoot) CheckIdle(scenarioName string) error {
	if register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName) != nil {
		return fmt.Errorf("scenario %q cannot start: the configworker deps key is already published, "+
			"so another run is still active in this process", scenarioName)
	}

	return nil
}

func (applicationRoot) Publish(store storage.TriangularStoreInterface) *fsmv2client.FSMv2Client {
	writer := dynamicchildren.NewWriter()
	register.SetGlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName, writer.Registry())

	return fsmv2client.NewFSMv2Client(writer, store)
}

func (applicationRoot) NewSupervisor(cfg scenariorunner.SupervisorConfig) (scenariorunner.Supervisor, error) {
	sup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:                      cfg.ID,
		Name:                    cfg.Name,
		Store:                   cfg.Store,
		Logger:                  cfg.Logger,
		TickInterval:            cfg.TickInterval,
		Dependencies:            cfg.Dependencies,
		EnableTraceLogging:      cfg.EnableTraceLogging,
		GracefulShutdownTimeout: cfg.GracefulShutdownTimeout,
	})
	if err != nil {
		return nil, err
	}

	return sup, nil
}

func (applicationRoot) Clear() {
	register.ClearGlobalDeps(configworker.WorkerTypeName)
}
