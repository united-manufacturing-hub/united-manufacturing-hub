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

package nmap

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

const (
	// ScanIntervalSeconds defines the fixed interval between scans.
	ScanIntervalSeconds = 1
)

// INmapService defines the interface for managing nmap scan services.
type INmapService interface {
	// GenerateS6ConfigForNmap generates a S6 config for a given nmap instance
	GenerateS6ConfigForNmap(nmapConfig *nmapserviceconfig.NmapServiceConfig, s6ServiceName string) (s6serviceconfig.S6ServiceConfig, error)
	// GetConfig returns the actual nmap config from the S6 service
	GetConfig(ctx context.Context, filesystemService filesystem.Service, nmapName string) (nmapserviceconfig.NmapServiceConfig, error)
	// Status checks the status of a nmap service
	Status(ctx context.Context, filesystemService filesystem.Service, nmapName string, tick uint64) (ServiceInfo, error)
	// AddNmapToS6Manager adds a nmap instance to the S6 manager
	AddNmapToS6Manager(ctx context.Context, cfg *nmapserviceconfig.NmapServiceConfig, nmapName string) error
	// UpdateNmapInS6Manager updates an existing nmap instance in the S6 manager
	UpdateNmapInS6Manager(ctx context.Context, cfg *nmapserviceconfig.NmapServiceConfig, nmapName string) error
	// RemoveNmapFromS6Manager removes a nmap instance from the S6 manager
	RemoveNmapFromS6Manager(ctx context.Context, nmapName string) error
	// korceRemoveNmap removes a Nmap instance from the s6 manager
	ForceRemoveNmap(ctx context.Context, filesystemService filesystem.Service, nmapName string) error
	// StartNmap starts a nmap instance
	StartNmap(ctx context.Context, nmapName string) error
	// StopNmap stops a nmap instance
	StopNmap(ctx context.Context, nmapName string) error
	// ServiceExists checks if a nmap service exists
	ServiceExists(ctx context.Context, filesystemService filesystem.Service, nmapName string) bool
	// ReconcileManager reconciles the nmap manager
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool)
}

// NmapScanResult contains the results of an nmap scan.
type NmapScanResult struct {
	// Timestamp of the scan
	Timestamp time.Time `json:"timestamp"`
	// Raw output from nmap
	RawOutput string `json:"rawOutput"`
	// Error message if scan failed
	Error string `json:"error,omitempty"`
	// Port result
	PortResult PortResult `json:"portResult"`
	// Overall scan metrics
	Metrics ScanMetrics `json:"metrics"`
}

// PortResult contains the result for a specific port.
type PortResult struct {
	// State (open/closed/filtered)
	State string `json:"state"`
	// Latency in milliseconds
	LatencyMs float64 `json:"latencyMs"`
	// Port number
	Port uint16 `json:"port"`
}

// ScanMetrics contains overall metrics for the scan.
type ScanMetrics struct {
	// Total duration of scan in seconds
	ScanDuration float64 `json:"scanDuration"`
}

// ServiceInfo contains information about an nmap service.
type ServiceInfo struct {
	// S6FSMState contains the current state of the S6 FSM
	S6FSMState string
	// NmapStatus contains information about the status of the nmap service
	NmapStatus NmapServiceInfo
	// S6ObservedState contains information about the S6 service
	S6ObservedState s6fsm.S6ObservedState
}

// NmapServiceInfo contains status information about the nmap service.
type NmapServiceInfo struct {
	// LastScan contains the result of the last scan
	LastScan *NmapScanResult
	// Logs contains the structured s6 log entries emitted by the
	// nmap service.
	//
	// **Performance consideration**
	//
	//   • Logs can grow quickly, and profiling shows that naïvely deep-copying
	//     this slice dominates CPU time (see https://flamegraph.com/share/592a6a59-25d1-11f0-86bc-aa320ab09ef2).
	//
	//   • The FSM needs read-only access to the logs for historical snapshots;
	//     it never mutates them.
	//
	//   • The s6 layer *always* allocates a brand-new slice when it returns
	//     logs (see DefaultService.GetLogs), so sharing the slice's backing
	//     array across snapshots cannot introduce data races.
	//
	// Therefore we override the default behaviour and copy only the 3-word
	// slice header (24 B on amd64) — see CopyLogs below.
	Logs []s6service.LogEntry
	// IsRunning indicates whether the nmap service is running
	IsRunning bool
}

// CopyLogs is a go-deepcopy override for the Logs field.
//
// go-deepcopy looks for a method with the signature
//
//	func (dst *T) Copy<FieldName>(src <FieldType>) error
//
// and, if present, calls it instead of performing its generic deep-copy logic.
// By assigning the slice directly we make a **shallow copy**: the header is
// duplicated but the underlying backing array is shared.
//
// Why this is safe:
//
//  1. The s6 service returns a fresh []LogEntry on every call, never reusing
//     or mutating a previously returned slice.
//  2. Logs is treated as immutable after the snapshot is taken.
//
// If either assumption changes, delete this method to fall back to the default
// deep-copy (O(n) but safe for mutable slices).
//
// See also: https://github.com/tiendc/go-deepcopy?tab=readme-ov-file#copy-struct-fields-via-struct-methods
func (nsi *NmapServiceInfo) CopyLogs(src []s6service.LogEntry) error {
	nsi.Logs = src

	return nil
}
