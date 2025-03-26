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

// Schema for the status message containing system state and DFC information
type StatusMessage struct {
	Core Core `json:"core"`
	// TBD: this is a placeholder for future plugins
	Plugins map[string]interface{} `json:"plugins"`
}

type Core struct {
	Agent            Agent            `json:"agent"`
	Container        Container        `json:"container"`
	Dfcs             []Dfc            `json:"dfcs"`
	Redpanda         Redpanda         `json:"redpanda"`
	UnifiedNamespace UnifiedNamespace `json:"unifiedNamespace"`
	Release          Release          `json:"release"`
}

type Agent struct {
	Health  *Health  `json:"health"`
	Latency *Latency `json:"latency"`
	// Hierarchical location of the agent within the factory.
	// The map keys represent different levels in the hierarchy, ordered from top to bottom:
	//
	// - 0: Enterprise level
	// - 1: Site level
	// - 2: Production line level
	// - ... (Additional levels as needed)
	//
	// The values are the corresponding location names.
	Location map[int]string `json:"location"`
}

type HealthCategory int

const (
	Neutral HealthCategory = iota
	Active
	Degraded
)

type Health struct {
	// Human-readable message describing the health state
	Message string `json:"message"`
	// Observed state of the component
	ObservedState string `json:"state"`
	// Desired state of the component
	DesiredState string `json:"desiredState"`
	// Category of the health state for easy classification
	Category HealthCategory `json:"category"`
}

type Latency struct {
	AvgMs float64 `json:"avgMs"`
	MaxMs float64 `json:"maxMs"`
	MinMs float64 `json:"minMs"`
	P95Ms float64 `json:"p95Ms"`
	P99Ms float64 `json:"p99Ms"`
}

type ContainerArchitecture string

const (
	ContainerArchitectureArm64 ContainerArchitecture = "arm64"
	ContainerArchitectureAmd64 ContainerArchitecture = "amd64"
)

type Container struct {
	Health *Health `json:"health"`
	CPU    *CPU    `json:"cpu"`
	Disk   *Disk   `json:"disk"`
	Memory *Memory `json:"memory"`
	Hwid   string  `json:"hwid"`
	// Processor architecture. Currently arm64 and amd64 are supported.
	Architecture ContainerArchitecture `json:"architecture"`
}

type CPU struct {
	Health *Health `json:"health"`
	// Total usage in milli-cores
	TotalUsageMCpu float64 `json:"totalUsageMCpu"`
	// Number of cores
	CoreCount int `json:"coreCount"`
}

type Disk struct {
	Health *Health `json:"health"`
	// Used bytes of the disk's data partition
	DataPartitionUsedBytes float64 `json:"dataPartitionUsedBytes"`
	// Total bytes of the disk's data partition
	DataPartitionTotalBytes float64 `json:"dataPartitionTotalBytes"`
}

type Memory struct {
	Health *Health `json:"health"`
	// Used bytes of the cgroup's memory
	CGroupUsedBytes float64 `json:"cGroupUsedBytes"`
	// Total bytes of the cgroup's memory
	CGroupTotalBytes float64 `json:"cGroupTotalBytes"`
}

type DfcType string

const (
	Custom            DfcType = "custom"
	DataBridge        DfcType = "data-bridge"
	ProtocolConverter DfcType = "protocol-converter"
)

type Dfc struct {
	// Deprecated: here for backward compatibility. Should just use `UUID` instead.
	CurrentVersionUUID *string     `json:"currentVersionUUID"`
	Name               *string     `json:"name"`
	UUID               string      `json:"uuid"`
	Health             *Health     `json:"health"`
	DfcType            DfcType     `json:"dfcType"`
	Metrics            *DFCMetrics `json:"metrics"`
	// For 'protocol-converter' type, this array contains exactly one connection.
	//
	// For 'data-bridge' type, this array always contains exactly two connections.
	//
	// For 'custom' type, this array is empty.
	Connections []Connection `json:"connections,omitempty"`
	// Additional information for 'data-bridge' type DFCs.
	Bridge *DfcBridgeInfo `json:"bridge,omitempty"`
}

type DFCMetrics struct {
	AvgInputThroughputPerMinuteInMsgSec float64 `json:"avgInputThroughputPerMinuteInMsgSec"`
}

type Connection struct {
	Name   string  `json:"name"`
	UUID   string  `json:"uuid"`
	Health *Health `json:"health"`
	// The connection URI in full, e.g., 'opc.tcp://hostname:port/path'. This includes the
	// scheme, host, port, and any required path elements.
	URI string `json:"uri"`
	// Last reported latency in milliseconds
	LastLatencyMs float64 `json:"lastLatencyMs"`
}

type DfcBridgeInfo struct {
	DataContract string `json:"dataContract"`
	InputType    string `json:"inputType"`
	OutputType   string `json:"outputType"`
}

type Redpanda struct {
	Health                                 *Health `json:"health"`
	AvgIncomingThroughputPerMinuteInMsgSec float64 `json:"avgIncomingThroughputPerMinuteInMsgSec"`
	AvgOutgoingThroughputPerMinuteInMsgSec float64 `json:"avgOutgoingThroughputPerMinuteInMsgSec"`
}

type UnifiedNamespace struct {
	EventsTable map[string]EventsTable `json:"eventsTable,omitempty"`
	UnsTable    map[string]UnsTable    `json:"unsTable,omitempty"`
}

type EventsTable struct {
	Bridges         []string    `json:"bridges"`
	Error           string      `json:"error"`
	Origin          *string     `json:"origin"`
	RawKafkaMessage EventKafka  `json:"rawKafkaMessage"`
	TimestampMS     float64     `json:"timestamp_ms"`
	UnsTreeID       string      `json:"unsTreeId"`
	Value           interface{} `json:"value"`
}

type EventKafka struct {
	Destination            []string          `json:"destination"`
	Headers                map[string]string `json:"headers"`
	KafkaInsertedTimestamp float64           `json:"kafkaInsertedTimestamp"`
	Key                    string            `json:"key"`
	LastPayload            string            `json:"lastPayload"`
	MessagesPerMinute      float64           `json:"messagesPerMinute"`
	Origin                 []string          `json:"origin"`
	Topic                  string            `json:"topic"`
}

type UnsTable struct {
	Area       *string  `json:"area,omitempty"`
	Enterprise string   `json:"enterprise"`
	EventGroup *string  `json:"eventGroup,omitempty"`
	EventTag   *string  `json:"eventTag,omitempty"`
	Instances  []string `json:"instances,omitempty"`
	IsError    *bool    `json:"isError,omitempty"`
	Line       *string  `json:"line,omitempty"`
	OriginID   *string  `json:"originId,omitempty"`
	Schema     *string  `json:"schema,omitempty"`
	Site       *string  `json:"site,omitempty"`
	WorkCell   *string  `json:"workCell,omitempty"`
}

type Release struct {
	Version string `json:"version"`
	Channel string `json:"channel"`
	// List of supported feature keywords. If a feature is supported, its corresponding keyword
	// will be included in the array.
	//
	// Note: at some point we should rethink this approach to avoid bloating the status message as
	// this list will likely grow over time.
	SupportedFeatures []string `json:"supportedFeatures"`
}
