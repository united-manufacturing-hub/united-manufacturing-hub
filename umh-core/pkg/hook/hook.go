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

// Package hook lets a release build register its connection workers at
// startup. A build from this repository alone registers nothing.
package hook

import (
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/handoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// Config carries the settings umh-core derives from its own configuration:
// the relay URL and the agent location.
type Config struct {
	RelayURL string
	Location map[int]string
}

// Deps is what umh-core hands to a registered function at startup.
type Deps struct {
	// Inbound carries messages for the message router.
	Inbound chan<- handoff.Message
	// Outbound carries status messages from the subscriber handler.
	Outbound <-chan handoff.Message
	// Replies carries action replies, unchanged.
	Replies <-chan *models.UMHMessage
	// Config holds the relay URL and the agent location.
	Config Config
	// Logger is the logger umh-core hands to the registered function.
	Logger *zap.SugaredLogger
}

var registered []func(Deps) error

// Register adds f. main calls every registered function once, in order,
// before the supervisor starts.
func Register(f func(Deps) error) {
	registered = append(registered, f)
}

// Run calls each registered function in order, and stops at the first error.
func Run(d Deps) error {
	for _, f := range registered {
		if err := f(d); err != nil {
			return err
		}
	}

	return nil
}
