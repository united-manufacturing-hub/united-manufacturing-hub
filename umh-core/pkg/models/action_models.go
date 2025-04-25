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

type DeployCustomDataFlowComponentPayload struct {
	Name              string                         `json:"name" binding:"required"`
	Payload           DeployDataFlowComponentPayload `json:"payload" binding:"required"`
	Meta              CdcfMeta                       `json:"meta"`
	IgnoreHealthCheck bool                           `json:"ignoreHealthCheck"`
}

type DeployDataFlowComponentPayload struct {
	CDFCPayload CDFCPayload `json:"customDataFlowComponent" binding:"required"`
}

type CDFCPayload struct {
	Inputs          DfcDataConfig            `json:"inputs"`
	Outputs         DfcDataConfig            `json:"outputs"`
	Pipeline        map[string]DfcDataConfig `json:"pipeline"`
	Inject          DfcDataConfig            `json:"inject"`
	IgnoreErrors    bool                     `json:"ignoreErrors"`
	BenthosImageTag string                   `json:"benthosImageTag"`
}

type DfcDataConfig struct {
	Data string `json:"data"`
	Type string `json:"type"`
}

type CdcfMeta struct {
	Type string `json:"type"`
}

// EditInstanceLocation holds the location information for the instance
type EditInstanceLocationModel struct {
	Enterprise string  `json:"enterprise"`
	Site       *string `json:"site,omitempty"`
	Area       *string `json:"area,omitempty"`
	Line       *string `json:"line,omitempty"`
	WorkCell   *string `json:"workCell,omitempty"`
}

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

// this is the structure of the action that the frontend sends in the first place
type CustomDFCPayload struct {
	CustomDataFlowComponent struct {
		Inputs struct {
			Type string `json:"type"`
			Data string `json:"data"`
		} `json:"inputs"`
		Outputs struct {
			Type string `json:"type"`
			Data string `json:"data"`
		} `json:"outputs"`
		Inject struct {
			Type string `json:"type"`
			Data string `json:"data"`
		} `json:"inject"`
		Pipeline struct {
			Processors map[string]struct {
				Type string `json:"type"`
				Data string `json:"data"`
			} `json:"processors"`
		} `json:"pipeline"`
	} `json:"customDataFlowComponent"`
}

// DeleteDFCPayload contains the UUID of the component to delete
type DeleteDFCPayload struct {
	UUID string `json:"uuid"`
}

type EditDataflowcomponentRequestSchemaJson struct {
	BasedOnUuid string `json:"basedOnUuid" yaml:"basedOnUuid" mapstructure:"basedOnUuid"`

	IgnoreHealthCheck *bool `json:"ignoreHealthCheck,omitempty" yaml:"ignoreHealthCheck,omitempty" mapstructure:"ignoreHealthCheck,omitempty"`

	Meta CdcfMeta `json:"meta" yaml:"meta" mapstructure:"meta"`

	Name string `json:"name" yaml:"name" mapstructure:"name"`

	Payload DeployDataFlowComponentPayload `json:"payload" yaml:"payload" mapstructure:"payload"`

	UUID string `json:"uuid" yaml:"uuid" mapstructure:"uuid"`
}

type GetDataflowcomponentResponse map[string]GetDataflowcomponentResponseContent

type GetDataflowcomponentResponseContent struct {
	// CreationTime corresponds to the JSON schema field "creationTime".
	CreationTime float64 `json:"creationTime" yaml:"creationTime" mapstructure:"creationTime"`

	// Creator corresponds to the JSON schema field "creator".
	Creator string `json:"creator" yaml:"creator" mapstructure:"creator"`

	// Meta corresponds to the JSON schema field "meta".
	Meta CommonDataFlowComponentMeta `json:"meta" yaml:"meta" mapstructure:"meta"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name" yaml:"name" mapstructure:"name"`

	// ParentDFC corresponds to the JSON schema field "parentDFC".
	ParentDFC interface{} `json:"parentDFC,omitempty" yaml:"parentDFC,omitempty" mapstructure:"parentDFC,omitempty"`

	// Payload corresponds to the JSON schema field "payload".
	Payload Payload `json:"payload" yaml:"payload" mapstructure:"payload"`
}

