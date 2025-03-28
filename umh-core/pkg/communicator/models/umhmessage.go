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

package models

import (
	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"
)

type MessageMetadata struct {
	TraceID uuid.UUID `json:"traceId"`
}

// DatasourceConnectionType specifies the type of data source connection.
type DatasourceConnectionType string

type Protocol string

const (
	// Deprecated: Use a data flow component instead.
	BenthosOPCUA Protocol = "benthos_opcua"
	// Deprecated: Use a data flow component instead.
	BenthosGeneric Protocol = "benthos_generic"
)

// UMHMessage is sent between UMH instances and users.
// UMHInstance contains the UUID of the UMH instance.
// Email identifies the user.
type UMHMessage struct {
	Email        string           `json:"email"`
	Content      string           `json:"content"`
	InstanceUUID uuid.UUID        `json:"umhInstance"`
	Metadata     *MessageMetadata `json:"metadata"`
}

// Define MessageType as a custom type for better type safety
type MessageType string

const (
	Subscribe        MessageType = "subscribe"
	Status           MessageType = "status"
	Action           MessageType = "action"
	ActionReply      MessageType = "action-reply"
	EncryptedContent MessageType = "encrypted-content"
)

// UMHMessageContent holds information about the type of the message
type UMHMessageContent struct {
	Payload     interface{} `json:"Payload"`
	MessageType MessageType `json:"MessageType"`
}

// ActionMessagePayload is the format for sending a message with type "action".
type ActionMessagePayload struct {
	ActionPayload interface{} `json:"actionPayload"`
	ActionType    ActionType  `json:"actionType"`
	ActionUUID    uuid.UUID   `json:"actionUUID"`
}

// ActionType is a custom string type to ensure type safety for specifying different action types.
type ActionType string

const (
	// UnknownAction represents an unknown action type.
	UnknownAction ActionType = "unknown"
	// DummyAction represents a dummy action type for testing purposes, it does nothing.
	DummyAction ActionType = "dummy"
	// TestBenthosInput represents the action type for testing a benthos input.
	TestBenthosInput ActionType = "test-benthos-input"
	// TestNetworkConnection represents the action type for testing a network connection.
	TestNetworkConnection ActionType = "test-network-connection"
	// Deprecated: Use DeployConnection instead.
	DeployOPCUAConnection ActionType = "deploy-opcua-connection"
	// DeployConnection represents the action type for deploying an OPC-UA connection.
	DeployConnection ActionType = "deploy-connection"
	// EditConnection represents the action type for editing a connection.
	EditConnection ActionType = "edit-connection"
	// DeleteConnection represents the action type for deleting a connection.
	DeleteConnection ActionType = "delete-connection"
	// GetConnectionNotes represents the action type for retrieving the notes of a connection.
	GetConnectionNotes ActionType = "get-connection-notes"
	// DeployOPCUADatasource represents the action type for deploying an OPC-UA datasource.
	DeployOPCUADatasource ActionType = "deploy-opcua-datasource"
	// GetDatasourceBasic represents an action type for retrieving data source information by its UUID
	// It does not contain sensitive information, like authentication
	GetDatasourceBasic ActionType = "get-datasource-basic"
	// EditOPCUADatasource represents an action type for editing an OPC-UA datasource.
	EditOPCUADatasource ActionType = "edit-opcua-datasource"
	// DeleteDatasource represents the action type for deleting a data source.
	DeleteDatasource ActionType = "delete-datasource"
	// UpgradeCompanion represents the action type for upgrading to a specific version
	UpgradeCompanion ActionType = "upgrade-companion"
	// EditMqttBroker sets the MQTT broker details
	EditMqttBroker ActionType = "edit-mqtt-broker"
	// DeployProtocolConverter represents the action type for deploying a protocol converter
	DeployProtocolConverter ActionType = "deploy-protocol-converter"
	// DeleteProtocolConverter represents the action type for deleting a protocol converter
	DeleteProtocolConverter ActionType = "delete-protocol-converter"
	// EditProtocolConverter represents the action type for editing a protocol converter
	EditProtocolConverter ActionType = "edit-protocol-converter"
	// GetProtocolConverter represents the action type for getting a protocol converter
	GetProtocolConverter ActionType = "get-protocol-converter"
	// GetAuditLog represents the action type for getting the audit log
	GetAuditLog ActionType = "get-audit-log"
	// EditInstanceLocation represents the action type for setting the location of the UMH instance
	EditInstanceLocation ActionType = "edit-instance-location"
	// EditInstance represents the action type for editing an UMH instance
	EditInstance ActionType = "edit-instance"
	// DeployDataFlowComponent represents the action type for deploying a data flow component
	DeployDataFlowComponent ActionType = "deploy-data-flow-component"
	// EditDataFlowComponent represents the action type for editing a data flow component
	EditDataFlowComponent ActionType = "edit-data-flow-component"
	// GetDataFlowComponent represents the action type for retrieving one or multiple data flow components
	GetDataFlowComponent ActionType = "get-data-flow-component"
	// DeleteDataFlowComponent represents the action type for deleting a data flow component
	DeleteDataFlowComponent ActionType = "delete-data-flow-component"
	// GetDataFlowComponentMetrics represents the action type for retrieving metrics of a data flow component
	GetDataFlowComponentMetrics ActionType = "get-data-flow-component-metrics"
	// GetDataFlowComponentLog reperesents the action type for getting the audit log for a data flow component
	GetDataFlowComponentLog ActionType = "get-data-flow-component-log"
	// GetKubernetesEvents represents the action type for retrieving Kubernetes events
	GetKubernetesEvents ActionType = "get-kubernetes-events"
	// GetOPCUATags represents the action type to retrieve all opcua tags
	GetOPCUATags ActionType = "get-opcua-tags"
	// RollbackDataFlowComponent represents the action type for rolling back a data flow component
	RollbackDataFlowComponent ActionType = "rollback-data-flow-component"
	// AllowAppSecretAccess is a META action type for allowing the app to access the secret, this is not a real action but used in the subscription flow
	AllowAppSecretAccess ActionType = "allow-app-secret-access"
	// GetConfiguration represents the action type for retrieving a configuration
	GetConfiguration ActionType = "get-configuration"
	// UpdateConfiguration represents the action type for updating a configuration
	UpdateConfiguration ActionType = "update-configuration"
)

