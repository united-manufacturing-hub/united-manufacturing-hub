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

package fsmv2nmap

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
)

// Operational states the nmap manager reports through GetCurrentFSMState. The
// connection FSM and the communicator read them to decide whether a device is
// reachable. The port states follow nmap's vocabulary
// (https://nmap.org/book/man-port-scanning-basics.html).
const (
	// OperationalStateStopped is reported for a disabled nmap worker.
	OperationalStateStopped = "stopped"
	// OperationalStateStarting is reported before the first scan result.
	OperationalStateStarting = "starting"
	// OperationalStateDegraded is reported when scans fail or go stale.
	OperationalStateDegraded = "degraded"

	OperationalStateOpen           = "open"
	OperationalStateClosed         = "closed"
	OperationalStateFiltered       = "filtered"
	OperationalStateUnfiltered     = "unfiltered"
	OperationalStateOpenFiltered   = "open_filtered"
	OperationalStateClosedFiltered = "closed_filtered"
)

// IsRunningState reports whether state means the worker is scanning: any port
// state, or degraded.
func IsRunningState(state string) bool {
	switch state {
	case OperationalStateOpen,
		OperationalStateFiltered,
		OperationalStateClosed,
		OperationalStateUnfiltered,
		OperationalStateClosedFiltered,
		OperationalStateOpenFiltered,
		OperationalStateDegraded:
		return true
	}

	return false
}

// PortState is the state of the scanned port as stored in PortResult.State.
type PortState string

const (
	PortStateOpen           PortState = "open"
	PortStateFiltered       PortState = "filtered"
	PortStateClosed         PortState = "closed"
	PortStateUnfiltered     PortState = "unfiltered"
	PortStateOpenFiltered   PortState = "open|filtered"
	PortStateClosedFiltered PortState = "closed|filtered"
)

// DialFailure classifies why a scan's TCP dial failed.
type DialFailure string

const (
	DialFailureDNSTimeout     DialFailure = "dns_timeout"
	DialFailureDNSError       DialFailure = "dns_error"
	DialFailureConnectTimeout DialFailure = "connect_timeout"
	DialFailureRefused        DialFailure = "refused"
	DialFailureUnreachable    DialFailure = "unreachable"
	DialFailureUnknown        DialFailure = "unknown"
)

// NmapObservedState is the observed state the nmap manager returns from
// GetLastObservedState.
type NmapObservedState struct {
	// ObservedNmapServiceConfig is the target and port the last scan dialed.
	ObservedNmapServiceConfig nmapserviceconfig.NmapServiceConfig
	ServiceInfo               ServiceInfo
}

// IsObservedState implements fsm.ObservedState.
func (n NmapObservedState) IsObservedState() {}

// ServiceInfo wraps the scan status of one nmap worker.
type ServiceInfo struct {
	NmapStatus NmapServiceInfo
}

// NmapServiceInfo is the scan status of one nmap worker.
type NmapServiceInfo struct {
	// LastScan is the result of the most recent scan.
	LastScan *NmapScanResult
	// IsRunning is true when the target port accepted the connection.
	IsRunning bool
}

// NmapScanResult is the result of one scan.
type NmapScanResult struct {
	// Timestamp is when the scan started.
	Timestamp time.Time `json:"timestamp"`
	// Target is the host the scan dialed.
	Target     string     `json:"target"`
	PortResult PortResult `json:"portResult"`
	// DialFailure is nil when the port is open.
	DialFailure *DialFailure `json:"dialFailure,omitempty"`
	// DialError is the raw dial error, empty when the port is open.
	DialError string `json:"dialError,omitempty"`
}

// PortResult is the scanned port and its state.
type PortResult struct {
	// State is one of the PortState values.
	State string `json:"state"`
	// LatencyMs is the dial round-trip time in milliseconds.
	LatencyMs float64 `json:"latencyMs"`
	Port      uint16  `json:"port"`
}
