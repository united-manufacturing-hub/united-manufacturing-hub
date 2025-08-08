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

// Event represents the type of configuration event.
type Event int

// ConfigEvent represents a configuration change event.
type ConfigEvent interface {
	EventType() Event
	ServiceName() string
}

// Event types.
const (
	Created Event = iota
	Deleted
	StateChanged
	ConfigChanged
)

// State represents the desired state of a service.
type State int

// Service states.
const (
	Down State = iota
	Up
)

// EventCreated represents a service creation event.
type EventCreated struct {
	Name         string
	DesiredState State
	Executable   string
	Parameters   map[int]string
}

// EventType returns the event type.
func (e EventCreated) EventType() Event { return Created }

// ServiceName returns the service name.
func (e EventCreated) ServiceName() string { return e.Name }

// EventDeleted represents a service deletion event.
type EventDeleted struct {
	Name string
}

// EventType returns the event type.
func (e EventDeleted) EventType() Event { return Deleted }

// ServiceName returns the service name.
func (e EventDeleted) ServiceName() string { return e.Name }

// EventStateChanged represents a service state change event.
type EventStateChanged struct {
	Name         string
	DesiredState State
}

// EventType returns the event type.
func (e EventStateChanged) EventType() Event { return StateChanged }

// ServiceName returns the service name.
func (e EventStateChanged) ServiceName() string { return e.Name }

// EventConfigChanged represents a service configuration change event.
type EventConfigChanged struct {
	Name       string
	Executable string
	Parameters map[int]string
}

// EventType returns the event type.
func (e EventConfigChanged) EventType() Event { return ConfigChanged }

// ServiceName returns the service name.
func (e EventConfigChanged) ServiceName() string { return e.Name }
