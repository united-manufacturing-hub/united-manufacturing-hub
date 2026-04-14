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
	"crypto/x509"
	"errors"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
)

// CertFetcherDependencies holds the cert fetcher's runtime dependencies.
type CertFetcherDependencies struct {
	*deps.BaseDependencies

	certHandler certificatehandler.Handler
	subHandler  gatekeeper.SubHandler

	lastFetchAt       time.Time
	consecutiveErrors int

	mu sync.RWMutex
}

// NewCertFetcherDependencies creates dependencies for the cert fetcher worker.
// The baseDeps parameter must be the pointer returned by WorkerBase.InitBase.
func NewCertFetcherDependencies(
	subHandler gatekeeper.SubHandler,
	certHandler certificatehandler.Handler,
	baseDeps *deps.BaseDependencies,
) (*CertFetcherDependencies, error) {
	if certHandler == nil {
		return nil, errors.New("certHandler must not be nil")
	}

	return &CertFetcherDependencies{
		BaseDependencies: baseDeps,
		subHandler:       subHandler,
		certHandler:      certHandler,
	}, nil
}

// HasSubHandler returns true if the subscriber handler is available.
func (d *CertFetcherDependencies) HasSubHandler() bool {
	d.mu.RLock()
	sh := d.subHandler
	d.mu.RUnlock()

	if sh == nil {
		return false
	}

	// Check if the lazy handler's underlying provider is ready
	lazy, ok := sh.(*lazySubHandler)
	if ok {
		return lazy.IsReady()
	}

	return true
}

// Subscribers returns the list of active subscriber emails.
func (d *CertFetcherDependencies) Subscribers() []string {
	d.mu.RLock()
	sh := d.subHandler
	d.mu.RUnlock()
	if sh == nil {
		return nil
	}
	return sh.Subscribers()
}

// FetchAllCerts iterates all active subscribers and fetches their certificates.
func (d *CertFetcherDependencies) FetchAllCerts(ctx context.Context) error {
	emails := d.Subscribers()
	if len(emails) == 0 {
		return nil
	}

	var lastErr error
	for _, email := range emails {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := d.certHandler.FetchAndStore(ctx, email)
		if err != nil {
			d.GetLogger().SentryError(deps.FeatureGatekeeper, d.GetHierarchyPath(), err, "cert_fetch_failed",
				deps.String("email", email))
			lastErr = err
		}
	}

	if lastErr != nil {
		d.RecordFetchError()
		return lastErr
	}

	d.RecordFetchSuccess()
	return nil
}

// Certificate delegates to the cert handler.
func (d *CertFetcherDependencies) Certificate(email string) *x509.Certificate {
	return d.certHandler.Certificate(email)
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
