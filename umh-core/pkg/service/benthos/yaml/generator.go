package yaml

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
	"gopkg.in/yaml.v3"
)

// Generator handles the generation of Benthos YAML configurations
type Generator struct {
	tmpl *template.Template
}

// NewGenerator creates a new YAML generator for Benthos configurations
func NewGenerator() *Generator {
	return &Generator{
		tmpl: template.Must(template.New("benthos").Parse(simplifiedTemplate)),
	}
}

// RenderConfig generates a Benthos YAML configuration from a BenthosServiceConfig
func (g *Generator) RenderConfig(cfg config.BenthosServiceConfig) (string, error) {
	if cfg.LogLevel == "" {
		cfg.LogLevel = "INFO"
	}

	// Convert the config to a normalized map
	configMap := g.configToMap(cfg)
	normalizedMap := normalizeConfig(configMap)

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(normalizedMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Benthos config: %w", err)
	}

	// Fix the indentation for http and logger sections to match the expected format
	yamlStr := string(yamlBytes)
	yamlStr = strings.ReplaceAll(yamlStr, "http:\n    address:", "http:\n  address:")
	yamlStr = strings.ReplaceAll(yamlStr, "logger:\n    level:", "logger:\n  level:")

	return yamlStr, nil
}

// configToMap converts a BenthosServiceConfig to a raw map for YAML generation
func (g *Generator) configToMap(cfg config.BenthosServiceConfig) map[string]interface{} {
	configMap := make(map[string]interface{})

	// Add all sections
	if cfg.Input != nil && len(cfg.Input) > 0 {
		configMap["input"] = cfg.Input
	}

	if cfg.Output != nil && len(cfg.Output) > 0 {
		configMap["output"] = cfg.Output
	}

	if cfg.Pipeline != nil {
		configMap["pipeline"] = cfg.Pipeline
	} else {
		configMap["pipeline"] = map[string]interface{}{
			"processors": []interface{}{},
		}
	}

	if cfg.Buffer != nil && len(cfg.Buffer) > 0 {
		configMap["buffer"] = cfg.Buffer
	} else {
		configMap["buffer"] = map[string]interface{}{
			"none": map[string]interface{}{},
		}
	}

	// Add cache resources if present
	if len(cfg.CacheResources) > 0 {
		configMap["cache_resources"] = cfg.CacheResources
	}

	// Add rate limit resources if present
	if len(cfg.RateLimitResources) > 0 {
		configMap["rate_limit_resources"] = cfg.RateLimitResources
	}

	// Add HTTP section with metrics port
	configMap["http"] = map[string]interface{}{
		"address": fmt.Sprintf("0.0.0.0:%d", cfg.MetricsPort),
	}

	// Add logger section with log level
	configMap["logger"] = map[string]interface{}{
		"level": cfg.LogLevel,
	}

	return configMap
}

// normalizeConfig applies Benthos defaults to ensure consistent comparison
func normalizeConfig(raw map[string]interface{}) map[string]interface{} {
	// Create a deep copy to avoid modifying the original
	normalized := make(map[string]interface{})
	for k, v := range raw {
		normalized[k] = v
	}

	// Ensure input exists and is rendered as an array when empty
	if input, ok := normalized["input"].(map[string]interface{}); ok {
		if len(input) == 0 {
			normalized["input"] = []interface{}{}
		}
	} else {
		normalized["input"] = []interface{}{}
	}

	// Ensure output exists and is rendered as an array when empty
	if output, ok := normalized["output"].(map[string]interface{}); ok {
		if len(output) == 0 {
			normalized["output"] = []interface{}{}
		}
	} else {
		normalized["output"] = []interface{}{}
	}

	// Ensure pipeline section exists with processors array
	if pipeline, ok := normalized["pipeline"].(map[string]interface{}); ok {
		if _, exists := pipeline["processors"]; !exists {
			pipeline["processors"] = []interface{}{}
		}
	} else {
		normalized["pipeline"] = map[string]interface{}{
			"processors": []interface{}{},
		}
	}

	// Set default buffer if missing
	if buffer, ok := normalized["buffer"].(map[string]interface{}); ok {
		if len(buffer) == 0 {
			normalized["buffer"] = map[string]interface{}{
				"none": map[string]interface{}{},
			}
		}
	} else {
		normalized["buffer"] = map[string]interface{}{
			"none": map[string]interface{}{},
		}
	}

	// Ensure http section with address exists
	if http, ok := normalized["http"].(map[string]interface{}); ok {
		if _, exists := http["address"]; !exists {
			http["address"] = "0.0.0.0:4195"
		}
	} else {
		normalized["http"] = map[string]interface{}{
			"address": "0.0.0.0:4195",
		}
	}

	// Ensure logger section with level exists
	if logger, ok := normalized["logger"].(map[string]interface{}); ok {
		if _, exists := logger["level"]; !exists {
			logger["level"] = "INFO"
		}
	} else {
		normalized["logger"] = map[string]interface{}{
			"level": "INFO",
		}
	}

	return normalized
}

// templateData represents the data structure expected by the simplified Benthos YAML template
type templateData struct {
	Input              string
	Output             string
	Pipeline           string
	CacheResources     string
	RateLimitResources string
	Buffer             string
	MetricsPort        int
	LogLevel           string
}

// simplifiedTemplate is a much simpler template that just places pre-rendered YAML blocks
var simplifiedTemplate = `input:{{.Input}}

output:{{.Output}}

pipeline:{{.Pipeline}}

cache_resources:{{.CacheResources}}

rate_limit_resources:{{.RateLimitResources}}

buffer:{{.Buffer}}

http:
  address: 0.0.0.0:{{.MetricsPort}}

logger:
  level: {{.LogLevel}}
`
