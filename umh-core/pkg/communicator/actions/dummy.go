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

package actions

import (
	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
)

// DummyAction represents a dummy action that does nothing, used for testing purposes
type DummyAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
}

// Parse implements the Action interface
func (d *DummyAction) Parse(payload interface{}) error {
	// Nothing to parse for dummy action
	return nil
}

// Validate implements the Action interface
func (d *DummyAction) Validate() error {
	// Nothing to validate for dummy action
	return nil
}

// Execute implements the Action interface
func (d *DummyAction) Execute() (interface{}, map[string]interface{}, error) {
	// Return a simple success message
	return "Dummy action executed successfully", nil, nil
}

// getUserEmail implements the Action interface
func (d *DummyAction) getUserEmail() string {
	return d.userEmail
}

// getUuid implements the Action interface
func (d *DummyAction) getUuid() uuid.UUID {
	return d.actionUUID
}
