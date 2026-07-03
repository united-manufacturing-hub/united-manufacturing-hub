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
	Plugins map[string]interface{} `json:"plugins"` // Extension point for future plugins
	Core    Core                   `json:"core"`
}

type Core struct {
	Agent         Agent          `json:"agent"`
	Health        *Health        `json:"health"`
	FeatureUsage  *FeatureUsage  `json:"featureUsage,omitempty"`
	Container     Container      `json:"container"`
	TopicBrowser  TopicBrowser   `json:"topicBrowser"`
	Release       Release        `json:"release"`
	Dfcs          []Dfc          `json:"dfcs"`
	Redpanda      Redpanda       `json:"redpanda"`
	DataModels    []DataModel    `json:"dataModels"`
	DataContracts []DataContract `json:"dataContracts"`
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

// String returns the string representation of a HealthCategory.
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

type DataModel struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	LatestVersion string `json:"latestVersion"`
	Hash          string `json:"hash"`
}

type DataContract struct {
	Name      string          `json:"name"`
	DataModel DataContractRef `json:"dataModel"`
	Flows     int             `json:"flows"`
}

type DataContractRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
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

// CauseKind enumerates the reason classes that can degrade CPU health.
type CauseKind string

// Cause is a single degradation reason with an associated numeric value.
type Cause struct {
	Kind  CauseKind `json:"kind"`
	Value float64   `json:"value"`
}

type CPU struct {
	Health *Health `json:"health"`
	// AvgMCpu/P95MCpu/P99MCpu are the avg/p95/p99 of the dead-zone usage ring
	// in milli-cores (signals.*UsageFraction * 1000). They are *float64 so
	// omitempty emits a real 0 (non-nil pointer) when the metric is fetchable
	// (the ring holds >= 2 entries) and omits it when un-fetchable (nil pointer:
	// outside the dead-zone, or the first dead-zone tick before the ring has 2
	// entries). Observability-only — they do not change the verdict.
	AvgMCpu       *float64 `json:"avgMCpu,omitempty"`
	P95MCpu       *float64 `json:"p95MCpu,omitempty"`
	P99MCpu       *float64 `json:"p99MCpu,omitempty"`
	ThrottleRatio *float64 `json:"throttleRatio,omitempty"` // Ratio of throttled periods (0.0-1.0); nil when cgroup unreadable
	// StealP95 is the 60s-windowed steal p95 as a fraction 0-1. It is *float64
	// so omitempty emits a real 0 (non-nil pointer) when steal is fetchable
	// (the box is virtualized) and omits it when un-fetchable (nil pointer: bare
	// metal, where steal is not a readable signal). It does not change the verdict.
	StealP95 *float64 `json:"stealP95,omitempty"`
	// PressureAvg60 is the PSI cpu.pressure some-avg60 value as a fraction 0-1
	// (the raw kernel 0-100 percentage divided by 100, same convention as
	// ThrottleRatio). It is *float64 so omitempty emits a real 0 (non-nil
	// pointer) when PSI is fetchable (the cpu.pressure file is present) and
	// omits it when un-fetchable (nil pointer: PSI absent). It does not change
	// the verdict.
	PressureAvg60 *float64 `json:"pressureAvg60,omitempty"`
	// HostBusyCores is the count of host-level busy CPU cores computed from
	// /proc/stat's non-idle jiffies. It is *float64 so omitempty emits a real 0
	// (non-nil pointer) when /proc/stat is readable (even on the baseline tick
	// where the delta is 0) and omits it when un-fetchable (nil pointer: cpu.stat
	// or /proc/stat unreadable — readProcStat runs only after the primary cpu.stat
	// read succeeds, so a cpu.stat failure also yields nil). It does not change
	// the verdict.
	HostBusyCores *float64 `json:"hostBusyCores,omitempty"`
	// State is always emitted (no omitempty), even when healthy. Attribution
	// and Causes are set only when State == "degraded".
	State          string  `json:"state"`                 // "healthy" | "degraded"
	Attribution    string  `json:"attribution,omitempty"` // "host" | "unknown" (set only when degraded)
	Causes         []Cause `json:"causes,omitempty"`      // each {kind, value} (set only when degraded)
	TotalUsageMCpu float64 `json:"totalUsageMCpu"`        // Total usage in milli-cores (1000m = 1 core)
	CoreCount      int     `json:"coreCount"`             // Number of CPU cores
	// Cgroup-specific fields for container resource limits
	CgroupCores  float64       `json:"cgroupCores,omitempty"`  // CPU quota from cgroup (e.g., 2.0 = 2 cores)
	IsThrottled  bool          `json:"isThrottled,omitempty"`  // True if recently throttled
	VerdictBasis *VerdictBasis `json:"verdictBasis,omitempty"` // The verdict's decision variables (nil when no verdict: cgroup read failure)
}

// VerdictBasis is the machine-readable why behind the CPU-health verdict: the
// exact decision variables (inputs, outputs, applicability, and thresholds)
// the verdict acted on. It is always emitted when a verdict was computed
// (healthy AND degraded), so the Management Console renders the headline, the
// host/container split, and the alert-rule budget dashboard from the same
// source the verdict used — the human-readable health.message and this block
// cannot silently diverge. It is nil only when no verdict exists (a cgroup
// read failure, where Decide is not called); the legacy display path covers
// that case.
type VerdictBasis struct {
	Headroom VerdictBasisHeadroom `json:"headroom"`
	Throttle VerdictBasisCause    `json:"throttle"`
	Pressure VerdictBasisCause    `json:"pressure"`
	Steal    VerdictBasisCause    `json:"steal"`
}

