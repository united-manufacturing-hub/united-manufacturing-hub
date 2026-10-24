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
// A marshal failure is returned rather than swallowed: an empty document is a
// supervisor with no children at all, persistence included.
func renderSupervisorChildrenYAML(registered []fsmv2config.ChildSpec) (string, error) {
	children := make([]fsmv2config.ChildSpec, 0, 1+len(registered))
	children = append(children, fsmv2config.ChildSpec{Name: "persistence", WorkerType: "persistence"})
	children = append(children, registered...)

	doc, err := yaml.Marshal(map[string]any{"children": children})
	if err != nil {
		return "", fmt.Errorf("marshal supervisor children document: %w", err)
	}

	return string(doc), nil
}