type Payload interface{}

type CommonDataFlowComponentMeta struct {
	// Key value pairs of additional metadata
	AdditionalMetadata map[string]interface{} `json:"additionalMetadata,omitempty" yaml:"additionalMetadata,omitempty" mapstructure:"additionalMetadata,omitempty"`

	// The type of the dataflow component
	Type string `json:"type" yaml:"type" mapstructure:"type"`

	AdditionalProperties interface{}
}

type GetDataflowcomponentRequestSchemaJson struct {
	// VersionUUIDs corresponds to the JSON schema field "versionUUIDs".
	VersionUUIDs []string `json:"versionUUIDs" yaml:"versionUUIDs" mapstructure:"versionUUIDs"`
}

type CommonDataFlowComponentCDFCPropertiesPayload struct {
	CDFCProperties CommonDataFlowComponentCDFCProperties `json:"customDataFlowComponent" yaml:"customDataFlowComponent" mapstructure:"customDataFlowComponent"`
}

type CommonDataFlowComponentCDFCProperties struct {
	// BenthosImageTag corresponds to the JSON schema field "benthosImageTag".
	BenthosImageTag *CommonDataFlowComponentBenthosImageTagConfig `json:"benthosImageTag,omitempty" yaml:"benthosImageTag,omitempty" mapstructure:"benthosImageTag,omitempty"`

	// this is only here to make the json schema happy
	IgnoreErrors *bool `json:"ignoreErrors,omitempty" yaml:"ignoreErrors,omitempty" mapstructure:"ignoreErrors,omitempty"`

	// Inputs corresponds to the JSON schema field "inputs".
	Inputs CommonDataFlowComponentInputConfig `json:"inputs" yaml:"inputs" mapstructure:"inputs"`

	// Outputs corresponds to the JSON schema field "outputs".
	Outputs CommonDataFlowComponentOutputConfig `json:"outputs" yaml:"outputs" mapstructure:"outputs"`

	// Pipeline corresponds to the JSON schema field "pipeline".
	Pipeline CommonDataFlowComponentPipelineConfig `json:"pipeline" yaml:"pipeline" mapstructure:"pipeline"`

	// RawYAML corresponds to the JSON schema field "rawYAML".
	RawYAML *CommonDataFlowComponentRawYamlConfig `json:"rawYAML,omitempty" yaml:"rawYAML,omitempty" mapstructure:"rawYAML,omitempty"`
}

type CommonDataFlowComponentBenthosImageTagConfig struct {
	// this is only here to make the json schema happy
	Tag *string `json:"tag,omitempty" yaml:"tag,omitempty" mapstructure:"tag,omitempty"`
}

type CommonDataFlowComponentInputConfig struct {
	// This is the YAML data for the input
	Data string `json:"data" yaml:"data" mapstructure:"data"`

	// This can for example be mqtt
	Type string `json:"type" yaml:"type" mapstructure:"type"`
}

type CommonDataFlowComponentOutputConfig struct {
	// This is the YAML data for the output
	Data string `json:"data" yaml:"data" mapstructure:"data"`

	// This can for example be kafka
	Type string `json:"type" yaml:"type" mapstructure:"type"`
}

type CommonDataFlowComponentPipelineConfig struct {
	// Processors corresponds to the JSON schema field "processors".
	Processors CommonDataFlowComponentPipelineConfigProcessors `json:"processors" yaml:"processors" mapstructure:"processors"`

	// If unset, defaults to -1, see also
	// https://docs.redpanda.com/redpanda-connect/configuration/processing_pipelines/
	Threads *int `json:"threads,omitempty" yaml:"threads,omitempty" mapstructure:"threads,omitempty"`
}

type CommonDataFlowComponentPipelineConfigProcessors map[string]struct {
	// This is the YAML data for the processor
	Data string `json:"data" yaml:"data" mapstructure:"data"`

	// This can for example be bloblang
	Type string `json:"type" yaml:"type" mapstructure:"type"`
}

type CommonDataFlowComponentRawYamlConfig struct {
	// This is the raw yaml data
	Data string `json:"data" yaml:"data" mapstructure:"data"`
}
