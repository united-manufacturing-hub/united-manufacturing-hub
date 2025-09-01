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
	Name              string                         `binding:"required"       json:"name"`
	Meta              CdcfMeta                       `json:"meta"`
	Payload           DeployDataFlowComponentPayload `binding:"required"       json:"payload"`
	IgnoreHealthCheck bool                           `json:"ignoreHealthCheck"`
}

type DeployDataFlowComponentPayload struct {
	CDFCPayload CDFCPayload `binding:"required" json:"customDataFlowComponent"`
}

type CDFCPayload struct {
	Pipeline        map[string]DfcDataConfig `json:"pipeline"`
	Inputs          DfcDataConfig            `json:"inputs"`
	Outputs         DfcDataConfig            `json:"outputs"`
	Inject          string                   `json:"inject"`
	BenthosImageTag string                   `json:"benthosImageTag"`
	IgnoreErrors    bool                     `json:"ignoreErrors"`
}

type DfcDataConfig struct {
	Data string `json:"data"`
	Type string `json:"type"`
}

type CdcfMeta struct {
	Type string `json:"type"`
}

// EditInstanceLocation holds the location information for the instance.
type EditInstanceLocationModel struct {
	Site       *string `json:"site,omitempty"`
	Area       *string `json:"area,omitempty"`
	Line       *string `json:"line,omitempty"`
	WorkCell   *string `json:"workCell,omitempty"`
	Enterprise string  `json:"enterprise"`
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
	Metadata     *MessageMetadata `json:"metadata"`
	Email        string           `json:"email"`
	Content      string           `json:"content"`
	InstanceUUID uuid.UUID        `json:"umhInstance"`
}

// Define MessageType as a custom type for better type safety.
type MessageType string

const (
	Subscribe        MessageType = "subscribe"
	Status           MessageType = "status"
	Action           MessageType = "action"
	ActionReply      MessageType = "action-reply"
	EncryptedContent MessageType = "encrypted-content"
)

