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

package examples

import (
	"context"
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const mockMoodPath = "mood"

// HelloworldScenarioV2 runs one helloworld child against a mock filesystem
// held in the dependency map. It creates the child with a happy mood file,
// rewrites the file to grumpy, and deletes it. After each change it waits for
// the child's observation to show the new mood.
//
// The child starts and then stays Running through all three moods, because
// only the mood "sad" moves it to Degraded. So this scenario waits for the
// mood in the observation.
var HelloworldScenarioV2 = ScenarioV2{
	Name:        "helloworld",
	Description: "A helloworld child reads its mood from a file. The scenario changes the file twice and checks that the observed mood follows each time",

	Dependencies: func() (map[string]any, func(), error) {
		moodFS := &mockFilesystem{files: map[string][]byte{}}
		moodFS.SetFile(mockMoodPath, "happy")

		deps := map[string]any{}

		var fsService filesystem.Service = moodFS

		config.SetDependency(deps, hello_world.FilesystemKey, fsService)

		return deps, nil, nil
	},

	Run: func(ctx context.Context, env Env) error {
		// Assert the Service back to *mockFilesystem, so Run can change the
		// file the worker reads.
		service, ok := config.LookupDependency(env.Dependencies, hello_world.FilesystemKey)
		if !ok {
			return errors.New("the helloworld scenario's dependency map holds no filesystem under hello_world.FilesystemKey")
		}

		moodFS, ok := service.(*mockFilesystem)
		if !ok {
			return errors.New("the filesystem under hello_world.FilesystemKey is not the scenario's mockFilesystem")
		}

		ref := dynamicchildren.Ref{WorkerType: "helloworld", Name: "hello-1"}

		env.Step("create the helloworld child with a mood file that says happy")

		if err := env.Client.Upsert(ref, map[string]any{
			"state":        "running",
			"moodFilePath": mockMoodPath,
		}); err != nil {
			return err
		}

		waitForMood := func(want string) error {
			return env.WaitFor(ctx, "the child's observation shows mood="+want,
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[hello_world.HelloworldStatus](ctx, env.Client, ref)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the child has not published an observation yet", nil
						}

						return false, "", err
					}

					return obs.Status.Mood == want, "mood=" + obs.Status.Mood, nil
				})
		}

		if err := waitForMood("happy"); err != nil {
			return err
		}

		env.Step("change the mood file to grumpy; wait for mood=grumpy in the observation")
		moodFS.SetFile(mockMoodPath, "grumpy")

		if err := waitForMood("grumpy"); err != nil {
			return err
		}

		env.Step("delete the mood file; wait for an empty mood in the observation")
		moodFS.RemoveFile(mockMoodPath)

		return waitForMood("")
	},
}
