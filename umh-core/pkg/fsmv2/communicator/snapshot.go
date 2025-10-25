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

package communicator

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

type CommunicatorDesiredState struct {
	shutdownRequested bool
}

func (d *CommunicatorDesiredState) ShutdownRequested() bool {
	return d.shutdownRequested
}

func (d *CommunicatorDesiredState) SetShutdownRequested(requested bool) {
	d.shutdownRequested = requested
}

type CommunicatorObservedState struct {
	CollectedAt       time.Time
	authenticated     bool
	jwtToken          string
	tokenExpiresAt    time.Time
	syncHealthy       bool
	consecutiveErrors int
}

func (o *CommunicatorObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o *CommunicatorObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &CommunicatorDesiredState{
		shutdownRequested: false,
	}
}

func (o *CommunicatorObservedState) SetAuthenticated(authenticated bool) {
	o.authenticated = authenticated
}

func (o *CommunicatorObservedState) IsAuthenticated() bool {
	return o.authenticated
}

func (o *CommunicatorObservedState) SetJWTToken(token string) {
	o.jwtToken = token
}

func (o *CommunicatorObservedState) GetJWTToken() string {
	return o.jwtToken
}

func (o *CommunicatorObservedState) SetTokenExpiresAt(expiresAt time.Time) {
	o.tokenExpiresAt = expiresAt
}

func (o *CommunicatorObservedState) GetTokenExpiresAt() time.Time {
	return o.tokenExpiresAt
}

func (o *CommunicatorObservedState) IsTokenExpired() bool {
	if o.tokenExpiresAt.IsZero() {
		return false
	}

	const refreshBuffer = 10 * time.Minute

	return time.Now().Add(refreshBuffer).After(o.tokenExpiresAt)
}

func (o *CommunicatorObservedState) SetSyncHealthy(healthy bool) {
	o.syncHealthy = healthy
}

func (o *CommunicatorObservedState) IsSyncHealthy() bool {
	return o.syncHealthy
}

func (o *CommunicatorObservedState) SetConsecutiveErrors(count int) {
	o.consecutiveErrors = count
}

func (o *CommunicatorObservedState) GetConsecutiveErrors() int {
	return o.consecutiveErrors
}
