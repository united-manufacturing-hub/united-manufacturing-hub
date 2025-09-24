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

package http

import (
	"net"
	"time"

	"github.com/united-manufacturing-hub/expiremap/v2/pkg/expiremap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/latency"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// LatestExternalIp is the latest external IP address
// Our backend server is configured to set a header containing the client's external IP address
// Note: This is best effort, and may not be accurate if the client is behind a proxy.
var LatestExternalIp net.IP

var latenciesFRB = expiremap.NewEx[time.Time, time.Duration](5*time.Minute, 5*time.Minute)
var latenciesDNS = expiremap.NewEx[time.Time, time.Duration](5*time.Minute, 5*time.Minute)
var latenciesTLS = expiremap.NewEx[time.Time, time.Duration](5*time.Minute, 5*time.Minute)
var latenciesConn = expiremap.NewEx[time.Time, time.Duration](5*time.Minute, 5*time.Minute)
var latenciesXResponseTimeHeader = expiremap.NewEx[time.Time, time.Duration](5*time.Minute, 5*time.Minute)
var latenciesReal = expiremap.NewEx[time.Time, time.Duration](5*time.Minute, 5*time.Minute)

// GetLatencyTimeTillFirstByte returns the latency metrics for time until first response byte.
func GetLatencyTimeTillFirstByte() models.Latency {
	return latency.CalculateLatency(latenciesFRB)
}

// GetLatencyTimeTillDNS returns the latency metrics for DNS resolution time.
func GetLatencyTimeTillDNS() models.Latency {
	return latency.CalculateLatency(latenciesDNS)
}

// GetLatencyTimeTillTLS returns the latency metrics for TLS handshake time.
func GetLatencyTimeTillTLS() models.Latency {
	return latency.CalculateLatency(latenciesTLS)
}

// GetLatencyTimeTillConn returns the latency metrics for connection establishment time.
func GetLatencyTimeTillConn() models.Latency {
	return latency.CalculateLatency(latenciesConn)
}

// GetLatencyTimeXResponseTimeHeader returns the latency metrics from X-Response-Time header values.
func GetLatencyTimeXResponseTimeHeader() models.Latency {
	return latency.CalculateLatency(latenciesXResponseTimeHeader)
}

// GetRealLatency returns the latency metrics for actual end-to-end request time.
func GetRealLatency() models.Latency {
	return latency.CalculateLatency(latenciesReal)
}
