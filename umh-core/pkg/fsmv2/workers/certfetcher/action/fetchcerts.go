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

// Package action provides idempotent actions for the certfetcher worker.
package action

import (
	"context"
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/certfetcher/snapshot"
)

// FetchCertsAction fetches certificates for all active subscribers.
type FetchCertsAction struct{}

// Execute fetches certificates for all active subscribers.
func (a *FetchCertsAction) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	d, ok := depsAny.(snapshot.CertFetcherDeps)
	if !ok {
		return errors.New("invalid dependencies type: expected CertFetcherDeps")
	}

	return d.FetchAllCerts(ctx)
}

// Name returns the action name for logging.
func (a *FetchCertsAction) Name() string { return "fetch_certs" }

// String returns the action name.
func (a *FetchCertsAction) String() string { return a.Name() }
