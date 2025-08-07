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

// createSubscriptionMessage creates a UMH message that subscribes to status updates
func createSubscriptionMessage() models.UMHMessage {
	// Create the subscription payload
	subscribePayload := models.SubscribeMessagePayload{
		Resubscribed: false, // This is a new subscription, not a resubscription
	}

	// Create the message content
	messageContent := models.UMHMessageContent{
		MessageType: models.Subscribe,
		Payload:     subscribePayload,
	}

	// Encode the content using the encoding package
	encodedContent, err := encoding.EncodeMessageFromUserToUMHInstance(messageContent)
	if err != nil {
		panic(fmt.Sprintf("Failed to encode subscription message: %v", err))
	}

	// Create the UMH message
	return models.UMHMessage{
		Metadata: &models.MessageMetadata{
			TraceID: uuid.New(),
		},
		Email:        "e2e-test@example.com",
		Content:      encodedContent,
		InstanceUUID: uuid.New(),
	}
}

// createResubscriptionMessage creates a UMH message that resubscribes to status updates
func createResubscriptionMessage() models.UMHMessage {
	// Create the subscription payload with resubscribed flag
	subscribePayload := models.SubscribeMessagePayload{
		Resubscribed: true, // This is a resubscription to refresh TTL
	}

	// Create the message content
	messageContent := models.UMHMessageContent{
		MessageType: models.Subscribe,
		Payload:     subscribePayload,
	}

	// Encode the content using the encoding package
	encodedContent, err := encoding.EncodeMessageFromUserToUMHInstance(messageContent)
	if err != nil {
		panic(fmt.Sprintf("Failed to encode resubscription message: %v", err))
	}

	// Create the UMH message
	return models.UMHMessage{
		Metadata: &models.MessageMetadata{
			TraceID: uuid.New(),
		},
		Email:        "e2e-test@example.com",
		Content:      encodedContent,
		InstanceUUID: uuid.New(),
	}
}

// createActionMessage creates a UMH message for a specific action type with payload
func createActionMessage(actionType models.ActionType, payload interface{}) models.UMHMessage {
	// Create the action payload
	actionPayload := models.ActionMessagePayload{
		ActionType:    actionType,
		ActionUUID:    uuid.New(),
		ActionPayload: payload,
	}

	// Create the message content
	messageContent := models.UMHMessageContent{
		MessageType: models.Action,
		Payload:     actionPayload,
	}

	// Encode the content using the encoding package
	encodedContent, err := encoding.EncodeMessageFromUserToUMHInstance(messageContent)
	if err != nil {
		panic(fmt.Sprintf("Failed to encode action message: %v", err))
	}

	// Create the UMH message
	return models.UMHMessage{
		Metadata: &models.MessageMetadata{
			TraceID: uuid.New(),
		},
		Email:        "e2e-test@example.com",
		Content:      encodedContent,
		InstanceUUID: uuid.New(),
	}
}

// createDeployProtocolConverterMessage creates a deploy protocol converter action message
func createDeployProtocolConverterMessage(name, ip string, port uint32, location map[int]string) models.UMHMessage {
	payload := models.ProtocolConverter{
		UUID:     &[]uuid.UUID{uuid.New()}[0], // Generate a new UUID
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
func createEditProtocolConverterMessage(protocolConverterUUID uuid.UUID, name, ip string, port uint32, location map[int]string) models.UMHMessage {
	// Create the benthos generate configuration for a read DFC
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
		IgnoreErrors: &[]bool{false}[0], // Don't ignore errors for testing
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
func createDeleteProtocolConverterMessage(protocolConverterUUID uuid.UUID) models.UMHMessage {
	payload := models.DeleteProtocolConverterPayload{
		UUID: protocolConverterUUID,
	}
	return createActionMessage(models.DeleteProtocolConverter, payload)
}

// createAddDataModelMessage creates an add data model action message
func createAddDataModelMessage(name, description string) models.UMHMessage {
	// Create a simple data model structure
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

	// Marshal the structure to YAML
	yamlData, err := yaml.Marshal(structure)
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal data model structure: %v", err))
	}

	// Base64 encode the YAML
	encodedStructure := base64.StdEncoding.EncodeToString(yamlData)

	payload := models.AddDataModelPayload{
		Name:             name,
		Description:      description,
		EncodedStructure: encodedStructure,
	}

	return createActionMessage(models.AddDataModel, payload)
}

// createEditDataModelMessage creates an edit data model action message
func createEditDataModelMessage(name, description string) models.UMHMessage {
	// Create an updated data model structure with additional fields
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
				"efficiency": { // New field added in edit
					PayloadShape: "timeseries-number",
				},
			},
		},
		"metadata": { // New section added in edit
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

	// Marshal the structure to YAML
	yamlData, err := yaml.Marshal(structure)
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal data model structure: %v", err))
	}

	// Base64 encode the YAML
	encodedStructure := base64.StdEncoding.EncodeToString(yamlData)

	payload := models.EditDataModelPayload{
		Name:             name,
		Description:      description,
		EncodedStructure: encodedStructure,
	}

	return createActionMessage(models.EditDataModel, payload)
}

// createDeleteDataModelMessage creates a delete data model action message
func createDeleteDataModelMessage(name string) models.UMHMessage {
	payload := models.DeleteDataModelPayload{
		Name: name,
	}

	return createActionMessage(models.DeleteDataModel, payload)
}
