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

package state

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

const (
	businessHoursStart  = 7
	businessHoursEnd    = 20
	proactiveReauthHour = 3
)

// RunningState represents the operational state where all children are healthy.
// The transport worker has a valid JWT and all child workers (Push/Pull) are running.
type RunningState struct {
	helpers.RunningHealthyBase
}

// Next evaluates the current snapshot and returns the next state or action.
func (s *RunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[transport_pkg.TransportConfig, transport_pkg.TransportStatus](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Transition(&StoppingState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to Stopping", nil)
	}

	// If token is expired, need to re-authenticate
	if snap.Status.IsTokenExpired() {
		return fsmv2.Transition(&StartingState{}, fsmv2.SignalNone, nil, "Token expired, transitioning to Starting for re-authentication", nil)
	}

	// Proactive night re-auth: if token would expire during business hours, re-auth at 3 AM
	if ShouldProactivelyReauth(snap.Status.JWTExpiry, time.Now()) {
		return fsmv2.Transition(&StartingState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("proactive night re-auth: token expires at %s (business hours), re-authing now",
				snap.Status.JWTExpiry.Local().Format("15:04")), nil)
	}

	// If any children are unhealthy, transition to degraded
	if snap.ChildrenUnhealthy > 0 {
		return fsmv2.Transition(&DegradedState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("children unhealthy (%d), transitioning to Degraded", snap.ChildrenUnhealthy), nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "All children healthy, transport running", nil)
}

// ShouldProactivelyReauth returns true if the token would expire during business
// hours, within 24 hours, and it's currently the proactive re-auth hour (3 AM).
// The 24-hour proximity check prevents unnecessary nightly re-auth for tokens
// that expire weeks away.
func ShouldProactivelyReauth(expiry time.Time, now time.Time) bool {
	if expiry.IsZero() {
		return false
	}

	// Only consider tokens expiring within 24 hours
	delta := expiry.Sub(now)
	if delta <= 0 || delta > 24*time.Hour {
		return false
	}

	expiryHour := expiry.Local().Hour()
	if expiryHour < businessHoursStart || expiryHour >= businessHoursEnd {
		return false
	}

	return now.Local().Hour() == proactiveReauthHour
}

// String returns the state name derived from the type.
func (s *RunningState) String() string {
	return helpers.DeriveStateName(s)
}
