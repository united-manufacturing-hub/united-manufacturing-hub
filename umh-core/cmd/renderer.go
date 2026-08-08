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
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// childNode is a single FSMv2 supervisor child in the rendered YAML document.
type childNode struct {
	Name       string `yaml:"name"`
	WorkerType string `yaml:"workerType"`
	UserSpec   *struct {
		Config string `yaml:"config"`
	} `yaml:"userSpec,omitempty"`
}

// communicatorSpec is the userSpec config for the communicator child. It is
// rendered via yaml.Marshal so that credentials round-trip byte-for-byte even
// when they contain characters (quotes, backslashes, newlines) that inline
// string formatting would mangle.
type communicatorSpec struct {
	RelayURL     string `yaml:"relayURL"`
	InstanceUUID string `yaml:"instanceUUID"`
	AuthToken    string `yaml:"authToken"`
	Timeout      string `yaml:"timeout"`
	State        string `yaml:"state"`
}

// renderSupervisorChildrenYAML renders the FSMv2 supervisor children document.
// The persistence child is always present; the communicator child appears only
// when both APIURL and AuthToken are set (E2 contract). The instanceUUID is
// interpolated into the communicator's userSpec config.
func renderSupervisorChildrenYAML(cfg config.AgentConfig, instanceUUID string) string {
	children := []childNode{
		{Name: "persistence", WorkerType: "persistence"},
	}

	if cfg.APIURL != "" && cfg.AuthToken != "" {
		specBytes, err := yaml.Marshal(communicatorSpec{
			RelayURL:     cfg.APIURL,
			InstanceUUID: instanceUUID,
			AuthToken:    cfg.AuthToken,
			Timeout:      "10s",
			State:        "running",
		})
		if err != nil {
			return ""
		}
		children = append(children, childNode{
			Name:       "communicator",
			WorkerType: "communicator",
			UserSpec:   &struct {
				Config string `yaml:"config"`
			}{Config: string(specBytes)},
		})
	}

	doc, err := yaml.Marshal(map[string]any{"children": children})
	if err != nil {
		return ""
	}
	return string(doc)
}
