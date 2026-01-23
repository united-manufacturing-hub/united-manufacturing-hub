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
	// Pointer (8 bytes)
	Changes *Diff
	// Strings (16 bytes each) - ordered first by size
	WorkerID   string
	WorkerType string
	Collection string
	Role       string
	EventType  string
	// Int64s (8 bytes each)
	SyncID      int64
	TimestampMs int64
	// Bool (1 byte)
	HasChanges bool
}

type EventSnapshot struct {
	// Map (8 bytes - pointer to map header)
	Document persistence.Document
	// Strings (16 bytes each) - ordered first by size
	WorkerID   string
	WorkerType string
	Collection string
	Role       string
	// Int64s (8 bytes each)
	ID          int64
	AtSyncID    int64
	TimestampMs int64
	// Int (8 bytes on 64-bit)
	EventCount int
}
