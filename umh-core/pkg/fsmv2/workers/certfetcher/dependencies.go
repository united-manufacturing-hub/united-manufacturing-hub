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

package certfetcher

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
)

// CertFetcherDependencies holds the cert fetcher's runtime dependencies.
type CertFetcherDependencies struct {
	*deps.BaseDependencies

	certHandler certificatehandler.Handler

	lastFetchAt       time.Time
	consecutiveErrors int

	mu sync.RWMutex
}

// NewCertFetcherDependencies creates dependencies for the cert fetcher worker.
func NewCertFetcherDependencies(
	certHandler certificatehandler.Handler,
	baseDeps *deps.BaseDependencies,
) (*CertFetcherDependencies, error) {
	if certHandler == nil {
		return nil, errors.New("certHandler must not be nil")
	}

	return &CertFetcherDependencies{
		BaseDependencies: baseDeps,
		certHandler:      certHandler,
	}, nil
}

// HasSubHandler delegates to cert handler.
// CertHandler returns the certificate handler.
func (d *CertFetcherDependencies) CertHandler() certificatehandler.Handler {
	return d.certHandler
}

// FetchAllCerts delegates to cert handler, then tracks success/failure.
func (d *CertFetcherDependencies) FetchAllCerts(ctx context.Context) error {
	err := d.certHandler.FetchAllCerts(ctx)
	if err != nil {
		d.RecordFetchError()
		return err
	}
	d.RecordFetchSuccess()
	return nil
}

// RecordFetchSuccess records a successful fetch cycle.
func (d *CertFetcherDependencies) RecordFetchSuccess() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastFetchAt = time.Now()
	d.consecutiveErrors = 0
}

// RecordFetchError increments the consecutive error count.
func (d *CertFetcherDependencies) RecordFetchError() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.consecutiveErrors++
}

// LastFetchAt returns the time of the last successful fetch.
func (d *CertFetcherDependencies) LastFetchAt() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastFetchAt
}

// ConsecutiveErrors returns the current consecutive error count.
func (d *CertFetcherDependencies) ConsecutiveErrors() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.consecutiveErrors
}
