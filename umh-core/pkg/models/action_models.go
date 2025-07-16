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
	Inject          string                   `json:"inject"`
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

// SubscribeMessagePayload is the format for sending a message with type "subscribe".
type SubscribeMessagePayload struct {
	Resubscribed bool `json:"resubscribed,omitempty"`
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
	//
	// Deprecated: Use GetMetrics instead. Kept for backward compatibility.
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
	// GetLogs represents the action type for retrieving logs
	GetLogs ActionType = "get-logs"
	// GetMetrics represents the action type for retrieving metrics
	GetMetrics ActionType = "get-metrics"
	// GetConfigFile represents the action type for retrieving the configuration file
	GetConfigFile ActionType = "get-config-file"
	// SetConfigFile represents the action type for updating the configuration file
	SetConfigFile ActionType = "set-config-file"
	// AddDataModel represents the action type for adding a data model
	AddDataModel ActionType = "add-datamodel"
	// DeleteDataModel represents the action type for deleting a data model
	DeleteDataModel ActionType = "delete-datamodel"
	// EditDataModel represents the action type for editing a data model
	EditDataModel ActionType = "edit-datamodel"
	// GetDataModel represents the action type for retrieving a data model
	GetDataModel ActionType = "get-datamodel"
	// DeployStreamProcessor represents the action type for deploying a stream processor
	DeployStreamProcessor ActionType = "deploy-stream-processor"
	// EditStreamProcessor represents the action type for editing a stream processor
	EditStreamProcessor ActionType = "edit-stream-processor"
	// DeleteStreamProcessor represents the action type for deleting a stream processor
	DeleteStreamProcessor ActionType = "delete-stream-processor"
	// GetStreamProcessor represents the action type for getting a stream processor
	GetStreamProcessor ActionType = "get-stream-processor"
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
			Data string `json:"data"`
		} `json:"rawYAML"`
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

	// State corresponds to the JSON schema field "state".
	State string `json:"state" yaml:"state" mapstructure:"state"`
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

type LogType string

const (
	AgentLogType                  LogType = "agent"
	DFCLogType                    LogType = "dfc"
	ProtocolConverterReadLogType  LogType = "protocol-converter-read"
	ProtocolConverterWriteLogType LogType = "protocol-converter-write"
	RedpandaLogType               LogType = "redpanda"
	TopicBrowserLogType           LogType = "topic-browser"
	StreamProcessorLogType        LogType = "stream-processor"
)

// GetLogsRequest contains the necessary fields for executing a `get-logs` action.
type GetLogsRequest struct {
	// StartTime represents the time frame to start the logs from in unix milliseconds
	StartTime int64 `json:"startTime"`
	// Type represents the type of the logs to retrieve
	Type LogType `json:"type"`
	// UUID represents the identifier of the entity to retrieve the logs for.
	// This is optional and only used for DFC and Protocol Converter logs.
	UUID string `json:"uuid"`
}

type GetLogsResponse struct {
	Logs []string `json:"logs"`
}

type MetricResourceType string

const (
	DFCMetricResourceType          MetricResourceType = "dfc"
	RedpandaMetricResourceType     MetricResourceType = "redpanda"
	TopicBrowserMetricResourceType MetricResourceType = "topic-browser"
)

// GetMetricsRequest contains the necessary fields for executing a `get-metrics` action.
type GetMetricsRequest struct {
	// Type represents the type of the resource to retrieve the metrics for
	Type MetricResourceType `json:"type" binding:"required"`
	// UUID represents the identifier of the entity to retrieve the metrics for.
	// This is optional and only used for DFC metrics.
	UUID string `json:"uuid"`
}

type MetricValueType string

const (
	MetricValueTypeNumber  MetricValueType = "number"
	MetricValueTypeString  MetricValueType = "string"
	MetricValueTypeBoolean MetricValueType = "boolean"
)

// Metric represents a single metric value, agnostic of the resource type.
type Metric struct {
	ValueType     MetricValueType `json:"value_type"`
	Value         any             `json:"value"`
	ComponentType string          `json:"component_type"`
	Path          string          `json:"path"`
	Name          string          `json:"name"`
}

// GetMetricsResponse contains the metrics yielded by the `get-metrics` action.
type GetMetricsResponse struct {
	Metrics []Metric `json:"metrics"`
}

