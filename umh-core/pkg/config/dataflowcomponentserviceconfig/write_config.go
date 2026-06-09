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

package dataflowcomponentserviceconfig

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// PlaceholderUMHTopicUnset is used when no input topics were configured for a write DFC.
// It makes the misconfiguration visible in Benthos logs rather than silently subscribing to nothing.
const PlaceholderUMHTopicUnset = "TOPIC_NOT_SET_BY_USER"

// WriteConfigSource holds the source (input) configuration for a write DFC.
type WriteConfigSource struct {
	// Topics is either a plain topic list (newline- or comma-separated, each line
	// optionally prefixed with "- ") or a Go template string that renders to such a list.
	Topics string `yaml:"topics,omitempty" json:"topics,omitempty" mapstructure:"topics,omitempty"`
}

// WriteConfigProcessing holds the processing configuration for a write DFC.
type WriteConfigProcessing struct {
	// Type is the processor type. Currently hardcoded to "nodered_js".
	Type string `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type,omitempty"`
	// Code is the processor code snippet (e.g. Node-RED JS).
	Code string `yaml:"code,omitempty" json:"code,omitempty" mapstructure:"code,omitempty"`
}

// WriteConfigDestination holds the destination (output) configuration for a write DFC.
type WriteConfigDestination struct {
	// Protocol is the Benthos output plugin name (e.g. "http_client", "kafka").
	// Currently hardcoded to "http_client".
	Protocol string `yaml:"protocol,omitempty" json:"protocol,omitempty" mapstructure:"protocol,omitempty"`
	// Code is the YAML body of the Benthos output plugin config.
	Code string `yaml:"code,omitempty" json:"code,omitempty" mapstructure:"code,omitempty"`
}

// WriteConfigExtra holds extra Benthos YAML that is inlined verbatim into the generated
// Benthos service config. Supported top-level keys: cache_resources, rate_limit_resources, buffer.
type WriteConfigExtra struct {
	Code string `yaml:"code,omitempty" json:"code,omitempty" mapstructure:"code,omitempty"`
}

// DataflowComponentWriteConfig is the typed, validated configuration for write-side DFCs.
// It is the internal form used by the FSM after rendering and parsing.
//
// YAML / JSON shape:
//
//	source:
//	  topics: |-
//	    - umh.v1.enterprise.site.line.device.*
//	    - umh.v1.enterprise.site.line.otherDevice.*
//	processing:
//	  type: "nodered_js"
//	  code: |-
//	    msg.payload["field"] = "value";
//	    return msg;
//	destination:
//	  type: "http_client"
//	  code: |-
//	    url: http://{{ .IP }}:{{ .PORT }}/post
//	    verb: POST
//	extra:
//	  code: |-
//	    buffer:
//	      memory: {}
type DataflowComponentWriteConfig struct {
	// Topics lists the UNS topics this write DFC subscribes to (parsed from Source.Topics).
	Topics      []string
	Processing  WriteConfigProcessing
	Destination WriteConfigDestination
	Extra       *WriteConfigExtra
}

// DataflowComponentWriteConfigInput is the wire/input form of write DFC config.
// Source.Topics may contain Go text/template actions and is rendered with
// user-supplied variables before being stored as the typed DataflowComponentWriteConfig.
//
// UnrecognizedFields captures any YAML keys not recognized by the current schema
// (e.g. fields from an older config format). They are round-tripped transparently
// so that a config written in an older format is never silently erased on read/write.
type DataflowComponentWriteConfigInput struct {
	Source      WriteConfigSource      `yaml:"source,omitempty"      json:"source,omitempty"      mapstructure:"source,omitempty"`
	Processing  WriteConfigProcessing  `yaml:"processing,omitempty"  json:"processing,omitempty"  mapstructure:"processing,omitempty"`
	Destination WriteConfigDestination `yaml:"destination,omitempty" json:"destination,omitempty" mapstructure:"destination,omitempty"`
	Extra       *WriteConfigExtra      `yaml:"extra,omitempty"       json:"extra,omitempty"       mapstructure:"extra,omitempty"`
	// UnrecognizedFields preserves unknown YAML keys for round-trip fidelity.
	UnrecognizedFields map[string]any `yaml:",inline" json:"-" mapstructure:"-"`
}

// HasOutput reports whether a write output is configured.
func (c DataflowComponentWriteConfigInput) HasOutput() bool {
	return c.Destination.Protocol != "" || c.Destination.Code != ""
}

// ToWriteConfig converts the input to a typed DataflowComponentWriteConfig by splitting
// Source.Topics without any template rendering. Use this when Source.Topics is already
// fully resolved (e.g. structural conversions, GET responses).
func (c DataflowComponentWriteConfigInput) ToWriteConfig() DataflowComponentWriteConfig {
	return DataflowComponentWriteConfig{
		Topics:      splitTopics(c.Source.Topics),
		Processing:  c.Processing,
		Destination: c.Destination,
		Extra:       c.Extra,
	}
}

// ToDataflowComponentServiceConfig expands the input config into a full Benthos service
// config. Source.Topics is split without template rendering — call Render first if the
// string still contains {{ }} actions.
func (c DataflowComponentWriteConfigInput) ToDataflowComponentServiceConfig(bridgedBy string) DataflowComponentServiceConfig {
	return c.ToWriteConfig().ToDataflowComponentServiceConfig(bridgedBy)
}

