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
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

type DummyActionPayload struct {
	Message string `json:"message"`
}

// DummyAction represents a dummy action that does nothing, used for testing purposes
type DummyAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	systemSnapshot  *fsm.SystemSnapshot
	payload         *DummyActionPayload
	configManager   config.ConfigManager
}

// Parse implements the Action interface
func (d *DummyAction) Parse(payload interface{}) error {
	// Convert the payload to a map
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		return errors.New("invalid payload format, expected map")
	}

	d.payload = &DummyActionPayload{
		Message: payloadMap["message"].(string),
	}

	return nil
}

// Validate implements the Action interface
func (d *DummyAction) Validate() error {
	// maximial message length is 100 characters
	if len(d.payload.Message) > 100 {
		return errors.New("message is too long, maximum length is 100 characters")
	}
	return nil
}

// Execute implements the Action interface
func (d *DummyAction) Execute() (interface{}, map[string]interface{}, error) {
	// Return a simple success message
	SendActionReply(d.instanceUUID, d.userEmail, d.actionUUID, models.ActionExecuting, "Starting the dummy action", d.outboundChannel, models.DummyAction)

	// get the config
	config, err := d.configManager.GetConfig(context.Background(), 0)
	if err != nil {
		return nil, nil, err
	}

	// update the config
	config.Agent.Location[0] = d.payload.Message

	// write the config
	err = d.configManager.WriteConfig(context.Background(), config)
	if err != nil {
		return nil, nil, err
	}

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
