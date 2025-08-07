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

package configwatcher

import "context"

// EventType represents the type of config change event
type EventType int

const (
	// Delete indicates a service configuration was deleted
	Delete EventType = iota
	// Create indicates a new service configuration was created
	Create
	// ConfigChange indicates service configuration details changed
	ConfigChange
	// StateChange indicates the desired state of a service changed
	StateChange
)

// String returns a string representation of the EventType
func (e EventType) String() string {
	switch e {
	case Delete:
		return "DELETE"
	case Create:
		return "CREATE"
	case ConfigChange:
		return "CONFIG_CHANGE"
	case StateChange:
		return "STATE_CHANGE"
	default:
		return "UNKNOWN"
	}
}

// ServiceDetails contains the configuration details of a service
type ServiceDetails struct {
	Type         string            `json:"type"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Contents     []string          `json:"contents,omitempty"`
	Environment  map[string]string `json:"environment,omitempty"`
	RunScript    string            `json:"runScript,omitempty"`
}

// DesiredState represents the desired state of a service
type DesiredState struct {
	Enabled bool `json:"enabled"`
	Running bool `json:"running"`
}

// Event represents a configuration change event
type Event struct {
	Type           EventType       `json:"type"`
	Name           string          `json:"name"`
	ServiceDetails *ServiceDetails `json:"serviceDetails,omitempty"`
	DesiredState   *DesiredState   `json:"desiredState,omitempty"`
}

// ConfigWatcher defines the interface for watching configuration changes
type ConfigWatcher interface {
	// Watch starts watching the configuration directory and returns a channel of events.
	// The channel will be closed when the context is cancelled or an unrecoverable error occurs.
	Watch(ctx context.Context, configPath string) (<-chan Event, error)

	// Close cleanly shuts down the watcher and releases any resources
	Close() error
}