// TestNetworkConnectionPayload contains the necessary fields for executing a TestNetworkConnection action.
type TestNetworkConnectionPayload struct {
	IP   string                   `json:"ip" binding:"required"`         // IP address of the target
	Type DatasourceConnectionType `json:"datasource" binding:"required"` // Type of data source connection (e.g., OPC-UA)
	Port uint32                   `json:"port" binding:"required"`       // Port to connect on
}

const (
	// OpcuaServer represents an OPC-UA server as the data source connection type.
	OpcuaServer DatasourceConnectionType = "opcua-server"
	// GenericAsset represents a generic asset as the data source connection type (not tied to a specific protocol).
	GenericAsset DatasourceConnectionType = "generic-asset"
	// ExternalMQTT represents an external MQTT broker as the data source connection type.
	ExternalMQTT DatasourceConnectionType = "external-mqtt"
)

// EditMqttBrokerPayload contains the necessary fields for setting the MQTT broker details.
type EditMqttBrokerPayload struct {
	IP          string    `json:"ip" binding:"required"`
	Port        uint32    `json:"port" binding:"required"`
	Username    string    `json:"username" binding:"required"`
	Password    string    `json:"password" binding:"required"`
	LastUpdated uint64    `json:"lastUpdated"`
	UUID        uuid.UUID `json:"uuid" binding:"required"`
}

// EditInstanceLocationAction contains the necessary fields for setting the location of the UMH instance.
type EditInstanceLocationAction struct {
	Enterprise string  `json:"enterprise" binding:"required"`
	Site       *string `json:"site"`
	Area       *string `json:"area"`
	Line       *string `json:"line"`
	WorkCell   *string `json:"workCell"`
}

// GetProtocolConverterPayload contains the necessary fields for getting a protocol converter.
type GetProtocolConverterPayload struct {
	UUID uuid.UUID `json:"uuid" binding:"required"`
}

// DeployConnectionPayload contains the necessary fields for executing a DeployConnection action.
type DeployConnectionPayload struct {
	Name     string                   `json:"name" binding:"required"`
	IP       string                   `json:"ip" binding:"required"`
	Type     DatasourceConnectionType `json:"datasource" binding:"required"`
	Notes    string                   `json:"notes"`
	Port     uint32                   `json:"port" binding:"required"`
	Uuid     uuid.UUID                `json:"uuid" binding:"required"`
	Location *ConnectionLocation      `json:"location"`
}

type GetConfigurationPayload struct {
	ConfigType  string   `json:"configType"`  // For backward compatibility
	ConfigTypes []string `json:"configTypes"` // New field for multiple config types
}

type UpdateConfigurationPayload struct {
	ConfigType string                 `json:"configType" binding:"required"`
	Config     map[string]interface{} `json:"config" binding:"required"`
}

