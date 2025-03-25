package integration_test

import (
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
	"gopkg.in/yaml.v3"
)

// BenthosBuilder is used to build configuration with Benthos services
type BenthosBuilder struct {
	full config.FullConfig
	// Map to track which services are active by name
	activeBenthos map[string]bool
}

// NewBenthosBuilder creates a new builder for Benthos configurations
func NewBenthosBuilder() *BenthosBuilder {
	return &BenthosBuilder{
		full: config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
			},
			Services: []config.S6FSMConfig{},
			Benthos:  []config.BenthosConfig{},
		},
		activeBenthos: make(map[string]bool),
	}
}

// AddGoldenBenthos adds a Benthos service that serves HTTP requests on port 8082
func (b *BenthosBuilder) AddGoldenBenthos() *BenthosBuilder {
	// Create Benthos config with an HTTP server input
	benthosConfig := config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            "golden-benthos",
			DesiredFSMState: "active",
		},
		BenthosServiceConfig: config.BenthosServiceConfig{
			MetricsPort: 0, // Auto-assign port
			Input: map[string]interface{}{
				"http_server": map[string]interface{}{
					"path":    "/",
					"address": "0.0.0.0:8082",
				},
			},
			Output: map[string]interface{}{
				"stdout": map[string]interface{}{},
			},
			LogLevel: "DEBUG",
		},
	}

	// Add to configuration
	b.full.Benthos = append(b.full.Benthos, benthosConfig)
	b.activeBenthos["golden-benthos"] = true
	return b
}

// AddGeneratorBenthos adds a Benthos service that generates messages and sends to stdout
func (b *BenthosBuilder) AddGeneratorBenthos(name string, interval string) *BenthosBuilder {
	// Create Benthos config with a generator input
	benthosConfig := config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: "active",
		},
		BenthosServiceConfig: config.BenthosServiceConfig{
			MetricsPort: 0, // Auto-assign port
			Input: map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  `root = "message from ` + name + `"`,
					"interval": interval,
					"count":    0, // Unlimited
				},
			},
			Output: map[string]interface{}{
				"stdout": map[string]interface{}{},
			},
			LogLevel: "INFO",
		},
	}

	// Add to configuration
	b.full.Benthos = append(b.full.Benthos, benthosConfig)
	b.activeBenthos[name] = true
	return b
}

// UpdateGeneratorBenthos updates the interval of an existing generator Benthos service
func (b *BenthosBuilder) UpdateGeneratorBenthos(name string, newInterval string) *BenthosBuilder {
	// Find and update the service
	for i, benthos := range b.full.Benthos {
		if benthos.FSMInstanceConfig.Name == name {
			// Create updated input config with new interval
			if input, ok := benthos.BenthosServiceConfig.Input["generate"].(map[string]interface{}); ok {
				input["interval"] = newInterval
				b.full.Benthos[i].BenthosServiceConfig.Input["generate"] = input
			}
			break
		}
	}
	return b
}

// StartBenthos sets a Benthos service to active state
func (b *BenthosBuilder) StartBenthos(name string) *BenthosBuilder {
	for i, benthos := range b.full.Benthos {
		if benthos.FSMInstanceConfig.Name == name {
			b.full.Benthos[i].FSMInstanceConfig.DesiredFSMState = "active"
			b.activeBenthos[name] = true
			break
		}
	}
	return b
}

// StopBenthos sets a Benthos service to inactive state
func (b *BenthosBuilder) StopBenthos(name string) *BenthosBuilder {
	for i, benthos := range b.full.Benthos {
		if benthos.FSMInstanceConfig.Name == name {
			b.full.Benthos[i].FSMInstanceConfig.DesiredFSMState = "stopped"
			b.activeBenthos[name] = false
			break
		}
	}
	return b
}

// CountActiveBenthos returns the number of active Benthos services
func (b *BenthosBuilder) CountActiveBenthos() int {
	count := 0
	for _, isActive := range b.activeBenthos {
		if isActive {
			count++
		}
	}
	return count
}

// BuildYAML converts the configuration to YAML format
func (b *BenthosBuilder) BuildYAML() string {
	out, _ := yaml.Marshal(b.full)
	return string(out)
}