// GetConfigFileResponse contains the config file content
type GetConfigFileResponse struct {
	Content          string `json:"content"`
	LastModifiedTime string `json:"lastModifiedTime"`
}

type SetConfigFilePayload struct {
	Content          string `json:"content"`
	LastModifiedTime string `json:"lastModifiedTime"`
}

type SetConfigFileResponse struct {
	Content          string `json:"content"`
	LastModifiedTime string `json:"lastModifiedTime"`
	Success          bool   `json:"success"`
}

type DataModelVersion struct {
	Structure map[string]Field `yaml:"structure"` // structure of the data model (fields)
}

// ModelRef represents a reference to another data model
type ModelRef struct {
	Name    string `yaml:"name"`    // name of the referenced data model
	Version string `yaml:"version"` // version of the referenced data model
}

type Field struct {
	PayloadShape string           `yaml:"_payloadshape,omitempty"` // payload shape of the field
	ModelRef     *ModelRef        `yaml:"_refModel,omitempty"`     // this is a special field that is used to reference another data model to be used as a type for this field
	Subfields    map[string]Field `yaml:",inline"`                 // subfields of the field (allow recursive definition of fields)
}

// AddDataModelPayload contains the necessary fields for executing an AddDataModel action.
type AddDataModelPayload struct {
	Name             string           `json:"name" binding:"required"`             // Name of the data model
	Description      string           `json:"description,omitempty"`               // Description of the data model
	EncodedStructure string           `json:"encodedStructure" binding:"required"` // Encoded structure of the data model
	Structure        map[string]Field `json:"-"`                                   // Data model version (not used in the action, but filled by the action)
}

// DeleteDataModelPayload contains the necessary fields for executing a DeleteDataModel action.
type DeleteDataModelPayload struct {
	Name string `json:"name" binding:"required"` // Name of the data model to delete
}

// EditDataModelPayload contains the necessary fields for executing an EditDataModel action.
type EditDataModelPayload struct {
	Name             string           `json:"name" binding:"required"`             // Name of the data model to edit
	Description      string           `json:"description,omitempty"`               // Description of the data model
	EncodedStructure string           `json:"encodedStructure" binding:"required"` // Encoded structure of the data model
	Structure        map[string]Field `json:"-"`                                   // Data model version (not used in the action, but filled by the action)
}

// GetDataModelPayload contains the necessary fields for executing a GetDataModel action.
type GetDataModelPayload struct {
	Name            string `json:"name" binding:"required"`   // Name of the data model to retrieve
	GetEnrichedTree bool   `json:"getEnrichedTree,omitempty"` // Whether to fill refModel fields with actual model data
}

// GetDataModelVersion represents a version of a data model with base64-encoded structure
type GetDataModelVersion struct {
	Description      string           `json:"description,omitempty"` // Description of the data model version
	EncodedStructure string           `json:"encodedStructure"`      // Base64-encoded structure of the data model version
	Structure        map[string]Field `json:"-"`                     // Data model version structure (not sent in response, but used internally)
}

// GetDataModelResponse contains the response for a GetDataModel action.
type GetDataModelResponse struct {
	Name        string                         `json:"name"`                  // Name of the data model
	Description string                         `json:"description,omitempty"` // Description of the data model
	Versions    map[string]GetDataModelVersion `json:"versions"`              // All versions of the data model
}

// Deprecated: Use GetMetricsRequest instead.
type GetDataflowcomponentMetricsRequest struct {
	UUID string `json:"uuid" binding:"required"`
}

type ActionReplyResponseSchemaJson struct {
	// Additional contextual data for the action, allows arbitrary key-value pairs.
	ActionContext ActionReplyResponseSchemaJsonActionContext `json:"actionContext,omitempty" yaml:"actionContext,omitempty" mapstructure:"actionContext,omitempty"`

	// Legacy action reply payload, can be a string or an object for backward
	// compatibility.
	ActionReplyPayload interface{} `json:"actionReplyPayload" yaml:"actionReplyPayload" mapstructure:"actionReplyPayload"`

	// Structured response payload for any action reply.
	ActionReplyPayloadV2 *ActionReplyResponseSchemaJsonActionReplyPayloadV2 `json:"actionReplyPayloadV2,omitempty" yaml:"actionReplyPayloadV2,omitempty" mapstructure:"actionReplyPayloadV2,omitempty"`

	// State of the action reply.
	ActionReplyState ActionReplyResponseSchemaJsonActionReplyState `json:"actionReplyState" yaml:"actionReplyState" mapstructure:"actionReplyState"`

	// Unique identifier for the action.
	ActionUUID string `json:"actionUUID" yaml:"actionUUID" mapstructure:"actionUUID"`
}

