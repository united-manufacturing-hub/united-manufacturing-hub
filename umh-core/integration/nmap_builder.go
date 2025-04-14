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

package integration_test

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"gopkg.in/yaml.v3"
)

// NmapBuilder is used to build configuration with Nmap service
type NmapBuilder struct {
	full config.FullConfig
	// Map to track which services are active by name
	activeNmap map[string]bool
}

// NewNmapBuilder creates a new builder for Benthos configurations
func NewNmapBuilder() *NmapBuilder {
	return &NmapBuilder{
		full: config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
			},
			Internal: config.InternalConfig{
				Services: []config.S6FSMConfig{},
				Nmap:     []config.NmapConfig{},
			},
		},
		activeNmap: make(map[string]bool),
	}
}

// AddGoldenNmap adds a Nmap service that serves HTTP requests on port 8082
func (b *NmapBuilder) AddGoldenNmap() *NmapBuilder {
	// Create Nmap config
	nmapConfig := config.NmapConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            "golden-nmap",
			DesiredFSMState: "port_open",
		},
		NmapServiceConfig: config.NmapServiceConfig{
			Target: "0.0.0.0",
			Port:   443,
		},
	}

	// Add to configuration
	b.full.Internal.Nmap = append(b.full.Internal.Nmap, nmapConfig)
	b.activeNmap["golden-nmap"] = true

	b.full.Internal.Redpanda = config.RedpandaConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            "redpanda",
			DesiredFSMState: "stopped",
		},
	}
	return b
}

// StartNmap sets a Nmap service to active state
func (b *NmapBuilder) StartNmap(name string) *NmapBuilder {
	for i, nmap := range b.full.Internal.Nmap {
		if nmap.FSMInstanceConfig.Name == name {
			b.full.Internal.Nmap[i].FSMInstanceConfig.DesiredFSMState = "active"
			b.activeNmap[name] = true
			break
		}
	}
	return b
}

// StopNmap sets a Nmap service to stopped state
func (b *NmapBuilder) StopNmap(name string) *NmapBuilder {
	for i, nmap := range b.full.Internal.Nmap {
		if nmap.FSMInstanceConfig.Name == name {
			b.full.Internal.Nmap[i].FSMInstanceConfig.DesiredFSMState = "stopped"
			b.activeNmap[name] = false
			break
		}
	}
	return b
}

// CountActiveNmap returns the number of active Benthos services
func (b *NmapBuilder) CountActiveNmap() int {
	count := 0
	for _, isActive := range b.activeNmap {
		if isActive {
			count++
		}
	}
	return count
}

// BuildYAML converts the configuration to YAML format
func (b *NmapBuilder) BuildYAML() string {
	out, _ := yaml.Marshal(b.full)
	return string(out)
}
