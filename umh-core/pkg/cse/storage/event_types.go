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

package storage

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

const (
	EventTypeObservation    = "observation"
	EventTypeDesiredChange  = "desired_change"
	EventTypeIdentityCreate = "identity_create"
)

type Event struct {
	Changes     *Diff
	WorkerID    string
	WorkerType  string
	Collection  string
	Role        string
	EventType   string
	SyncID      int64
	TimestampMs int64
	HasChanges  bool
}

type EventSnapshot struct {
	Document    persistence.Document
	WorkerID    string
	WorkerType  string
	Collection  string
	Role        string
	ID          int64
	AtSyncID    int64
	EventCount  int
	TimestampMs int64
}