// Additional contextual data for the action, allows arbitrary key-value pairs.
type ActionReplyResponseSchemaJsonActionContext map[string]interface{}

type ActionReplyResponseSchemaJsonActionReplyPayloadV2 struct {
	// Machine-readable error code (e.g., 'ERR_CONFIG_CHANGED').
	ErrorCode *string `json:"errorCode,omitempty" yaml:"errorCode,omitempty" mapstructure:"errorCode,omitempty"`

	// Human-readable error message.
	Message string `json:"message" yaml:"message" mapstructure:"message"`

	// Additional payload for the action reply.
	Payload ActionReplyResponseSchemaJsonActionReplyPayloadV2Payload `json:"payload,omitempty" yaml:"payload,omitempty" mapstructure:"payload,omitempty"`
}

type ActionReplyResponseSchemaJsonActionReplyState string

const ActionReplyResponseSchemaJsonActionReplyStateActionConfirmed ActionReplyResponseSchemaJsonActionReplyState = "action-confirmed"
const ActionReplyResponseSchemaJsonActionReplyStateActionExecuting ActionReplyResponseSchemaJsonActionReplyState = "action-executing"
const ActionReplyResponseSchemaJsonActionReplyStateActionFailure ActionReplyResponseSchemaJsonActionReplyState = "action-failure"
const ActionReplyResponseSchemaJsonActionReplyStateActionSuccess ActionReplyResponseSchemaJsonActionReplyState = "action-success"

// var enumValues_ActionReplyResponseSchemaJsonActionReplyState = []interface{}{
// 	"action-confirmed",
// 	"action-executing",
// 	"action-success",
// 	"action-failure",
// }

type ActionReplyResponseSchemaJsonActionReplyPayloadV2Payload map[string]interface{}

// Error codes for the action reply payload.
// The error codes have two purposes:
// 1. To allow the frontend to determine if the action can be retried or not
// 2. To display them to the user
// if an error code starts with "ERR_RETRY_" the action can be retried (see fetcher.js in the frontend)
const (
	// ErrParseFailed is the error code for a failed parse of the action payload.
	// the error is not retryable because the parsing is deterministic
	ErrParseFailed = "ERR_ACTION_PARSE_FAILED"
	// ErrValidationFailed is the error code for a failed validation of the action payload.
	// the error is not retryable because the validation is deterministic
	ErrValidationFailed = "ERR_VALIDATION_FAILED"
	// ErrRetryRollbackTimeout is the error code for a timeout during the dfc deployment.
	// It is retryable because the timeout might be caused by a busy system.
	ErrRetryRollbackTimeout = "ERR_RETRY_ROLLBACK_TIMEOUT"
	// ErrConfigFileInvalid is sent when the deployment of a dfc fails because the config file is invalid.
	ErrConfigFileInvalid = "ERR_CONFIG_FILE_INVALID"
	// ErrRetryConfigWriteFailed is the error code for a config file write failure.
	// It is retryable because the write failure might be caused by temporary filesystem issues.
	ErrRetryConfigWriteFailed = "ERR_RETRY_CONFIG_WRITE_FAILED"
	// ErrGetCacheModTimeFailed is the error code for a failed cache mod time retrieval.
	// It is not retryable because we already changed the config file and the user should refresh the page.
	ErrGetCacheModTimeFailed = "ERR_GET_CACHE_MOD_TIME_FAILED"
)

type ProtocolConverterConnection struct {
	IP   string `json:"ip" binding:"required"`
	Port uint32 `json:"port" binding:"required"`
}

type ProtocolConverterDFC struct {
	IgnoreErrors *bool                                 `json:"ignoreErrors,omitempty" yaml:"ignoreErrors,omitempty" mapstructure:"ignoreErrors,omitempty"`
	Inputs       CommonDataFlowComponentInputConfig    `json:"inputs" yaml:"inputs" mapstructure:"inputs"`
	Pipeline     CommonDataFlowComponentPipelineConfig `json:"pipeline" yaml:"pipeline" mapstructure:"pipeline"`
	RawYAML      *CommonDataFlowComponentRawYamlConfig `json:"rawYAML,omitempty" yaml:"rawYAML,omitempty" mapstructure:"rawYAML,omitempty"`
}

