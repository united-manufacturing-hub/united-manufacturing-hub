// Copyright 2026 UMH Systems GmbH
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

package main

import (
	"fmt"

	"gopkg.in/yaml.v3"

	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
)

// renderSupervisorChildrenYAML renders the FSMv2 supervisor children document:
// the persistence child, then the children the Management Console module
// returned, in their order. The document is built with yaml.Marshal, so a
// child's config reaches the supervisor unchanged.
//
// The document carries no child's dependencies. They are live values that
// yaml cannot carry, and the document is stored; addChildDependencies hands
// them to the supervisor instead.
//
// A marshal failure is returned rather than swallowed: an empty document is a
// supervisor with no children at all, persistence included.
func renderSupervisorChildrenYAML(registered []fsmv2config.ChildSpec) (string, error) {
	children := make([]fsmv2config.ChildSpec, 0, 1+len(registered))
	children = append(children, fsmv2config.ChildSpec{Name: "persistence", WorkerType: "persistence"})

	for _, c := range registered {
		children = append(children, fsmv2config.ChildSpec{
			Name:       c.Name,
			WorkerType: c.WorkerType,
			UserSpec:   c.UserSpec,
			Enabled:    c.Enabled,
		})
	}

	doc, err := yaml.Marshal(map[string]any{"children": children})
	if err != nil {
		return "", fmt.Errorf("marshal supervisor children document: %w", err)
	}

	return string(doc), nil
}

// addChildDependencies adds the dependencies of the module's children to deps,
// the map the supervisor hands to every child. They stay live values. A child
// may not set a key that is already set, so the module cannot replace one of
// umh-core's own dependencies; the error names the child by its position.
func addChildDependencies(deps map[string]any, registered []fsmv2config.ChildSpec) error {
	for i, c := range registered {
		for key, value := range c.Dependencies {
			if _, ok := deps[key]; ok {
				return fmt.Errorf("child %d sets the dependency %q, which is already set", i, key)
			}

			deps[key] = value
		}
	}

	return nil
}