// UMHMessageContent holds information about the type of the message.
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
	// It does not contain sensitive information, like authentication.
	GetDatasourceBasic ActionType = "get-datasource-basic"
	// EditOPCUADatasource represents an action type for editing an OPC-UA datasource.
	EditOPCUADatasource ActionType = "edit-opcua-datasource"
	// DeleteDatasource represents the action type for deleting a data source.
	DeleteDatasource ActionType = "delete-datasource"
	// UpgradeCompanion represents the action type for upgrading to a specific version.
	UpgradeCompanion ActionType = "upgrade-companion"
	// EditMqttBroker sets the MQTT broker details.
	EditMqttBroker ActionType = "edit-mqtt-broker"
	// DeployProtocolConverter represents the action type for deploying a protocol converter.
	DeployProtocolConverter ActionType = "deploy-protocol-converter"
	// DeleteProtocolConverter represents the action type for deleting a protocol converter.
	DeleteProtocolConverter ActionType = "delete-protocol-converter"
	// EditProtocolConverter represents the action type for editing a protocol converter.
	EditProtocolConverter ActionType = "edit-protocol-converter"
	// GetProtocolConverter represents the action type for getting a protocol converter.
	GetProtocolConverter ActionType = "get-protocol-converter"
	// GetAuditLog represents the action type for getting the audit log.
	GetAuditLog ActionType = "get-audit-log"
	// EditInstanceLocation represents the action type for setting the location of the UMH instance.
	EditInstanceLocation ActionType = "edit-instance-location"
	// EditInstance represents the action type for editing an UMH instance.
	EditInstance ActionType = "edit-instance"
	// DeployDataFlowComponent represents the action type for deploying a data flow component.
	DeployDataFlowComponent ActionType = "deploy-data-flow-component"
	// EditDataFlowComponent represents the action type for editing a data flow component.
	EditDataFlowComponent ActionType = "edit-data-flow-component"
	// GetDataFlowComponent represents the action type for retrieving one or multiple data flow components.
	GetDataFlowComponent ActionType = "get-data-flow-component"
	// DeleteDataFlowComponent represents the action type for deleting a data flow component.
	DeleteDataFlowComponent ActionType = "delete-data-flow-component"
	// GetDataFlowComponentMetrics represents the action type for retrieving metrics of a data flow component
	//
	// Deprecated: Use GetMetrics instead. Kept for backward compatibility.
	GetDataFlowComponentMetrics ActionType = "get-data-flow-component-metrics"
	// GetDataFlowComponentLog reperesents the action type for getting the audit log for a data flow component.
	GetDataFlowComponentLog ActionType = "get-data-flow-component-log"
	// GetKubernetesEvents represents the action type for retrieving Kubernetes events.
	GetKubernetesEvents ActionType = "get-kubernetes-events"
	// GetOPCUATags represents the action type to retrieve all opcua tags.
	GetOPCUATags ActionType = "get-opcua-tags"
	// RollbackDataFlowComponent represents the action type for rolling back a data flow component.
	RollbackDataFlowComponent ActionType = "rollback-data-flow-component"
	// AllowAppSecretAccess is a META action type for allowing the app to access the secret, this is not a real action but used in the subscription flow.
	AllowAppSecretAccess ActionType = "allow-app-secret-access"
	// GetConfiguration represents the action type for retrieving a configuration.
	GetConfiguration ActionType = "get-configuration"
	// UpdateConfiguration represents the action type for updating a configuration.
	UpdateConfiguration ActionType = "update-configuration"
	// GetLogs represents the action type for retrieving logs.
	GetLogs ActionType = "get-logs"
	// GetMetrics represents the action type for retrieving metrics.
	GetMetrics ActionType = "get-metrics"
	// GetConfigFile represents the action type for retrieving the configuration file.
	GetConfigFile ActionType = "get-config-file"
	// SetConfigFile represents the action type for updating the configuration file.
	SetConfigFile ActionType = "set-config-file"
	// AddDataModel represents the action type for adding a data model.
	AddDataModel ActionType = "add-datamodel"
	// DeleteDataModel represents the action type for deleting a data model.
	DeleteDataModel ActionType = "delete-datamodel"
	// EditDataModel represents the action type for editing a data model.
	EditDataModel ActionType = "edit-datamodel"
	// GetDataModel represents the action type for retrieving a data model.
	GetDataModel ActionType = "get-datamodel"
	// DeployStreamProcessor represents the action type for deploying a stream processor.
	DeployStreamProcessor ActionType = "deploy-stream-processor"
	// EditStreamProcessor represents the action type for editing a stream processor.
	EditStreamProcessor ActionType = "edit-stream-processor"
	// DeleteStreamProcessor represents the action type for deleting a stream processor.
	DeleteStreamProcessor ActionType = "delete-stream-processor"
	// GetStreamProcessor represents the action type for getting a stream processor.
	GetStreamProcessor ActionType = "get-stream-processor"
)

// TestNetworkConnectionPayload contains the necessary fields for executing a TestNetworkConnection action.
type TestNetworkConnectionPayload struct {
	IP   string                   `binding:"required" json:"ip"`         // IP address of the target
	Type DatasourceConnectionType `binding:"required" json:"datasource"` // Type of data source connection (e.g., OPC-UA)
	Port uint32                   `binding:"required" json:"port"`       // Port to connect on
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
	IP          string    `binding:"required" json:"ip"`
	Username    string    `binding:"required" json:"username"`
	Password    string    `binding:"required" json:"password"`
	LastUpdated uint64    `json:"lastUpdated"`
	Port        uint32    `binding:"required" json:"port"`
	UUID        uuid.UUID `binding:"required" json:"uuid"`
}

// EditInstanceLocationAction contains the necessary fields for setting the location of the UMH instance.
type EditInstanceLocationAction struct {
	Site       *string `json:"site"`
	Area       *string `json:"area"`
	Line       *string `json:"line"`
	WorkCell   *string `json:"workCell"`
	Enterprise string  `binding:"required" json:"enterprise"`
}

// GetProtocolConverterPayload contains the necessary fields for getting a protocol converter.
type GetProtocolConverterPayload struct {
	UUID uuid.UUID `binding:"required" json:"uuid"`
}

// DeployConnectionPayload contains the necessary fields for executing a DeployConnection action.
type DeployConnectionPayload struct {
	Location *ConnectionLocation      `json:"location"`
	Name     string                   `binding:"required" json:"name"`
	IP       string                   `binding:"required" json:"ip"`
	Type     DatasourceConnectionType `binding:"required" json:"datasource"`
	Notes    string                   `json:"notes"`
	Port     uint32                   `binding:"required" json:"port"`
	Uuid     uuid.UUID                `binding:"required" json:"uuid"`
}

