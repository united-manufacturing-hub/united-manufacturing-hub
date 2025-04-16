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

package dataflowcomponent

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/snapshot"
)

// Reconcile examines the DataflowComponentInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (d *DataflowComponentInstance) Reconcile(ctx context.Context, currentSnapshot snapshot.SystemSnapshot, filesystemService filesystem.Service) (err error, reconciled bool) {
	sentry.ReportFSMFatal(d.baseFSMInstance.GetLogger(), d.baseFSMInstance.GetID(), "DataflowComponentInstance", "Reconcile", fmt.Errorf("not implemented"))
	panic("not implemented")
}
