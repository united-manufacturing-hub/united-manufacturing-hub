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
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
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

// StatusFunc returns the connection part of a status message.
// The status generator calls it once for each status message.
type StatusFunc func(ctx context.Context, subscriberCount int) *models.Communicator

// Result is what a registered function returns.
type Result struct {
	// Children are the child workers the release build adds under the
	// application.
	Children []config.ChildSpec
	// Status provides the connection part of status messages. It is nil when
	// this function does not provide it.
	Status StatusFunc
}

var registered []func(Deps) (Result, error)

// Register adds f. main calls every registered function once, in order,
// before the supervisor starts.
func Register(f func(Deps) (Result, error)) {
	registered = append(registered, f)
}

// Run calls each registered function in order. Its result holds their
// children in that order, and the one Status that is not nil. It stops at
// the first error, and returns an error when two functions both provide a
// Status.
func Run(d Deps) (Result, error) {
	var result Result
	statusFrom := -1

	for i, f := range registered {
		r, err := f(d)
		if err != nil {
			return Result{}, err
		}

		result.Children = append(result.Children, r.Children...)

		if r.Status != nil {
			if result.Status != nil {
				return Result{}, fmt.Errorf("two registered functions provide a Status: function %d and function %d", statusFrom, i)
			}

			result.Status = r.Status
			statusFrom = i
		}
	}

	return result, nil
}
