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

package hello_world

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// HelloworldConfig holds the user-provided configuration for the helloworld worker.
// Embeds BaseUserSpec to support the StateGetter interface, allowing WorkerBase.DeriveDesiredState
// to extract the desired state from the "state" YAML field.
type HelloworldConfig struct {
	config.BaseUserSpec `yaml:",inline"`

	// MoodFilePath is the path to the mood file whose contents set the worker's mood.
	// When empty, mood checking is skipped.
	MoodFilePath string `yaml:"moodFilePath" json:"moodFilePath"`
}

// HelloworldStatus holds the runtime observation data for the helloworld worker.
type HelloworldStatus struct {
	// HelloSaid tracks whether the SayHelloAction has been executed.
	HelloSaid bool `json:"helloSaid"`
	// Mood is read from the mood file at HelloworldConfig.MoodFilePath by CollectObservedState.
	Mood string `json:"mood,omitempty"`
}
