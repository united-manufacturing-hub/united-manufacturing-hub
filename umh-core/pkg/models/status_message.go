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

// StatusMessage represents the complete system state including core components and plugins.
type StatusMessage struct {
	Core    Core                   `json:"core"`
	Plugins map[string]interface{} `json:"plugins"` // Extension point for future plugins
}

type Core struct {
	Health           *Health          `json:"health"`
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

// String returns the string representation of a HealthCategory
func (hc HealthCategory) String() string {
	switch hc {
	case Active:
		return "active"
	case Degraded:
		return "degraded"
	case Neutral:
		return "neutral"
	default:
		return "unknown"
	}
}

type Health struct {
	Message       string         `json:"message"`      // Human-readable message describing the health state
	ObservedState string         `json:"state"`        // Observed state of the component
	DesiredState  string         `json:"desiredState"` // Desired state of the component
	Category      HealthCategory `json:"category"`     // Category of the health state for easy classification
}

type Latency struct {
	AvgMs float64 `json:"avgMs"` // Average latency in milliseconds
	MaxMs float64 `json:"maxMs"` // Maximum latency in milliseconds
	MinMs float64 `json:"minMs"` // Minimum latency in milliseconds
	P95Ms float64 `json:"p95Ms"` // 95th percentile latency in milliseconds
	P99Ms float64 `json:"p99Ms"` // 99th percentile latency in milliseconds
}

// ContainerArchitecture represents the processor architecture of the container.
type ContainerArchitecture string

const (
	ArchitectureArm64 ContainerArchitecture = "arm64"
	ArchitectureAmd64 ContainerArchitecture = "amd64"
)

type Container struct {
	Health       *Health               `json:"health"`
	CPU          *CPU                  `json:"cpu"`
	Disk         *Disk                 `json:"disk"`
	Memory       *Memory               `json:"memory"`
	Hwid         string                `json:"hwid"`         // Hardware identifier
	Architecture ContainerArchitecture `json:"architecture"` // Processor architecture
}

type CPU struct {
	Health         *Health `json:"health"`
	TotalUsageMCpu float64 `json:"totalUsageMCpu"` // Total usage in milli-cores (1000m = 1 core)
	CoreCount      int     `json:"coreCount"`      // Number of CPU cores
}

type Disk struct {
	Health                  *Health `json:"health"`
	DataPartitionUsedBytes  int64   `json:"dataPartitionUsedBytes"`  // Used bytes of the disk's data partition
	DataPartitionTotalBytes int64   `json:"dataPartitionTotalBytes"` // Total bytes of the disk's data partition
}

type Memory struct {
	Health           *Health `json:"health"`
	CGroupUsedBytes  int64   `json:"cGroupUsedBytes"`  // Used bytes of the cgroup's memory
	CGroupTotalBytes int64   `json:"cGroupTotalBytes"` // Total bytes of the cgroup's memory
}

// DfcType represents the type of Data Flow Component.
type DfcType string

const (
	DfcTypeCustom            DfcType = "custom"
	DfcTypeDataBridge        DfcType = "data-bridge"
	DfcTypeProtocolConverter DfcType = "protocol-converter"
)

// Dfc represents a Data Flow Component.
type Dfc struct {
	CurrentVersionUUID *string        `json:"currentVersionUUID,omitempty"` // Deprecated: use UUID instead
	Name               *string        `json:"name"`
	UUID               string         `json:"uuid"`
	Health             *Health        `json:"health"`
	Type               DfcType        `json:"dfcType"` // Type of the DFC
	Metrics            *DfcMetrics    `json:"metrics"`
	Bridge             *DfcBridgeInfo `json:"bridge,omitempty"` // Additional info for data-bridge type
	// For 'protocol-converter' type, this array contains exactly one connection.
	//
	// For 'data-bridge' type, this array always contains exactly two connections.
	//
	// For 'custom' type, this array is empty.
	Connections []Connection `json:"connections,omitempty"` // Connection details based on DFC type
}

type DfcMetrics struct {
	AvgInputThroughputPerMinuteInMsgSec float64 `json:"avgInputThroughputPerMinuteInMsgSec"` // Messages per second, averaged over a minute
}

// Connection represents a connection to an external system and only exists within a DFC.
type Connection struct {
	Name          string  `json:"name"`
	UUID          string  `json:"uuid"`
	Health        *Health `json:"health"`
	URI           string  `json:"uri"`           // Full connection URI including scheme, host, port, and path
	LastLatencyMs float64 `json:"lastLatencyMs"` // Last reported latency in milliseconds
}

type DfcBridgeInfo struct {
	DataContract string `json:"dataContract"` // Contract defining the data format
	InputType    string `json:"inputType"`    // Type of input data
	OutputType   string `json:"outputType"`   // Type of output data
}

type Redpanda struct {
	Health                                 *Health `json:"health"`
	AvgIncomingThroughputPerMinuteInMsgSec float64 `json:"avgIncomingThroughputPerMinuteInMsgSec"` // Incoming messages per second, averaged over a minute
	AvgOutgoingThroughputPerMinuteInMsgSec float64 `json:"avgOutgoingThroughputPerMinuteInMsgSec"` // Outgoing messages per second, averaged over a minute
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

type Version struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Release struct {
	Health  *Health `json:"health"`
	Version string  `json:"version"`
	Channel string  `json:"channel"`
	// List of supported feature keywords. If a feature is supported, its corresponding keyword
	// will be included in the array.
	//
	// Note: at some point we should rethink this approach to avoid bloating the status message as
	// this list will likely grow over time.
	SupportedFeatures []string  `json:"supportedFeatures"`
	Versions          []Version `json:"versions"`
}
