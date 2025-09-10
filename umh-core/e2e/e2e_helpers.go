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

package e2e_test

import (
	"encoding/base64"
	"fmt"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"gopkg.in/yaml.v3"
)

// createUMHMessage is a helper that creates a UMH message with common structure
func createUMHMessage(content models.UMHMessageContent) (models.UMHMessage, error) {
	encodedContent, err := encoding.EncodeMessageFromUserToUMHInstance(content)
	if err != nil {
		return models.UMHMessage{}, fmt.Errorf("failed to encode message: %w", err)
	}

	return models.UMHMessage{
		Metadata: &models.MessageMetadata{
			TraceID: uuid.New(),
		},
		Email:        "e2e-test@example.com",
		Content:      encodedContent,
		InstanceUUID: uuid.New(),
	}, nil
}

// encodeDataModelStructure marshals a data model structure to base64-encoded YAML
func encodeDataModelStructure(structure map[string]models.Field) (string, error) {
	yamlData, err := yaml.Marshal(structure)
	if err != nil {
		return "", fmt.Errorf("failed to marshal data model structure: %w", err)
	}
	return base64.StdEncoding.EncodeToString(yamlData), nil
}

// createSubscriptionMessage creates a UMH message that subscribes to status updates
func createSubscriptionMessage() (models.UMHMessage, error) {
	subscribePayload := models.SubscribeMessagePayload{
		Resubscribed: false,
	}

	messageContent := models.UMHMessageContent{
		MessageType: models.Subscribe,
		Payload:     subscribePayload,
	}

	return createUMHMessage(messageContent)
}

// createResubscriptionMessage creates a UMH message that resubscribes to status updates
func createResubscriptionMessage() (models.UMHMessage, error) {
	subscribePayload := models.SubscribeMessagePayload{
		Resubscribed: true,
	}

	messageContent := models.UMHMessageContent{
		MessageType: models.Subscribe,
		Payload:     subscribePayload,
	}

	return createUMHMessage(messageContent)
}

// createActionMessage creates a UMH message for a specific action type with payload
func createActionMessage(actionType models.ActionType, payload interface{}) (models.UMHMessage, error) {
	actionPayload := models.ActionMessagePayload{
		ActionType:    actionType,
		ActionUUID:    uuid.New(),
		ActionPayload: payload,
	}

	messageContent := models.UMHMessageContent{
		MessageType: models.Action,
		Payload:     actionPayload,
	}

	return createUMHMessage(messageContent)
}

// createDeployProtocolConverterMessage creates a deploy protocol converter action message
func createDeployProtocolConverterMessage(name, ip string, port uint32, location map[int]string) (models.UMHMessage, error) {
	protocolConverterUUID := uuid.New()
	payload := models.ProtocolConverter{
		UUID:     &protocolConverterUUID,
		Name:     name,
		Location: location,
		Connection: models.ProtocolConverterConnection{
			IP:   ip,
			Port: port,
		},
		// Leave ReadDFC, WriteDFC, and TemplateInfo as nil for initial deployment
	}

	return createActionMessage(models.DeployProtocolConverter, payload)
}

// createEditProtocolConverterMessage creates an edit protocol converter action message
// with benthos generate configuration and UNS output
func createEditProtocolConverterMessage(protocolConverterUUID uuid.UUID, name, ip string, port uint32, location map[int]string) (models.UMHMessage, error) {
	ignoreErrors := false
	readDFC := &models.ProtocolConverterDFC{
		Inputs: models.CommonDataFlowComponentInputConfig{
			Type: "generate",
			Data: `generate:
  auto_replay_nacks: true
  batch_size: 1
  count: 0
  interval: 1s
  mapping: root = "hello world from e2e bridge test"`,
		},
		Pipeline: models.CommonDataFlowComponentPipelineConfig{
			Processors: models.CommonDataFlowComponentPipelineConfigProcessors{
				"0": {
					Type: "tag_processor",
					Data: `tag_processor:
  defaults: |
    msg.meta.location_path = "{{ .location_path }}";
    msg.meta.data_contract = "_raw";
    msg.meta.tag_name = "e2e_bridge_data";
    return msg;`,
				},
			},
		},
		IgnoreErrors: &ignoreErrors,
	}

	payload := models.ProtocolConverter{
		UUID:     &protocolConverterUUID,
		Name:     name,
		Location: location,
		Connection: models.ProtocolConverterConnection{
			IP:   ip,
			Port: port,
		},
		ReadDFC: readDFC,
		// WriteDFC and TemplateInfo remain nil for this test
	}

	return createActionMessage(models.EditProtocolConverter, payload)
}

// createDeleteProtocolConverterMessage creates a delete protocol converter action message
func createDeleteProtocolConverterMessage(protocolConverterUUID uuid.UUID) (models.UMHMessage, error) {
	payload := models.DeleteProtocolConverterPayload{
		UUID: protocolConverterUUID,
	}
	return createActionMessage(models.DeleteProtocolConverter, payload)
}

// createAddDataModelMessage creates an add data model action message
func createAddDataModelMessage(name, description string) (models.UMHMessage, error) {
	structure := map[string]models.Field{
		"temperature": {
			PayloadShape: "timeseries-number",
		},
		"pressure": {
			PayloadShape: "timeseries-number",
		},
		"status": {
			PayloadShape: "timeseries-string",
		},
		"measurements": {
			Subfields: map[string]models.Field{
				"vibration": {
					PayloadShape: "timeseries-number",
				},
				"rpm": {
					PayloadShape: "timeseries-number",
				},
			},
		},
	}

	encodedStructure, err := encodeDataModelStructure(structure)
	if err != nil {
		return models.UMHMessage{}, err
	}

	payload := models.AddDataModelPayload{
		Name:             name,
		Description:      description,
		EncodedStructure: encodedStructure,
	}

	return createActionMessage(models.AddDataModel, payload)
}

// createEditDataModelMessage creates an edit data model action message
func createEditDataModelMessage(name, description string) (models.UMHMessage, error) {
	structure := map[string]models.Field{
		"temperature": {
			PayloadShape: "timeseries-number",
		},
		"pressure": {
			PayloadShape: "timeseries-number",
		},
		"status": {
			PayloadShape: "timeseries-string",
		},
		"measurements": {
			Subfields: map[string]models.Field{
				"vibration": {
					PayloadShape: "timeseries-number",
				},
				"rpm": {
					PayloadShape: "timeseries-number",
				},
				"efficiency": {
					PayloadShape: "timeseries-number",
				},
			},
		},
		"metadata": {
			Subfields: map[string]models.Field{
				"location": {
					PayloadShape: "timeseries-string",
				},
				"operator": {
					PayloadShape: "timeseries-string",
				},
			},
		},
	}

	encodedStructure, err := encodeDataModelStructure(structure)
	if err != nil {
		return models.UMHMessage{}, err
	}

	payload := models.EditDataModelPayload{
		Name:             name,
		Description:      description,
		EncodedStructure: encodedStructure,
	}

	return createActionMessage(models.EditDataModel, payload)
}

// createDeleteDataModelMessage creates a delete data model action message
func createDeleteDataModelMessage(name string) (models.UMHMessage, error) {
	payload := models.DeleteDataModelPayload{
		Name: name,
	}

	return createActionMessage(models.DeleteDataModel, payload)
}
