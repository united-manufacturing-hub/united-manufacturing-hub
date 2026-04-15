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

// Package snapshot defines shared types for the certfetcher worker.
package snapshot

import (
	"context"
	"crypto/x509"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// CertFetcherDeps defines the dependency interface for the cert fetcher action.
// Lives in snapshot package to break import cycle: action → snapshot, certfetcher → action.
type CertFetcherDeps interface {
	deps.Dependencies
	Subscribers() []string
	FetchAllCerts(ctx context.Context) error
	Certificate(email string) *x509.Certificate
	HasSubHandler() bool
}
