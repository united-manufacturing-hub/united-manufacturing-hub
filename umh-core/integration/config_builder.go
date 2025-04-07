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

// builder.go (or in integration_test.go)
package integration_test

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"gopkg.in/yaml.v3"
)

type Builder struct {
	full config.FullConfig
}

func NewBuilder() *Builder {
	return &Builder{
		full: config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080, // Default port inside container
			},
			Internal: config.InternalConfig{
				Services: []config.S6FSMConfig{},
				Benthos:  []config.BenthosConfig{},
			},
		},
	}
}

// SetMetricsPort sets a custom metrics port for the agent
func (b *Builder) SetMetricsPort(port int) *Builder {
	b.full.Agent.MetricsPort = port
	return b
}

func (b *Builder) AddGoldenService() *Builder {
	b.full.Internal.Services = append(b.full.Internal.Services, config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            "golden-service",
			DesiredFSMState: "running",
		},
		S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
			Command: []string{
				"/usr/local/bin/benthos",
				"-c",
				"/run/service/golden-service/config/golden-service.yaml",
			},
			Env: map[string]string{
				"LOG_LEVEL": "DEBUG",
			},
			ConfigFiles: map[string]string{
				"golden-service.yaml": `---
input:
  http_server:
    path: /
    address: 0.0.0.0:8082
output:
  stdout: {}
`,
			},
		},
	})
	return b
}

func (b *Builder) AddService(s config.S6FSMConfig) *Builder {
	b.full.Internal.Services = append(b.full.Internal.Services, s)
	return b
}

func (b *Builder) BuildYAML() string {
	out, _ := yaml.Marshal(b.full)
	return string(out)
}

func (b *Builder) AddSleepService(name string, duration string) *Builder {
	b.full.Internal.Services = append(b.full.Internal.Services, config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: "running",
		},
		S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
			Command: []string{"sleep", duration},
		},
	})
	return b
}

// StopService stops a service by name
func (b *Builder) StopService(name string) *Builder {
	for i, s := range b.full.Internal.Services {
		if s.FSMInstanceConfig.Name == name {
			b.full.Internal.Services[i].FSMInstanceConfig.DesiredFSMState = "stopped"
			break
		}
	}
	return b
}

// StartService starts a service by name
func (b *Builder) StartService(name string) *Builder {
	for i, s := range b.full.Internal.Services {
		if s.FSMInstanceConfig.Name == name {
			b.full.Internal.Services[i].FSMInstanceConfig.DesiredFSMState = "running"
			break
		}
	}
	return b
}
