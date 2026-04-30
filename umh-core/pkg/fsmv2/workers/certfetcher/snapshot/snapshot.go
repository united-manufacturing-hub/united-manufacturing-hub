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

// Package snapshot defines the observed and desired state types for the certfetcher worker.
package snapshot

import (
	"context"
	"crypto/x509"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// CertFetcherDeps defines the dependency interface for the cert fetcher worker.
type CertFetcherDeps interface {
	deps.Dependencies
	Subscribers() []string
	FetchAllCerts(ctx context.Context) error
	Certificate(email string) *x509.Certificate
	HasSubHandler() bool
}

// CertFetcherDesiredState represents the target configuration.
type CertFetcherDesiredState struct {
	config.BaseDesiredState
}

// CertFetcherObservedState represents the current state snapshot.
type CertFetcherObservedState struct {
	CollectedAt time.Time `json:"collected_at"`
	LastFetchAt time.Time `json:"last_fetch_at,omitempty"`

	CertFetcherDesiredState `json:",inline"`

	State string `json:"state"`

	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

	deps.MetricsEmbedder `json:",inline"`

	ConsecutiveErrors int  `json:"consecutive_errors"`
	SubscriberCount   int  `json:"subscriber_count"`
	CachedCertCount   int  `json:"cached_cert_count"`
	HasSubHandler     bool `json:"has_sub_handler"`
}

// GetTimestamp returns when this snapshot was collected.
func (o CertFetcherObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// GetObservedDesiredState returns the embedded desired state.
func (o CertFetcherObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.CertFetcherDesiredState
}

// SetState sets the current FSM state name.
func (o CertFetcherObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s
	return o
}

// SetShutdownRequested sets the shutdown flag.
func (o CertFetcherObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v
	return o
}

// SetParentMappedState is a no-op for standalone workers.
func (o CertFetcherObservedState) SetParentMappedState(_ string) fsmv2.ObservedState {
	return o
}
