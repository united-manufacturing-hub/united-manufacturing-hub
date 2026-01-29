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

package sentry

// ErrorFields provides typed logging fields for fsmv2 error logging.
// This struct enforces type safety at compile time - the Err field is `error`,
// not `string`, which prevents the common mistake of passing err.Error().
type ErrorFields struct {
	Feature       string // Required: who owns this feature?
	Err           error  // The error (type-safe: `error`, not `string`)
	HierarchyPath string // Optional: the hierarchy path for FSM version detection
	WorkerID      string // Optional: specific worker identifier
	WorkerType    string // Optional: type of worker
}

// ZapFields converts ErrorFields to a slice of interface{} suitable for zap structured logging.
// Usage: logger.Errorw("event_name", fields.ZapFields()...)
func (f ErrorFields) ZapFields() []interface{} {
	fields := []interface{}{
		"feature", f.Feature,
	}

	if f.Err != nil {
		fields = append(fields, "error", f.Err)
	}

	if f.HierarchyPath != "" {
		fields = append(fields, "hierarchy_path", f.HierarchyPath)
	}

	if f.WorkerID != "" {
		fields = append(fields, "worker_id", f.WorkerID)
	}

	if f.WorkerType != "" {
		fields = append(fields, "worker_type", f.WorkerType)
	}

	return fields
}
