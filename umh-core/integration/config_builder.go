// builder.go (or in integration_test.go)
package integration_test

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"gopkg.in/yaml.v3"
)

type Builder struct {
	full config.FullConfig
}

func NewBuilder() *Builder {
	return &Builder{
		full: config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
			},
			Services: []config.S6FSMConfig{},
			Benthos:  []config.BenthosConfig{},
		},
	}
}

func (b *Builder) AddGoldenService() *Builder {
	b.full.Services = append(b.full.Services, config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            "golden-service",
			DesiredFSMState: "running",
		},
		S6ServiceConfig: config.S6ServiceConfig{
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
	b.full.Services = append(b.full.Services, s)
	return b
}

func (b *Builder) BuildYAML() string {
	out, _ := yaml.Marshal(b.full)
	return string(out)
}

func (b *Builder) AddSleepService(name string, duration string) *Builder {
	b.full.Services = append(b.full.Services, config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: "running",
		},
		S6ServiceConfig: config.S6ServiceConfig{
			Command: []string{"sleep", duration},
		},
	})
	return b
}

// StopService stops a service by name
func (b *Builder) StopService(name string) *Builder {
	for i, s := range b.full.Services {
		if s.FSMInstanceConfig.Name == name {
			b.full.Services[i].FSMInstanceConfig.DesiredFSMState = "stopped"
			break
		}
	}
	return b
}

// StartService starts a service by name
func (b *Builder) StartService(name string) *Builder {
	for i, s := range b.full.Services {
		if s.FSMInstanceConfig.Name == name {
			b.full.Services[i].FSMInstanceConfig.DesiredFSMState = "running"
			break
		}
	}
	return b
}