type GetConfigurationPayload struct {
	ConfigType  string   `json:"configType"`  // For backward compatibility
	ConfigTypes []string `json:"configTypes"` // New field for multiple config types
}

type UpdateConfigurationPayload struct {
	Config     map[string]interface{} `binding:"required" json:"config"`
	ConfigType string                 `binding:"required" json:"configType"`
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
	Location *ConnectionLocation       `json:"location,omitempty"`
	Uuid     uuid.UUID                 `binding:"required"          json:"uuid"`
}

// DeleteConnectionPayload only contains the UUID to identify the connection to delete.
//
// TODO: Should we maybe have a single struct with a required UUID field and reuse it? (e.g. IdentifierPayload).
type DeleteConnectionPayload struct {
	Uuid uuid.UUID `binding:"required" json:"uuid"`
}

// GetConnectionNotesPayload contains the necessary fields for executing a GetConnectionNotes action.
type GetConnectionNotesPayload struct {
	Uuid uuid.UUID `binding:"required" json:"uuid"`
}

// GetConnectionNotesReplyPayload contains the notes of a connection to be sent as a reply to a GetConnectionNotes action.
type GetConnectionNotesReplyPayload struct {
	Notes string `json:"notes"`
}

type GetDatasourcePayload struct {
	Uuid uuid.UUID `binding:"required" json:"uuid"`
}

type DatasourceAuthentication struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	CertificatePub  string `json:"certificatePub"`
	CertificatePriv string `json:"certificatePriv"`
}

// DeleteDatasourcePayload only contains the UUID to identify the data source to delete.
type DeleteDatasourcePayload struct {
	Uuid uuid.UUID `binding:"required" json:"uuid"`
}

// UpgradeCompanionPayload contains the versio the user wants to upgrade to.
type UpgradeCompanionPayload struct {
	Version semver.Version `binding:"required" json:"version"`
}

type DeployProtocolConverterPayload struct {
	Metadata map[string]string `json:"metadata,omitempty"`

	// Generic only
	InputYaml      *string   `json:"inputYaml,omitempty"`
	ProcessorYaml  *string   `json:"processorYaml,omitempty"`
	InjectYaml     *string   `json:"injectYaml,omitempty"`
	Type           Protocol  `binding:"required"             json:"type"`
	Name           string    `binding:"required"             json:"name"`
	ConnectionUUID uuid.UUID `binding:"required"             json:"connectionUUID"`
	UUID           uuid.UUID `binding:"required"             json:"uuid"`
	IgnoreErrors   bool      `json:"ignoreErrors,omitempty"`
}

type DeleteProtocolConverterPayload struct {
	UUID uuid.UUID `binding:"required" json:"uuid"`
}

type EditProtocolConverterPayload struct {
	Name     *string           `json:"name,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`

	// Generic only
	InputYaml     *string   `json:"inputYaml,omitempty"`
	ProcessorYaml *string   `json:"processorYaml,omitempty"`
	InjectYaml    *string   `json:"injectYaml,omitempty"`
	UUID          uuid.UUID `binding:"required"             json:"uuid"`

	IgnoreErrors bool `json:"ignoreErrors,omitempty"`
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
	ActionReplyPayload interface{} `binding:"required" json:"actionReplyPayload"`
	// ActionContext is an optional field that can be used to provide additional context for the action.
	ActionContext    map[string]interface{} `json:"actionContext,omitempty"`
	ActionReplyState ActionReplyState       `binding:"required"             json:"actionReplyState"`
	ActionUUID       uuid.UUID              `binding:"required"             json:"actionUUID"`
}

// this is the structure of the action that the frontend sends in the first place.
type CustomDFCPayload struct {
	CustomDataFlowComponent struct {
		Pipeline struct {
			Processors map[string]struct {
				Type string `json:"type"`
				Data string `json:"data"`
			} `json:"processors"`
		} `json:"pipeline"`
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
	} `json:"customDataFlowComponent"`
}

// DeleteDFCPayload contains the UUID of the component to delete.
type DeleteDFCPayload struct {
	UUID string `json:"uuid"`
}