// splitTopics splits a newline- or comma-separated topic string into a trimmed,
// non-empty []string. YAML list entries prefixed with "- " are also handled.
func splitTopics(s string) []string {
	// normalize commas to newlines, then split
	s = strings.ReplaceAll(s, ",", "\n")
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		t = strings.TrimPrefix(t, "- ")
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// HasOutput reports whether a write output is configured.
func (c DataflowComponentWriteConfig) HasOutput() bool {
	return c.Destination.Protocol != "" || c.Destination.Code != ""
}

// ToDataflowComponentServiceConfig expands the typed write config into a full Benthos
// service config ready for the FSM to deploy.
//
// bridgedBy is the resolved consumer-group identifier. When empty the consumer_group
// field is left blank (acceptable for structural-only conversions).
func (c DataflowComponentWriteConfig) ToDataflowComponentServiceConfig(bridgedBy string) DataflowComponentServiceConfig {
	processorType := c.Processing.Type
	if processorType == "" {
		processorType = "nodered_js"
	}
	pipeline := c.codeToPipeline(c.Processing.Code, processorType)

	var input map[string]any
	var output map[string]any

	if c.HasOutput() {
		topics := c.Topics
		if len(topics) == 0 {
			topics = []string{PlaceholderUMHTopicUnset}
		}
		input = map[string]any{
			"uns": map[string]any{
				"consumer_group": bridgedBy,
				"umh_topics":     topics,
			},
		}

		if c.Destination.Protocol != "" {
			if c.Destination.Code != "" {
				var listConfig []any
				if err := yaml.Unmarshal([]byte(c.Destination.Code), &listConfig); err == nil {
					output = map[string]any{c.Destination.Protocol: listConfig}
				} else {
					destConfig := map[string]any{}
					if err := yaml.Unmarshal([]byte(c.Destination.Code), &destConfig); err != nil {
						destConfig = map[string]any{"_raw": c.Destination.Code}
					}
					output = map[string]any{c.Destination.Protocol: destConfig}
				}
			} else {
				output = map[string]any{c.Destination.Protocol: map[string]any{}}
			}
		}
	}

	cacheResources, rateLimitResources, buffer := c.parseExtra()
	return DataflowComponentServiceConfig{
		BenthosConfig: BenthosConfig{
			Input:              input,
			Pipeline:           pipeline,
			Output:             output,
			CacheResources:     cacheResources,
			RateLimitResources: rateLimitResources,
			Buffer:             buffer,
		},
	}
}

// ToDisplayDataflowComponentServiceConfig converts the typed write config into a
// DataflowComponentServiceConfig for display purposes (e.g. get-protocolconverter).
// The UNS input is intentionally omitted — it is auto-generated at render time.
func (c DataflowComponentWriteConfig) ToDisplayDataflowComponentServiceConfig() DataflowComponentServiceConfig {
	processorType := c.Processing.Type
	if processorType == "" {
		processorType = "nodered_js"
	}
	pipeline := c.codeToPipeline(c.Processing.Code, processorType)

	var output map[string]any
	if c.Destination.Protocol != "" {
		destConfig := map[string]any{}
		if c.Destination.Code != "" {
			if err := yaml.Unmarshal([]byte(c.Destination.Code), &destConfig); err != nil {
				destConfig = map[string]any{"_raw": c.Destination.Code}
			}
		}
		output = map[string]any{c.Destination.Protocol: destConfig}
	}

	cacheResources, rateLimitResources, buffer := c.parseExtra()
	return DataflowComponentServiceConfig{
		BenthosConfig: BenthosConfig{
			Pipeline:           pipeline,
			Output:             output,
			CacheResources:     cacheResources,
			RateLimitResources: rateLimitResources,
			Buffer:             buffer,
		},
	}
}

// parseExtra parses Extra.Code into the three benthos top-level inject fields.
// Known top-level keys: cache_resources, rate_limit_resources, buffer.
// Unknown keys in the YAML are silently ignored.
func (c DataflowComponentWriteConfig) parseExtra() (cacheResources []map[string]any, rateLimitResources []map[string]any, buffer map[string]any) {
	if c.Extra == nil || c.Extra.Code == "" {
		return nil, nil, nil
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(c.Extra.Code), &parsed); err != nil {
		return nil, nil, nil
	}
	if raw, ok := parsed["cache_resources"].([]any); ok {
		for _, item := range raw {
			if m, ok := item.(map[string]any); ok {
				cacheResources = append(cacheResources, m)
			}
		}
	}
	if raw, ok := parsed["rate_limit_resources"].([]any); ok {
		for _, item := range raw {
			if m, ok := item.(map[string]any); ok {
				rateLimitResources = append(rateLimitResources, m)
			}
		}
	}
	if m, ok := parsed["buffer"].(map[string]any); ok {
		buffer = m
	}
	return cacheResources, rateLimitResources, buffer
}

// codeToPipeline converts a code string and processor type to a Benthos pipeline config.
func (c DataflowComponentWriteConfig) codeToPipeline(code, processorType string) map[string]any {
	if code == "" {
		code = "return msg;"
	}
	return map[string]any{
		"processors": []any{
			map[string]any{
				processorType: map[string]any{
					"code": code,
				},
			},
		},
	}
}
