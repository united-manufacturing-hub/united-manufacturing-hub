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

// Package telemetry declares every Sentry event umh-core can report. The set
// lives in telemetry.yaml and reaches Go through identifiers.gen.go.
package telemetry

// Severity is an event's log level, declared once per event in telemetry.yaml.
type Severity string

const (
	// SeverityWarning reports at WARN level.
	SeverityWarning Severity = "warning"
	// SeverityError reports at ERROR level.
	SeverityError Severity = "error"
)

// Identifier is one declared event.
type Identifier struct {
	// Tag is the hierarchical event name, and the Sentry event_name tag.
	Tag string
	// Brief is one sentence on what the event means, shown under the Sentry title.
	Brief string
	// Severity is the level the event reports at.
	Severity Severity
}

// IsZero reports whether the Identifier carries no tag, which means it did not
// come from the generated tree.
func (identifier Identifier) IsZero() bool {
	return identifier.Tag == ""
}
