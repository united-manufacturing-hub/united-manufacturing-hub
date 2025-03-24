package yaml

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// Normalizer handles the normalization of Benthos configurations
type Normalizer struct{}

// NewNormalizer creates a new configuration normalizer for Benthos
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeConfig applies Benthos defaults to a structured config
func (n *Normalizer) NormalizeConfig(cfg config.BenthosServiceConfig) config.BenthosServiceConfig {
	// Create a copy
	normalized := cfg

	// Ensure Input map exists
	if normalized.Input == nil {
		normalized.Input = make(map[string]interface{})
	}

	// Ensure Output map exists
	if normalized.Output == nil {
		normalized.Output = make(map[string]interface{})
	}

	// Ensure Pipeline map exists with processors
	if normalized.Pipeline == nil {
		normalized.Pipeline = map[string]interface{}{
			"processors": []interface{}{},
		}
	} else if _, exists := normalized.Pipeline["processors"]; !exists {
		normalized.Pipeline["processors"] = []interface{}{}
	}

	// Set default buffer if missing
	if normalized.Buffer == nil || len(normalized.Buffer) == 0 {
		normalized.Buffer = map[string]interface{}{
			"none": map[string]interface{}{},
		}
	}

	// Set default metrics port if not specified
	if normalized.MetricsPort == 0 {
		normalized.MetricsPort = 4195
	}

	// Set default log level if not specified
	if normalized.LogLevel == "" {
		normalized.LogLevel = "INFO"
	}

	return normalized
}
