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

package connection

import (
	"context"

	"github.com/looplab/fsm"
)

// registerCallbacks sets up the callbacks for state machine events
// These callbacks are executed synchronously and should not have any network calls or other operations that could fail
func (c *Connection) registerCallbacks() {
	c.baseFSMInstance.AddCallback("enter_"+OperationalStateTesting, func(ctx context.Context, e *fsm.Event) {
		c.baseFSMInstance.GetLogger().Infof("Connection %s entering state Testing", c.Config.Name)
	})

	c.baseFSMInstance.AddCallback("enter_"+OperationalStateSuccess, func(ctx context.Context, e *fsm.Event) {
		c.baseFSMInstance.GetLogger().Infof("Connection %s entering state Success", c.Config.Name)
	})

	c.baseFSMInstance.AddCallback("enter_"+OperationalStateFailure, func(ctx context.Context, e *fsm.Event) {
		c.baseFSMInstance.GetLogger().Infof("Connection %s entering state Failure", c.Config.Name)
	})

	c.baseFSMInstance.AddCallback("enter_"+OperationalStateStopping, func(ctx context.Context, e *fsm.Event) {
		c.baseFSMInstance.GetLogger().Infof("Connection %s entering state Stopping", c.Config.Name)
	})

	c.baseFSMInstance.AddCallback("enter_"+OperationalStateStopped, func(ctx context.Context, e *fsm.Event) {
		c.baseFSMInstance.GetLogger().Infof("Connection %s entering state Stopped", c.Config.Name)
	})
}
