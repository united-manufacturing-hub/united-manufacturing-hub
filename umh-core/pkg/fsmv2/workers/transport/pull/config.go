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

package pull

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// PullConfig holds the user-provided configuration for the pull worker.
// PullWorker config comes from parent TransportWorker, so this is minimal.
type PullConfig struct {
	config.BaseUserSpec `yaml:",inline"`
}

// PullStatus holds the runtime observation data for the pull worker.
type PullStatus struct {
	DegradedEnteredAt time.Time `json:"degraded_entered_at,omitempty"`
	LastErrorAt       time.Time `json:"last_error_at,omitempty"`

	LastRetryAfter time.Duration `json:"last_retry_after,omitempty"`

	LastErrorType       httpTransport.ErrorType `json:"last_error_type"`
	ConsecutiveErrors   int                     `json:"consecutive_errors"`
	PendingMessageCount int                     `json:"pending_message_count"`

	HasTransport    bool `json:"has_transport"`
	HasValidToken   bool `json:"has_valid_token"`
	IsBackpressured bool `json:"is_backpressured"`
}
