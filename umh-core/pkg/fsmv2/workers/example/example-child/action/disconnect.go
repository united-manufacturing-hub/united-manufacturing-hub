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

package action

import (
	"context"
)

const DisconnectActionName = "disconnect"

// DisconnectAction closes the connection gracefully
// This is a stateless action - completely empty struct with no fields.
// Dependencies will be injected via Execute() in Phase 2C when the Action interface is updated.
type DisconnectAction struct {
	// COMPLETELY EMPTY - no dependencies
}

// NewDisconnectAction creates a new disconnect action
func NewDisconnectAction() *DisconnectAction {
	return &DisconnectAction{}
}

// Execute releases the connection back to the pool
// TEMPORARY LIMITATION: Cannot access dependencies until Phase 2C when Execute signature changes
// to Execute(ctx context.Context, deps Dependencies) error
func (a *DisconnectAction) Execute(ctx context.Context) error {
	// TODO(Phase 2C): Inject dependencies via Execute() parameter
	// For now, this is a skeleton implementation
	return nil
}

func (a *DisconnectAction) String() string {
	return DisconnectActionName
}

func (a *DisconnectAction) Name() string {
	return DisconnectActionName
}