type ConnectionLocation struct {
	Site     *string `json:"site"`
	Area     *string `json:"area"`
	Line     *string `json:"line"`
	WorkCell *string `json:"workCell"`
}

// EditConnectionPayload contains the necessary fields for executing an EditConnection action.
// It allows to partially edit different fields of a connection.
type EditConnectionPayload struct {
	Name     *string                   `json:"name,omitempty"`
	IP       *string                   `json:"ip,omitempty"`
	Type     *DatasourceConnectionType `json:"datasource,omitempty"`
	Notes    *string                   `json:"notes,omitempty"`
	Port     *uint32                   `json:"port,omitempty"`
	Uuid     uuid.UUID                 `json:"uuid" binding:"required"`
	Location *ConnectionLocation       `json:"location,omitempty"`
}

// DeleteConnectionPayload only contains the UUID to identify the connection to delete.
//
// TODO: Should we maybe have a single struct with a required UUID field and reuse it? (e.g. IdentifierPayload)
type DeleteConnectionPayload struct {
	Uuid uuid.UUID `json:"uuid" binding:"required"`
}

// GetConnectionNotesPayload contains the necessary fields for executing a GetConnectionNotes action.
type GetConnectionNotesPayload struct {
	Uuid uuid.UUID `json:"uuid" binding:"required"`
}

// GetConnectionNotesReplyPayload contains the notes of a connection to be sent as a reply to a GetConnectionNotes action.
type GetConnectionNotesReplyPayload struct {
	Notes string `json:"notes"`
}

type GetDatasourcePayload struct {
	Uuid uuid.UUID `json:"uuid" binding:"required"`
}

type DatasourceAuthentication struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	CertificatePub  string `json:"certificatePub"`
	CertificatePriv string `json:"certificatePriv"`
}

// DeleteDatasourcePayload only contains the UUID to identify the data source to delete.
type DeleteDatasourcePayload struct {
	Uuid uuid.UUID `json:"uuid" binding:"required"`
}

// UpgradeCompanionPayload contains the versio the user wants to upgrade to.
type UpgradeCompanionPayload struct {
	Version semver.Version `json:"version" binding:"required"`
}

type DeployProtocolConverterPayload struct {
	Type           Protocol          `json:"type" binding:"required"`
	Name           string            `json:"name" binding:"required"`
	ConnectionUUID uuid.UUID         `json:"connectionUUID" binding:"required"`
	UUID           uuid.UUID         `json:"uuid" binding:"required"`
	IgnoreErrors   bool              `json:"ignoreErrors,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`

	// Generic only
	InputYaml     *string `json:"inputYaml,omitempty"`
	ProcessorYaml *string `json:"processorYaml,omitempty"`
	InjectYaml    *string `json:"injectYaml,omitempty"`
}

type DeleteProtocolConverterPayload struct {
	UUID uuid.UUID `json:"uuid" binding:"required"`
}

type EditProtocolConverterPayload struct {
	UUID uuid.UUID `json:"uuid" binding:"required"`

	Name         *string           `json:"name,omitempty"`
	IgnoreErrors bool              `json:"ignoreErrors,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`

	// Generic only
	InputYaml     *string `json:"inputYaml,omitempty"`
	ProcessorYaml *string `json:"processorYaml,omitempty"`
	InjectYaml    *string `json:"injectYaml,omitempty"`
}

// ActionReplyState specifies the current state of an action.
type ActionReplyState string

const (
	// ActionConfirmed indicates the action is confirmed and about to start.
	ActionConfirmed ActionReplyState = "action-confirmed"
	// ActionExecuting indicates the action is currently being executed.
	ActionExecuting ActionReplyState = "action-executing"
	// ActionFinishedSuccessfull indicates the action finished successfully.
	ActionFinishedSuccessfull ActionReplyState = "action-success"
	// ActionFinishedWithFailure indicates the action failed.
	ActionFinishedWithFailure ActionReplyState = "action-failure"
)

// ActionReplyMessagePayload contains the fields required for an action reply message.
type ActionReplyMessagePayload struct {
	ActionReplyState   ActionReplyState `json:"actionReplyState" binding:"required"`
	ActionReplyPayload interface{}      `json:"actionReplyPayload" binding:"required"`
	ActionUUID         uuid.UUID        `json:"actionUUID" binding:"required"`
	// ActionContext is an optional field that can be used to provide additional context for the action.
	ActionContext map[string]interface{} `json:"actionContext,omitempty"`
}