type ProtocolConverterVariable struct {
	Label string `json:"label" yaml:"label" mapstructure:"label"`
	Value string `json:"value" yaml:"value" mapstructure:"value"`
}

type ProtocolConverterTemplateInfo struct {
	IsTemplated bool                        `json:"isTemplated" yaml:"isTemplated" mapstructure:"isTemplated"`
	Variables   []ProtocolConverterVariable `json:"variables" yaml:"variables" mapstructure:"variables"`
	RootUUID    uuid.UUID                   `json:"rootUUID" yaml:"rootUUID" mapstructure:"rootUUID"`
}

type ProtocolConverter struct {
	UUID         *uuid.UUID                     `json:"uuid" binding:"required"`
	Name         string                         `json:"name" binding:"required"`
	Location     map[int]string                 `json:"location"`
	Connection   ProtocolConverterConnection    `json:"connection"`
	ReadDFC      *ProtocolConverterDFC          `json:"readDFC"`
	WriteDFC     *ProtocolConverterDFC          `json:"writeDFC"`
	TemplateInfo *ProtocolConverterTemplateInfo `json:"templateInfo"`
	Meta         *ProtocolConverterMeta         `json:"meta"`
}

type ProtocolConverterMeta struct {
	ProcessingMode string `json:"processingMode"`
	Protocol       string `json:"protocol"`
}

// StreamProcessorModelRef represents a reference to a data model
type StreamProcessorModelRef struct {
	Name    string `json:"name" binding:"required"`    // name of the referenced data model
	Version string `json:"version" binding:"required"` // version of the referenced data model
}

// StreamProcessorVariable represents a template variable
type StreamProcessorVariable struct {
	Label string `json:"label" yaml:"label" mapstructure:"label"`
	Value string `json:"value" yaml:"value" mapstructure:"value"`
}

// StreamProcessorTemplateInfo contains template information
type StreamProcessorTemplateInfo struct {
	IsTemplated bool                      `json:"isTemplated" yaml:"isTemplated" mapstructure:"isTemplated"`
	Variables   []StreamProcessorVariable `json:"variables" yaml:"variables" mapstructure:"variables"`
	RootUUID    uuid.UUID                 `json:"rootUUID" yaml:"rootUUID" mapstructure:"rootUUID"`
}

// StreamProcessorConfig represents the configuration for a stream processor
type StreamProcessorConfig struct {
	Sources StreamProcessorSourceMapping `yaml:"sources"` // source alias to UNS topic mapping
	Mapping StreamProcessorMapping       `yaml:"mapping"` // field mappings
}

// StreamProcessorSourceMapping represents the source mapping (alias to UNS topic)
type StreamProcessorSourceMapping map[string]string

// StreamProcessorMapping represents field mappings with support for nested structures
// where the key is the output field name and the value is either:
// - A string transformation expression (e.g., "source_alias * 2")
// - A nested map[string]interface{} for complex field structures
type StreamProcessorMapping map[string]interface{}

// StreamProcessor represents a stream processor configuration
type StreamProcessor struct {
	UUID          *uuid.UUID                   `json:"uuid" binding:"required"`
	Name          string                       `json:"name" binding:"required"`
	Location      map[int]string               `json:"location"`
	Model         StreamProcessorModelRef      `json:"model" binding:"required"`
	EncodedConfig string                       `json:"encodedConfig" binding:"required"` // base64-encoded YAML structure containing sources and mappings
	Config        *StreamProcessorConfig       `json:"-"`                                // parsed configuration (not used in the action, but filled by the action)
	TemplateInfo  *StreamProcessorTemplateInfo `json:"templateInfo"`
}

// GetStreamProcessorPayload contains the necessary fields for getting a stream processor.
type GetStreamProcessorPayload struct {
	UUID uuid.UUID `json:"uuid" binding:"required"`
}

// DeleteStreamProcessorPayload contains the UUID of the stream processor to delete.
type DeleteStreamProcessorPayload struct {
	UUID string `json:"uuid" binding:"required"`
}
