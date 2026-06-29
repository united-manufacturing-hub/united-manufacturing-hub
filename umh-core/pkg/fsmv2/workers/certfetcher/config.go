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

import "time"

// CertFetcherConfig holds user-provided configuration for the cert fetcher worker.
// Empty because certfetcher has no user-configurable fields.
type CertFetcherConfig struct{}

// CertFetcherStatus holds runtime observation data for the cert fetcher worker.
type CertFetcherStatus struct {
	LastFetchAt time.Time `json:"last_fetch_at,omitempty"`

	ConsecutiveErrors int  `json:"consecutive_errors"`
	SubscriberCount   int  `json:"subscriber_count"`
	CachedCertCount   int  `json:"cached_cert_count"`
	HasSubHandler     bool `json:"has_sub_handler"`
}
