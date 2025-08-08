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

// Package manager defines the interfaces for managing lifecycle operations
// of services in the s6-rc proof of concept.
package manager

import (
	"s6-rc-poc/cmd/shared"
)

// Service describes lifecycle management operations for a service.
// Implementations should perform create/remove/start/stop/restart/status
// and provide access to exit history for troubleshooting.
type Service interface {
	Create(name string, desiredState shared.State, executable string, parameters map[int]string) error
	Remove(name string) error
	Start(name string) error
	Stop(name string) error
	Restart(name string) error
	Status(name string) error
	ExitHistory(name string) error
}
