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
	"reflect"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// DeltaEntry represents a single change for storage in DeltaStore.
// This is the internal representation stored in the delta table.
type DeltaEntry struct {
	Timestamp  time.Time // When the change occurred
	Changes    *Diff     // Field-level changes
	WorkerType string    // Type of worker (e.g., "container", "relay")
	ID         string    // Unique identifier for the worker
	Role       string    // "identity", "desired", or "observed"
	SyncID     int64     // Global monotonic sequence number
}

// Diff represents field-level changes between two documents.
type Diff struct {
	Added    map[string]interface{}   // New fields that were added
	Modified map[string]ModifiedField // Fields that were changed
	Removed  []string                 // Field names that were deleted
}

// ModifiedField represents a field that changed between two documents.
type ModifiedField struct {
	Old interface{} // Previous value
	New interface{} // New value
}

// IsEmpty returns true if the Diff contains no changes.
// Returns true if the receiver is nil (nil Diff is considered empty).
func (d *Diff) IsEmpty() bool {
	if d == nil {
		return true
	}

	return len(d.Added) == 0 && len(d.Modified) == 0 && len(d.Removed) == 0
}

// computeDiff computes field-level differences between two documents.
// Returns nil if documents are identical.
func computeDiff(oldDoc, newDoc persistence.Document) *Diff {
	diff := &Diff{
		Added:    make(map[string]interface{}),
		Modified: make(map[string]ModifiedField),
		Removed:  []string{},
	}

	for key, newVal := range newDoc {
		if oldVal, exists := oldDoc[key]; !exists {
			diff.Added[key] = newVal
		} else if !reflect.DeepEqual(oldVal, newVal) {
			diff.Modified[key] = ModifiedField{Old: oldVal, New: newVal}
		}
	}

	for key := range oldDoc {
		if _, exists := newDoc[key]; !exists {
			diff.Removed = append(diff.Removed, key)
		}
	}

	if diff.IsEmpty() {
		return nil
	}

	return diff
}
