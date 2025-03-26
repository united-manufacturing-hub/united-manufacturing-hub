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

// This file was generated from JSON Schema using quicktype, do not modify it directly.
// To parse and unparse this JSON data, add this code to your project and do:
//
//    statusMessage, err := UnmarshalStatusMessage(bytes)
//    bytes, err = statusMessage.Marshal()

package generated

import "encoding/json"

func UnmarshalStatusMessage(data []byte) (StatusMessage, error) {
	var r StatusMessage
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *StatusMessage) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

// Schema for the status message containing system state and DFC information
type StatusMessage struct {
	Core    Core                   `json:"core"`
	General General                `json:"general"`
	Plugins map[string]interface{} `json:"plugins"`
}

type Core struct {
	Agent     Agent     `json:"agent"`
	Container Container `json:"container"`
	// List of deployed DFCs. Different DFC types can have different properties
	Dfcs           []Dfc                  `json:"dfcs"`
	EventsTable    map[string]EventsTable `json:"eventsTable,omitempty"`
	Latency        Latency                `json:"latency"`
	Redpanda       Redpanda               `json:"redpanda"`
	ReleaseChannel string                 `json:"releaseChannel"`
	// List of supported feature keywords. If a feature is supported, its corresponding keyword
	// will be included in the array.
	SupportedFeatures []string            `json:"supportedFeatures"`
	UnsTable          map[string]UnsTable `json:"unsTable,omitempty"`
	Version           string              `json:"version"`
}

type Agent struct {
	Health Health `json:"health"`
}

type Health struct {
	Message string `json:"message"`
	State   string `json:"state"`
}

type Container struct {
	// Processor architecture. Examples: 'arm/v7', 'armv7', 'armhf', 'x86_64', etc.
	Architecture string `json:"architecture"`
	CPU          CPU    `json:"cpu"`
	Disk         Disk   `json:"disk"`
	Hwid         string `json:"hwid"`
	Memory       Memory `json:"memory"`
}

type CPU struct {
	Health Health `json:"health"`
	// CPU limit in number of cores
	Limit float64 `json:"limit"`
	// CPU usage as a percentage (0-100%)
	Usage float64 `json:"usage"`
}

type Disk struct {
	Health Health `json:"health"`
	// Disk limit in bytes
	Limit float64 `json:"limit"`
	// Disk usage in bytes
	Usage float64 `json:"usage"`
}

type Memory struct {
	Health Health `json:"health"`
	// Memory limit in bytes
	Limit float64 `json:"limit"`
	// Memory usage in bytes
	Usage float64 `json:"usage"`
}

type Dfc struct {
	CurrentVersionUUID *string     `json:"currentVersionUUID"`
	DeploySuccess      bool        `json:"deploySuccess"`
	DfcType            DfcType     `json:"dfcType"`
	Health             *Health     `json:"health"`
	Metrics            *DFCMetrics `json:"metrics"`
	Name               *string     `json:"name"`
	UUID               string      `json:"uuid"`
	// For 'protocol-converter' type, this array contains exactly one connection.
	//
	// For 'data-bridge' type, this array always contains exactly two connections.
	Connections  []Connection `json:"connections,omitempty"`
	DataContract *string      `json:"dataContract,omitempty"`
	InputType    *string      `json:"inputType,omitempty"`
	IsReadOnly   *bool        `json:"isReadOnly,omitempty"`
	OutputType   *string      `json:"outputType,omitempty"`
}

type Connection struct {
	Health Health `json:"health"`
	// Latency in milliseconds
	Latency float64 `json:"latency"`
	Name    string  `json:"name"`
	// The connection URI in full, e.g., 'opc.tcp://hostname:port/path'. This includes the
	// scheme, host, port, and any required path elements.
	URI  string `json:"uri"`
	UUID string `json:"uuid"`
}

type DFCMetrics struct {
	FailedMessages float64 `json:"failedMessages"`
	// Throughput expressed in messages per second
	ThroughputMsgPerSEC float64 `json:"throughputMsgPerSec"`
	Unprocessable       float64 `json:"unprocessable"`
	Unprocessable24H    float64 `json:"unprocessable24h"`
}

type EventsTable struct {
	Bridges         []string   `json:"bridges"`
	Error           string     `json:"error"`
	Origin          *string    `json:"origin"`
	RawKafkaMessage EventKafka `json:"rawKafkaMessage"`
	// Timestamp in milliseconds
	TimestampMS float64     `json:"timestamp_ms"`
	UnsTreeID   string      `json:"unsTreeId"`
	Value       interface{} `json:"value"`
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

type Latency struct {
	// Average latency in milliseconds
	Avg float64 `json:"avg"`
	// Maximum latency in milliseconds
	Max float64 `json:"max"`
	// Minimum latency in milliseconds
	Min float64 `json:"min"`
	// 95th percentile latency in milliseconds
	P95 float64 `json:"p95"`
	// 99th percentile latency in milliseconds
	P99 float64 `json:"p99"`
}

type Redpanda struct {
	Health Health `json:"health"`
	// Incoming throughput in messages per second
	ThroughputIncomingMsgPerSEC float64 `json:"throughputIncomingMsgPerSec"`
	// Outgoing throughput in messages per second
	ThroughputOutgoingMsgPerSEC float64 `json:"throughputOutgoingMsgPerSec"`
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

type General struct {
	Location map[string]string `json:"location"`
}

type DfcType string

const (
	Custom            DfcType = "custom"
	DataBridge        DfcType = "data-bridge"
	ProtocolConverter DfcType = "protocol-converter"
	Uninitialized     DfcType = "uninitialized"
)
