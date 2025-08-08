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

package manager

import (
	"fmt"

	"s6-rc-poc/cmd/shared"
)

// Create creates or updates a service definition and applies it. Desired
// state is represented via bundle membership: services included in the
// bundle are considered "up" after changeover; excluded are "down".
func (s *S6RCService) Create(
	name string,
	desiredState shared.State,
	executable string,
	parameters map[int]string,
) error { // interface method
	if name == "" {
		return errServiceNameRequired
	}
	if executable == "" {
		return fmt.Errorf("service %s: %w", name, errExecutableRequired)
	}

	if err := s.writeServiceDefinition(name, executable, parameters); err != nil {
		return err
	}

	// Represent desired state via bundle membership
	if err := s.setBundleMembership(name, desiredState == shared.Up); err != nil {
		return err
	}

	return s.compileAndChangeover()
}