type EditDataflowcomponentRequestSchemaJson struct {
	IgnoreHealthCheck *bool `json:"ignoreHealthCheck,omitempty" mapstructure:"ignoreHealthCheck,omitempty" yaml:"ignoreHealthCheck,omitempty"`

	BasedOnUuid string `json:"basedOnUuid" mapstructure:"basedOnUuid" yaml:"basedOnUuid"`

	Meta CdcfMeta `json:"meta" mapstructure:"meta" yaml:"meta"`

	Name string `json:"name" mapstructure:"name" yaml:"name"`

	UUID string `json:"uuid" mapstructure:"uuid" yaml:"uuid"`

	Payload DeployDataFlowComponentPayload `json:"payload" mapstructure:"payload" yaml:"payload"`
}

type GetDataflowcomponentResponse map[string]GetDataflowcomponentResponseContent

type GetDataflowcomponentResponseContent struct {

	// Meta corresponds to the JSON schema field "meta".
	Meta CommonDataFlowComponentMeta `json:"meta" mapstructure:"meta" yaml:"meta"`

	// ParentDFC corresponds to the JSON schema field "parentDFC".
	ParentDFC interface{} `json:"parentDFC,omitempty" mapstructure:"parentDFC,omitempty" yaml:"parentDFC,omitempty"`

	// Payload corresponds to the JSON schema field "payload".
	Payload Payload `json:"payload" mapstructure:"payload" yaml:"payload"`

	// Creator corresponds to the JSON schema field "creator".
	Creator string `json:"creator" mapstructure:"creator" yaml:"creator"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name" mapstructure:"name" yaml:"name"`

	// State corresponds to the JSON schema field "state".
	State string `json:"state" mapstructure:"state" yaml:"state"`
	// CreationTime corresponds to the JSON schema field "creationTime".
	CreationTime float64 `json:"creationTime" mapstructure:"creationTime" yaml:"creationTime"`
}

type Payload interface{}

type CommonDataFlowComponentMeta struct {
	AdditionalProperties interface{}
	// Key value pairs of additional metadata
	AdditionalMetadata map[string]interface{} `json:"additionalMetadata,omitempty" mapstructure:"additionalMetadata,omitempty" yaml:"additionalMetadata,omitempty"`

	// The type of the dataflow component
	Type string `json:"type" mapstructure:"type" yaml:"type"`
}

type GetDataflowcomponentRequestSchemaJson struct {
	// VersionUUIDs corresponds to the JSON schema field "versionUUIDs".
	VersionUUIDs []string `json:"versionUUIDs" mapstructure:"versionUUIDs" yaml:"versionUUIDs"`
}

type CommonDataFlowComponentCDFCPropertiesPayload struct {
	CDFCProperties CommonDataFlowComponentCDFCProperties `json:"customDataFlowComponent" mapstructure:"customDataFlowComponent" yaml:"customDataFlowComponent"`
}

type CommonDataFlowComponentCDFCProperties struct {

	// Pipeline corresponds to the JSON schema field "pipeline".
	Pipeline CommonDataFlowComponentPipelineConfig `json:"pipeline" mapstructure:"pipeline" yaml:"pipeline"`

	// BenthosImageTag corresponds to the JSON schema field "benthosImageTag".
	BenthosImageTag *CommonDataFlowComponentBenthosImageTagConfig `json:"benthosImageTag,omitempty" mapstructure:"benthosImageTag,omitempty" yaml:"benthosImageTag,omitempty"`

	// this is only here to make the json schema happy
	IgnoreErrors *bool `json:"ignoreErrors,omitempty" mapstructure:"ignoreErrors,omitempty" yaml:"ignoreErrors,omitempty"`

	// RawYAML corresponds to the JSON schema field "rawYAML".
	RawYAML *CommonDataFlowComponentRawYamlConfig `json:"rawYAML,omitempty" mapstructure:"rawYAML,omitempty" yaml:"rawYAML,omitempty"`

	// Inputs corresponds to the JSON schema field "inputs".
	Inputs CommonDataFlowComponentInputConfig `json:"inputs" mapstructure:"inputs" yaml:"inputs"`

	// Outputs corresponds to the JSON schema field "outputs".
	Outputs CommonDataFlowComponentOutputConfig `json:"outputs" mapstructure:"outputs" yaml:"outputs"`
}

type CommonDataFlowComponentBenthosImageTagConfig struct {
	// this is only here to make the json schema happy
	Tag *string `json:"tag,omitempty" mapstructure:"tag,omitempty" yaml:"tag,omitempty"`
}

