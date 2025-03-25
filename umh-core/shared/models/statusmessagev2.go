package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/healthstatus"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/models/ctypes"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/models/generated"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/models/mgmtconfig"
)

type StatusMessageV2 struct {
	Instance     Instance                                              `json:"instance"`
	InstanceData generated.StatusMessageInstanceSchemaJsonInstanceData `json:"instanceData"`
	Data         Data                                                  `json:"data"`
	Kubernetes   Kubernetes                                            `json:"kubernetes"`
	Network      Network                                               `json:"network"`
	// Deprecated: Use `UnsTable` & `EventTable` instead
	UnifiedNamespace   UnifiedNamespaceV2                                                   `json:"unifiedNamespace"`
	UnsTable           UnsTable                                                             `json:"unsTable"`
	EventTable         EventTable                                                           `json:"eventTable"`
	Module             Module                                                               `json:"module"`
	Version            int                                                                  `json:"version"`
	Secrets            Secrets                                                              `json:"secrets"`
	DataFlowComponents generated.StatusMessageDataflowcomponentSchemaJsonDataFlowComponents `json:"dataFlowComponents"`
	Cryptography       Cryptography                                                         `json:"cryptography"`
}

// General Health Type
type Health struct {
	GoodHealth bool                      `json:"goodHealth"`
	Status     healthstatus.HealthStatus `json:"status"`
	Tooltip    string                    `json:"tooltip"`
}

// General Types
type OSInfo struct {
	KernelVersion   string `json:"kernelVersion"`
	OperatingSystem string `json:"operatingSystem"`
	Architecture    string `json:"architecture"`
}

type EthernetAdapter struct {
	Name    string   `json:"name"`
	IP4     []string `json:"ip4"`
	IP6     []string `json:"ip6"`
	Gateway string   `json:"gateway"`
	DNS     []string `json:"dns"`
	MAC     string   `json:"mac"`
}

// Health Types
type ResourceUtilization struct {
	CpuPercent  float64 `json:"cpuPercent"`
	CpuMax      string  `json:"cpuMax"`
	RamPercent  float64 `json:"ramPercent"`
	RamMax      string  `json:"ramMax"`
	DiskPercent float64 `json:"diskPercent"`
	DiskMax     string  `json:"diskMax"`
}

type KubernetesEvent struct {
	Event   string `json:"event"`
	Count   int    `json:"count"`
	Message string `json:"message"`
}

type DataSourceMetric struct { // Deprecated
	Name           string    `json:"name"`
	ConnectionUUID uuid.UUID `json:"connectionUUID"`
	UUID           uuid.UUID `json:"uuid"`
	RatePerSecond  float64   `json:"ratePerSecond"`
	Health         Health    `json:"health"`
}

type ProtocolConverterMetric struct { // Deprecated
	Name           string          `json:"name"`
	Type           ctypes.Protocol `json:"type"`
	ConnectionUUID uuid.UUID       `json:"connectionUUID"`
	UUID           uuid.UUID       `json:"uuid"`
	RatePerSecond  float64         `json:"ratePerSecond"`
	Health         Health          `json:"health"`
}

type KubernetesHealth struct {
	Health          Health            `json:"health"`
	ErrorEvents     []KubernetesEvent `json:"errorEvents"`
	CompanionUptime float64           `json:"companionUptime"`
	UMHUptime       float64           `json:"umhUptime"`
}

type ModuleHealth struct {
	Name   string `json:"name"`
	Health Health `json:"health"`
}

type ConnectionHealth struct {
	Name           string                          `json:"name"`
	UUID           uuid.UUID                       `json:"uuid"`
	IsInitialized  bool                            `json:"isInitialized"`
	URI            string                          `json:"uri"`
	ConnectionType ctypes.DatasourceConnectionType `json:"connectionType"`
	LatencyMs      float64                         `json:"latencyMs"`
	Health         Health                          `json:"health"`
	Location       *mgmtconfig.ConnectionLocation  `json:"location"`
}

type DataHealth struct {
	KafkaRate               int                       `json:"kafkaRate"`
	TimescaleDBRate         int                       `json:"timescaleDBRate"`
	DataSourceHealth        []DataSourceMetric        `json:"dataSourceHealth"`
	ProtocolConverterHealth []ProtocolConverterMetric `json:"protocolConverterHealth"`
	Health                  Health                    `json:"health"`
}

type InstanceHealth struct {
	Health              Health              `json:"health"`
	ResourceUtilization ResourceUtilization `json:"resourceUtilization"`
	DeviceUptime        float64             `json:"deviceUptime"`
}

type InstanceLocation struct {
	Enterprise string `json:"enterprise"`
	Site       string `json:"site"`
	Area       string `json:"area"`
	Line       string `json:"line"`
	WorkCell   string `json:"workCell"`
}

type InstallationScope struct {
	Enabled    bool             `json:"enabled"`
	MQTTBroker MqttBrokerStatus `json:"mqttBroker"`
}

type MqttBrokerStatus struct {
	Ip          string    `json:"ip"`
	Port        uint32    `json:"port"`
	Username    string    `json:"username"`
	Password    string    `json:"password"`
	LastUpdated uint64    `json:"lastUpdated"`
	UUID        uuid.UUID `json:"uuid"`
}

type Instance struct {
	General           OSInfo            `json:"general"` // Deprecated
	Health            InstanceHealth    `json:"health"`  // Deprecated
	Location          InstanceLocation  `json:"location"`
	InstallationScope InstallationScope `json:"installationScope"` // Deprecated
	Latency           Latency           `json:"latency"`           // Deprecated
	// LatencyExtended is a map of DNS, TLS, and other latencies
	// It is not meant to be parsed by the UI, but is used for debugging purposes
	// It is therefore also not required to be sent by the simulator
	LatencyExtended map[string]Latency `json:"latencyExtended"` // Deprecated
}

type Latency struct {
	Min time.Duration `json:"min"`
	Max time.Duration `json:"max"`
	P95 time.Duration `json:"p95"`
	P99 time.Duration `json:"p99"`
	Avg time.Duration `json:"avg"`
}

type KubernetesGeneral struct {
	ManagementCompanionVersion string `json:"managementCompanionVersion"`
	UMHVersion                 string `json:"umhVersion"`
}

type Kubernetes struct {
	General KubernetesGeneral `json:"general"`
	Health  KubernetesHealth  `json:"health"`
}

type Module struct {
	General struct{}       `json:"general"`
	Health  []ModuleHealth `json:"health"`
}

type EthernetAdaptersGeneral struct {
	EthernetAdapters []EthernetAdapter `json:"ethernetAdapters"`
}

type ConnectionHealthInfo struct {
	Connections []ConnectionHealth `json:"connections"`
}

type Network struct {
	General EthernetAdaptersGeneral `json:"general"`
	Health  ConnectionHealthInfo    `json:"health"`
}

type Data struct {
	General struct{}   `json:"general"`
	Health  DataHealth `json:"health"`
}

type Secrets map[string]string

type Cryptography struct {
	Version uint64 `json:"version"` // If version is 0, then no encrypted connection can be established (for example because the instance was created without a certificate & private key)
	// VersionMetaInformation currently can be CryptographyVersion1MetaInformation
	VersionMetaInformation interface{} `json:"versionMetaInformation"`
}

type CryptographyVersion1MetaInformation struct {
	Certificate string `json:"certificate"`
}
