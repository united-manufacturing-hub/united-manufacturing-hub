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

package examplebridge

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"

// ExamplebridgeConfig is the typed configuration of the example bridge worker.
type ExamplebridgeConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	// Name is the bridge's name.
	Name string `json:"name" yaml:"name"`
	// Connection is the bridge's connection. Host and Port become the IP and
	// PORT template names.
	Connection Connection `json:"connection" yaml:"connection"`
	// Location is the bridge's own location levels, keyed by the level
	// index as written in YAML. The bridge merges these levels with the
	// agent's to build the location names.
	Location map[string]string `json:"location,omitempty" yaml:"location"`
	// Variables is the bridge's own variables section. It becomes the
	// "variables" template name and the source of one shortcut per variable.
	Variables map[string]any `json:"variables" yaml:"variables"`
	// Read is a flow template. The bridge keeps it unrendered: a flow renders
	// it against the names the bridge builds, never the bridge itself.
	Read config.Template `json:"read,omitempty" yaml:"read"`
	// Write is a flow template, like Read.
	Write config.Template `json:"write,omitempty" yaml:"write"`
	// Names are the template names DeriveDesiredState builds.
	Names map[string]any `json:"names,omitempty" yaml:"-"`
}

// Connection is the bridge's connection.
type Connection struct {
	// Host becomes the IP template name.
	Host string `json:"host" yaml:"host"`
	// Port becomes the PORT template name.
	Port int `json:"port" yaml:"port"`
}

// Template returns the flow template named by flow ("read" or "write").
func (c ExamplebridgeConfig) Template(flow string) config.Template {
	switch flow {
	case "read":
		return c.Read
	case "write":
		return c.Write
	default:
		return ""
	}
}

// ExamplebridgeStatus is the observed status of the example bridge worker.
type ExamplebridgeStatus struct{}