// VerdictBasisHeadroom is the primary saturation signal: how many cores of
// free capacity the box has, the inputs that produced it, and whether the
// Schmitt latch has fired. Cores is negative when the box is over-subscribed
// (hostBusyMean exceeds capacity minus the reserve); it is not clamped to 0.
type VerdictBasisHeadroom struct {
	Cores        float64 `json:"cores"`        // HeadroomCores = capacity - hostBusyMean - reserve; may be negative
	HostBusyMean float64 `json:"hostBusyMean"` // 60s mean of host-level busy cores (the verdict's saturation input)
	Capacity     float64 `json:"capacity"`     // cgroup quota if set and positive, else host logical CPU count
	Reserve      float64 `json:"reserve"`      // the CPU reserve set aside (1.0 core; TODO calibrate with fleet data)
	Fired        bool    `json:"fired"`        // SaturationFired: headroom < 0 latched, clears above the recover threshold
}

// VerdictBasisCause is one secondary starvation signal: the measured value,
// the fire threshold the latch compares against, the latch state, and whether
// the rule is applicable to this box (a rule that cannot be measured here —
// no cgroup limit, no PSI, bare metal — has applies=false).
type VerdictBasisCause struct {
	Value     float64 `json:"value"`     // the metric (ThrottleRatio / PressureAvg60Out / StealP95)
	Threshold float64 `json:"threshold"` // the Schmitt fire threshold (0.05 / 0.20 / 0.10)
	Fired     bool    `json:"fired"`     // the Schmitt latch state
	Applies   bool    `json:"applies"`   // whether this rule is live for this box
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
	DfcTypeStreamProcessor   DfcType = "stream-processor"
)

// Dfc represents a Data Flow Component.
type Dfc struct {
	CurrentVersionUUID *string        `json:"currentVersionUUID,omitempty"` // Deprecated: use UUID instead
	Name               *string        `json:"name"`
	UUID               string         `json:"uuid"`
	Health             *Health        `json:"health"`
	ReadFlowHealth     *Health        `json:"readFlowHealth,omitempty"`  // Health of the read (source) data flow, only for protocol-converter type
	WriteFlowHealth    *Health        `json:"writeFlowHealth,omitempty"` // Health of the write (sink) data flow, only for protocol-converter type
	Type               DfcType        `json:"dfcType"`                   // Type of the DFC
	Metrics            *DfcMetrics    `json:"metrics"`
	Bridge             *DfcBridgeInfo `json:"bridge,omitempty"` // Additional info for data-bridge type
	// For 'protocol-converter' type, this array contains exactly one connection.
	//
	// For 'data-bridge' type, this array always contains exactly two connections.
	//
	// For 'custom' type, this array is empty.
	Connections   []Connection `json:"connections,omitempty"` // Connection details based on DFC type
	IsInitialized bool         `json:"isInitialized"`         // Whether the DFC is initialized
}

type DfcMetrics struct {
	AvgInputThroughputPerMinuteInMsgSec  float64 `json:"avgInputThroughputPerMinuteInMsgSec"` // Messages per second, averaged over a minute
	AvgOutputThroughputPerMinuteInMsgSec float64 `json:"avgOutputThroughputPerMinuteInMsgSec"`
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
	Health                                   *Health `json:"health"`
	AvgIncomingThroughputPerMinuteInBytesSec float64 `json:"avgIncomingThroughputPerMinuteInBytesSec"` // Incoming bytes per second, averaged over a minute
	AvgOutgoingThroughputPerMinuteInBytesSec float64 `json:"avgOutgoingThroughputPerMinuteInBytesSec"` // Outgoing bytes per second, averaged over a minute
}

type TopicBrowser struct {
	Health *Health `json:"health"`
	// UnsBundles is a map because there might be the case that the topic browser service generated more than one uns bundle
	// between two runs of the status message generation. In this case, we need to send the uns bundles in the order they were generated
	// to not lose any data. The order is maintained by the index of the map.
	// The uns bundles are compressed protobuf data of protobuf type tbproto.UnsBundle.
	// also, if we send the status message to a new subscriber, we want to send the cached uns bundle first and then the new uns bundles
	UnsBundles map[int][]byte `json:"unsBundles"`
	TopicCount int            `json:"topicCount"`
}

type EventsTable struct {
	Value           interface{} `json:"value"`
	Origin          *string     `json:"origin"`
	Error           string      `json:"error"`
	UnsTreeID       string      `json:"unsTreeId"`
	Bridges         []string    `json:"bridges"`
	RawKafkaMessage EventKafka  `json:"rawKafkaMessage"`
	TimestampMS     float64     `json:"timestamp_ms"`
}

type EventKafka struct {
	Headers                map[string]string `json:"headers"`
	Key                    string            `json:"key"`
	LastPayload            string            `json:"lastPayload"`
	Topic                  string            `json:"topic"`
	Destination            []string          `json:"destination"`
	Origin                 []string          `json:"origin"`
	KafkaInsertedTimestamp float64           `json:"kafkaInsertedTimestamp"`
	MessagesPerMinute      float64           `json:"messagesPerMinute"`
}

type UnsTable struct {
	Area       *string  `json:"area,omitempty"`
	EventGroup *string  `json:"eventGroup,omitempty"`
	EventTag   *string  `json:"eventTag,omitempty"`
	IsError    *bool    `json:"isError,omitempty"`
	Line       *string  `json:"line,omitempty"`
	OriginID   *string  `json:"originId,omitempty"`
	Schema     *string  `json:"schema,omitempty"`
	Site       *string  `json:"site,omitempty"`
	WorkCell   *string  `json:"workCell,omitempty"`
	Enterprise string   `json:"enterprise"`
	Instances  []string `json:"instances,omitempty"`
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