type CommonDataFlowComponentInputConfig struct {
	// This is the YAML data for the input
	Data string `json:"data" mapstructure:"data" yaml:"data"`

	// This can for example be mqtt
	Type string `json:"type" mapstructure:"type" yaml:"type"`
}

type CommonDataFlowComponentOutputConfig struct {
	// This is the YAML data for the output
	Data string `json:"data" mapstructure:"data" yaml:"data"`

	// This can for example be kafka
	Type string `json:"type" mapstructure:"type" yaml:"type"`
}

type CommonDataFlowComponentPipelineConfig struct {
	// Processors corresponds to the JSON schema field "processors".
	Processors CommonDataFlowComponentPipelineConfigProcessors `json:"processors" mapstructure:"processors" yaml:"processors"`

	// If unset, defaults to -1, see also
	// https://docs.redpanda.com/redpanda-connect/configuration/processing_pipelines/
	Threads *int `json:"threads,omitempty" mapstructure:"threads,omitempty" yaml:"threads,omitempty"`
}

type CommonDataFlowComponentPipelineConfigProcessors map[string]struct {
	// This is the YAML data for the processor
	Data string `json:"data" mapstructure:"data" yaml:"data"`

	// This can for example be bloblang
	Type string `json:"type" mapstructure:"type" yaml:"type"`
}

type CommonDataFlowComponentRawYamlConfig struct {
	// This is the raw yaml data
	Data string `json:"data" mapstructure:"data" yaml:"data"`
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
	// Type represents the type of the logs to retrieve
	Type LogType `json:"type"`
	// UUID represents the identifier of the entity to retrieve the logs for.
	// This is optional and only used for DFC and Protocol Converter logs.
	UUID string `json:"uuid"`
	// StartTime represents the time frame to start the logs from in unix milliseconds
	StartTime int64 `json:"startTime"`
}

type GetLogsResponse struct {
	Logs []string `json:"logs"`
}

type MetricResourceType string

const (
	DFCMetricResourceType               MetricResourceType = "dfc"
	RedpandaMetricResourceType          MetricResourceType = "redpanda"
	TopicBrowserMetricResourceType      MetricResourceType = "topic-browser"
	StreamProcessorMetricResourceType   MetricResourceType = "stream-processor"
	ProtocolConverterMetricResourceType MetricResourceType = "protocol-converter"
)

