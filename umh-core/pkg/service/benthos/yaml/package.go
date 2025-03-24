package yaml

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

var (
	defaultGenerator  = NewGenerator()
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// RenderBenthosYAML is a package-level function for easy YAML generation
func RenderBenthosYAML(input, output, pipeline, cacheResources, rateLimitResources, buffer interface{}, metricsPort int, logLevel string) (string, error) {
	// Create a config object from the individual components
	cfg := config.BenthosServiceConfig{
		MetricsPort: metricsPort,
		LogLevel:    logLevel,
	}

	// Convert each section to the appropriate map
	if input != nil {
		if inputMap, ok := input.(map[string]interface{}); ok {
			cfg.Input = inputMap
		}
	}

	if output != nil {
		if outputMap, ok := output.(map[string]interface{}); ok {
			cfg.Output = outputMap
		}
	}

	if pipeline != nil {
		if pipelineMap, ok := pipeline.(map[string]interface{}); ok {
			cfg.Pipeline = pipelineMap
		}
	}

	if buffer != nil {
		if bufferMap, ok := buffer.(map[string]interface{}); ok {
			cfg.Buffer = bufferMap
		}
	}

	// Handle resources
	if cacheResources != nil {
		if cacheArray, ok := cacheResources.([]map[string]interface{}); ok {
			cfg.CacheResources = cacheArray
		} else if cacheList, ok := cacheResources.([]interface{}); ok {
			// Try to convert each item to the expected type
			for _, item := range cacheList {
				if resMap, ok := item.(map[string]interface{}); ok {
					cfg.CacheResources = append(cfg.CacheResources, resMap)
				}
			}
		}
	}

	if rateLimitResources != nil {
		if rateArray, ok := rateLimitResources.([]map[string]interface{}); ok {
			cfg.RateLimitResources = rateArray
		} else if rateList, ok := rateLimitResources.([]interface{}); ok {
			// Try to convert each item to the expected type
			for _, item := range rateList {
				if resMap, ok := item.(map[string]interface{}); ok {
					cfg.RateLimitResources = append(cfg.RateLimitResources, resMap)
				}
			}
		}
	}

	// Use the generator to render the YAML
	return defaultGenerator.RenderConfig(cfg)
}

// NormalizeBenthosConfig is a package-level function for easy config normalization
func NormalizeBenthosConfig(cfg config.BenthosServiceConfig) config.BenthosServiceConfig {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison
func ConfigsEqual(desired, observed config.BenthosServiceConfig) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation
func ConfigDiff(desired, observed config.BenthosServiceConfig) string {
	return defaultComparator.ConfigDiff(desired, observed)
}