// GetMetricsRequest contains the necessary fields for executing a `get-metrics` action.
type GetMetricsRequest struct {
	// Type represents the type of the resource to retrieve the metrics for
	Type MetricResourceType `binding:"required" json:"type"`
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

// GetConfigFileResponse contains the config file content.
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

// ModelRef represents a reference to another data model.
type ModelRef struct {
	Name    string `yaml:"name"`    // name of the referenced data model
	Version string `yaml:"version"` // version of the referenced data model
}

type Field struct {
	ModelRef     *ModelRef        `yaml:"_refModel,omitempty"`     // this is a special field that is used to reference another data model to be used as a type for this field
	Subfields    map[string]Field `yaml:",inline"`                 // subfields of the field (allow recursive definition of fields)
	PayloadShape string           `yaml:"_payloadshape,omitempty"` // payload shape of the field
}

// AddDataModelPayload contains the necessary fields for executing an AddDataModel action.
type AddDataModelPayload struct {
	Structure        map[string]Field `json:"-"`                                             // Data model version (not used in the action, but filled by the action)
	Name             string           `binding:"required"           json:"name"`             // Name of the data model
	Description      string           `json:"description,omitempty"`                         // Description of the data model
	EncodedStructure string           `binding:"required"           json:"encodedStructure"` // Encoded structure of the data model
}

// DeleteDataModelPayload contains the necessary fields for executing a DeleteDataModel action.
type DeleteDataModelPayload struct {
	Name string `binding:"required" json:"name"` // Name of the data model to delete
}

// EditDataModelPayload contains the necessary fields for executing an EditDataModel action.
type EditDataModelPayload struct {
	Structure        map[string]Field `json:"-"`                                             // Data model version (not used in the action, but filled by the action)
	Name             string           `binding:"required"           json:"name"`             // Name of the data model to edit
	Description      string           `json:"description,omitempty"`                         // Description of the data model
	EncodedStructure string           `binding:"required"           json:"encodedStructure"` // Encoded structure of the data model
}

// GetDataModelPayload contains the necessary fields for executing a GetDataModel action.
type GetDataModelPayload struct {
	Name            string `binding:"required"               json:"name"` // Name of the data model to retrieve
	GetEnrichedTree bool   `json:"getEnrichedTree,omitempty"`             // Whether to fill refModel fields with actual model data
}

// GetDataModelVersion represents a version of a data model with base64-encoded structure.
type GetDataModelVersion struct {
	Structure        map[string]Field `json:"-"`                     // Data model version structure (not sent in response, but used internally)
	Description      string           `json:"description,omitempty"` // Description of the data model version
	EncodedStructure string           `json:"encodedStructure"`      // Base64-encoded structure of the data model version
}

// GetDataModelResponse contains the response for a GetDataModel action.
type GetDataModelResponse struct {
	Versions    map[string]GetDataModelVersion `json:"versions"`              // All versions of the data model
	Name        string                         `json:"name"`                  // Name of the data model
	Description string                         `json:"description,omitempty"` // Description of the data model
}

// Deprecated: Use GetMetricsRequest instead.
type GetDataflowcomponentMetricsRequest struct {
	UUID string `binding:"required" json:"uuid"`
}

type ActionReplyResponseSchemaJson struct {
	// Additional contextual data for the action, allows arbitrary key-value pairs.
	ActionContext ActionReplyResponseSchemaJsonActionContext `json:"actionContext,omitempty" mapstructure:"actionContext,omitempty" yaml:"actionContext,omitempty"`

	// Legacy action reply payload, can be a string or an object for backward
	// compatibility.
	ActionReplyPayload interface{} `json:"actionReplyPayload" mapstructure:"actionReplyPayload" yaml:"actionReplyPayload"`

	// Structured response payload for any action reply.
	ActionReplyPayloadV2 *ActionReplyResponseSchemaJsonActionReplyPayloadV2 `json:"actionReplyPayloadV2,omitempty" mapstructure:"actionReplyPayloadV2,omitempty" yaml:"actionReplyPayloadV2,omitempty"`

	// State of the action reply.
	ActionReplyState ActionReplyResponseSchemaJsonActionReplyState `json:"actionReplyState" mapstructure:"actionReplyState" yaml:"actionReplyState"`

	// Unique identifier for the action.
	ActionUUID string `json:"actionUUID" mapstructure:"actionUUID" yaml:"actionUUID"`
}

// Additional contextual data for the action, allows arbitrary key-value pairs.
type ActionReplyResponseSchemaJsonActionContext map[string]interface{}

type ActionReplyResponseSchemaJsonActionReplyPayloadV2 struct {
	// Machine-readable error code (e.g., 'ERR_CONFIG_CHANGED').
	ErrorCode *string `json:"errorCode,omitempty" mapstructure:"errorCode,omitempty" yaml:"errorCode,omitempty"`

	// Additional payload for the action reply.
	Payload ActionReplyResponseSchemaJsonActionReplyPayloadV2Payload `json:"payload,omitempty" mapstructure:"payload,omitempty" yaml:"payload,omitempty"`

	// Human-readable error message.
	Message string `json:"message" mapstructure:"message" yaml:"message"`
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
// if an error code starts with "ERR_RETRY_" the action can be retried (see fetcher.js in the frontend).
const (
	// ErrParseFailed is the error code for a failed parse of the action payload.
	// the error is not retryable because the parsing is deterministic.
	ErrParseFailed = "ERR_ACTION_PARSE_FAILED"
	// ErrValidationFailed is the error code for a failed validation of the action payload.
	// the error is not retryable because the validation is deterministic.
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
	IP   string `binding:"required" json:"ip"`
	Port uint32 `binding:"required" json:"port"`
}

type ProtocolConverterDFC struct {
	Pipeline     CommonDataFlowComponentPipelineConfig `json:"pipeline"               mapstructure:"pipeline"               yaml:"pipeline"`
	IgnoreErrors *bool                                 `json:"ignoreErrors,omitempty" mapstructure:"ignoreErrors,omitempty" yaml:"ignoreErrors,omitempty"`
	RawYAML      *CommonDataFlowComponentRawYamlConfig `json:"rawYAML,omitempty"      mapstructure:"rawYAML,omitempty"      yaml:"rawYAML,omitempty"`
	Inputs       CommonDataFlowComponentInputConfig    `json:"inputs"                 mapstructure:"inputs"                 yaml:"inputs"`
}

type ProtocolConverterVariable struct {
	Label string `json:"label" mapstructure:"label" yaml:"label"`
	Value string `json:"value" mapstructure:"value" yaml:"value"`
}

type ProtocolConverterTemplateInfo struct {
	Variables   []ProtocolConverterVariable `json:"variables"   mapstructure:"variables"   yaml:"variables"`
	RootUUID    uuid.UUID                   `json:"rootUUID"    mapstructure:"rootUUID"    yaml:"rootUUID"`
	IsTemplated bool                        `json:"isTemplated" mapstructure:"isTemplated" yaml:"isTemplated"`
}

type ProtocolConverter struct {
	UUID         *uuid.UUID                     `binding:"required"  json:"uuid"`
	Location     map[int]string                 `json:"location"`
	ReadDFC      *ProtocolConverterDFC          `json:"readDFC"`
	WriteDFC     *ProtocolConverterDFC          `json:"writeDFC"`
	TemplateInfo *ProtocolConverterTemplateInfo `json:"templateInfo"`
	Meta         *ProtocolConverterMeta         `json:"meta"`
	Name         string                         `binding:"required"  json:"name"`
	Connection   ProtocolConverterConnection    `json:"connection"`
}

type ProtocolConverterMeta struct {
	ProcessingMode string `json:"processingMode"`
	Protocol       string `json:"protocol"`
}

// StreamProcessorModelRef represents a reference to a data model.
type StreamProcessorModelRef struct {
	Name    string `binding:"required" json:"name"`    // name of the referenced data model
	Version string `binding:"required" json:"version"` // version of the referenced data model
}

// StreamProcessorVariable represents a template variable.
type StreamProcessorVariable struct {
	Label string `json:"label" mapstructure:"label" yaml:"label"`
	Value string `json:"value" mapstructure:"value" yaml:"value"`
}

// StreamProcessorTemplateInfo contains template information.
type StreamProcessorTemplateInfo struct {
	Variables   []StreamProcessorVariable `json:"variables"   mapstructure:"variables"   yaml:"variables"`
	RootUUID    uuid.UUID                 `json:"rootUUID"    mapstructure:"rootUUID"    yaml:"rootUUID"`
	IsTemplated bool                      `json:"isTemplated" mapstructure:"isTemplated" yaml:"isTemplated"`
}

// StreamProcessorConfig represents the configuration for a stream processor.
type StreamProcessorConfig struct {
	Sources StreamProcessorSourceMapping `yaml:"sources"` // source alias to UNS topic mapping
	Mapping StreamProcessorMapping       `yaml:"mapping"` // field mappings
}

// StreamProcessorSourceMapping represents the source mapping (alias to UNS topic).
type StreamProcessorSourceMapping map[string]string

// StreamProcessorMapping represents field mappings with support for nested structures
// where the key is the output field name and the value is either:
// - A string transformation expression (e.g., "source_alias * 2")
// - A nested map[string]interface{} for complex field structures.
type StreamProcessorMapping map[string]interface{}

// StreamProcessor represents a stream processor configuration.
type StreamProcessor struct {
	UUID              *uuid.UUID                   `binding:"required"                 json:"uuid"`
	Location          map[int]string               `json:"location"`
	Config            *StreamProcessorConfig       `json:"-"` // parsed configuration (not used in the action, but filled by the action)
	TemplateInfo      *StreamProcessorTemplateInfo `json:"templateInfo"`
	Model             StreamProcessorModelRef      `binding:"required"                 json:"model"`
	Name              string                       `binding:"required"                 json:"name"`
	EncodedConfig     string                       `binding:"required"                 json:"encodedConfig"` // base64-encoded YAML structure containing sources and mappings
	IgnoreHealthCheck bool                         `json:"ignoreHealthCheck,omitempty"`
}

// GetStreamProcessorPayload contains the necessary fields for getting a stream processor.
type GetStreamProcessorPayload struct {
	UUID uuid.UUID `binding:"required" json:"uuid"`
}

// DeleteStreamProcessorPayload contains the UUID of the stream processor to delete.
type DeleteStreamProcessorPayload struct {
	UUID string `binding:"required" json:"uuid"`
}
