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
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// DataflowComponentWriteConfig is the typed, validated configuration for write-side DFCs.
// It replaces the generic DataflowComponentServiceConfig for write flows, making the
// nodered_js processor and UNS topics first-class typed fields instead of free-form maps.
//
// This struct is the internal typed form used for FSM deployment. The wire format
// (models.WriteDFC) embeds DataflowComponentWriteConfigInput so that InputTopics remains
// a raw string (preserving template actions for round-trip fidelity).
//
// YAML / JSON shape:
//
//	input_topics:
//	  - umh.v1.enterprise.site.line.device.*
//	  - umh.v1.enterprise.site.line.otherDevice.*
//	processing_nodered_js: |-
//	  msg.payload["field"] = "value";
//	  return msg;
//	output:
//	  http_client:
//	    url: http://{{ .IP }}:{{ .PORT }}/post
//	    verb: POST
//	buffer:
//	  none: {}
type DataflowComponentWriteConfig struct {
	// InputTopics lists the UNS topics this write DFC subscribes to.
	InputTopics []string `yaml:"input_topics,omitempty" json:"input_topics,omitempty" mapstructure:"input_topics,omitempty"`

	// ProcessingNoderedJS is the Node-RED JS snippet that transforms each message.
	// Must end with "return msg;" (or "return null;" to drop the message).
	// Defaults to "return msg;" when empty.
	ProcessingNoderedJS string `yaml:"processing_nodered_js,omitempty" json:"processing_nodered_js,omitempty" mapstructure:"processing_nodered_js,omitempty"`

	// Output is the Benthos output configuration (e.g. http_client, mqtt).
	// When non-empty, a UNS input is automatically generated during rendering.
	Output map[string]any `yaml:"output,omitempty" json:"output,omitempty" mapstructure:"output,omitempty"`

	// Buffer is the optional Benthos buffer configuration.
	Buffer map[string]any `yaml:"buffer,omitempty" json:"buffer,omitempty" mapstructure:"buffer,omitempty"`
}

// DataflowComponentWriteConfigInput is the wire/input form of write DFC config.
// InputTopics is a single string that may contain Go text/template actions
// (e.g. "{{- range .topics }}{{ . }}\n{{ end }}") or a plain newline-/comma-separated
// topic list. It is rendered with user-supplied variables at action time and
// converted to the typed DataflowComponentWriteConfig before being stored.
type DataflowComponentWriteConfigInput struct {
	// InputTopics is either a plain topic list (newline- or comma-separated)
	// or a Go template string that renders to such a list.
	InputTopics         string         `yaml:"input_topics,omitempty"          json:"input_topics,omitempty"`
	ProcessingNoderedJS string         `yaml:"processing_nodered_js,omitempty" json:"processing_nodered_js,omitempty"`
	Output              map[string]any `yaml:"output,omitempty"                json:"output,omitempty"`
	Buffer              map[string]any `yaml:"buffer,omitempty"                json:"buffer,omitempty"`
}

// HasOutput reports whether a write output is configured.
func (c DataflowComponentWriteConfigInput) HasOutput() bool {
	return len(c.Output) > 0
}

// ToWriteConfig converts the input to a typed DataflowComponentWriteConfig by splitting
// InputTopics on newlines/commas without any template rendering. Use this when the
// InputTopics string is already fully resolved (e.g. structural conversions, GET responses).
func (c DataflowComponentWriteConfigInput) ToWriteConfig() DataflowComponentWriteConfig {
	return DataflowComponentWriteConfig{
		InputTopics:         splitTopics(c.InputTopics),
		ProcessingNoderedJS: c.ProcessingNoderedJS,
		Output:              c.Output,
		Buffer:              c.Buffer,
	}
}


// ToDataflowComponentServiceConfig expands the input config into a full Benthos service
// config. InputTopics is split without template rendering — call Render first if the
// string still contains {{ }} actions.
func (c DataflowComponentWriteConfigInput) ToDataflowComponentServiceConfig(bridgedBy string) DataflowComponentServiceConfig {
	return c.ToWriteConfig().ToDataflowComponentServiceConfig(bridgedBy)
}

// Render executes InputTopics as a Go template with scope, splits the result
// into individual topic strings, and returns a fully typed DataflowComponentWriteConfig.
func (c DataflowComponentWriteConfigInput) Render(scope map[string]any) (DataflowComponentWriteConfig, error) {
	rendered := c.InputTopics
	if strings.Contains(c.InputTopics, "{{") {
		tpl, err := template.New("input_topics").Option("missingkey=zero").Parse(c.InputTopics)
		if err != nil {
			return DataflowComponentWriteConfig{}, fmt.Errorf("failed to parse input_topics template: %w", err)
		}
		var buf bytes.Buffer
		if err := tpl.Execute(&buf, scope); err != nil {
			return DataflowComponentWriteConfig{}, fmt.Errorf("failed to render input_topics template: %w", err)
		}
		rendered = buf.String()
	}
	return DataflowComponentWriteConfig{
		InputTopics:         splitTopics(rendered),
		ProcessingNoderedJS: c.ProcessingNoderedJS,
		Output:              c.Output,
		Buffer:              c.Buffer,
	}, nil
}

// splitTopics splits a newline- or comma-separated topic string into a trimmed,
// non-empty []string.
func splitTopics(s string) []string {
	// normalise commas to newlines, then split
	s = strings.ReplaceAll(s, ",", "\n")
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// HasOutput reports whether a write output is configured.
// Used as a gate: the UNS input is only generated when an output exists.
func (c DataflowComponentWriteConfig) HasOutput() bool {
	return len(c.Output) > 0
}

// ToDataflowComponentServiceConfig expands the typed write config into a full Benthos
// service config ready for the FSM to deploy.
//
// bridgedBy is the resolved consumer-group identifier (from .internal.bridged_by in the
// render scope). When empty the consumer_group field is left blank, which is acceptable
// for structural-only conversions that never hit the wire (e.g. SpecToRuntime).
func (c DataflowComponentWriteConfig) ToDataflowComponentServiceConfig(bridgedBy string) DataflowComponentServiceConfig {
	code := c.ProcessingNoderedJS
	pipeline := c.codeToPipeline(code)

	var input map[string]any
	if len(c.Output) > 0 {
		umhTopics := c.InputTopics
		if len(umhTopics) == 0 {
			// Sentinel: action validation normally rejects an empty InputTopics when
			// Output is present, but set-config-file bypasses actions and can produce
			// this state. The sentinel makes the misconfiguration visible in Benthos
			// logs rather than silently subscribing to nothing.
			umhTopics = []string{"TOPIC_NOT_SET_BY_USER"}
		}
		input = map[string]any{
			"uns": map[string]any{
				"consumer_group": bridgedBy,
				"umh_topics":     umhTopics,
			},
		}
	}

	return DataflowComponentServiceConfig{
		BenthosConfig: BenthosConfig{
			Input:    input,
			Pipeline: pipeline,
			Output:   c.Output,
			Buffer:   c.Buffer,
		},
	}
}

// ToDisplayDataflowComponentServiceConfig converts the typed write config into a
// DataflowComponentServiceConfig for display purposes (e.g. get-protocolconverter).
// The UNS input is intentionally omitted — it is auto-generated at render time and
// must not be exposed to the frontend as if it were user-configured.
func (c DataflowComponentWriteConfig) ToDisplayDataflowComponentServiceConfig() DataflowComponentServiceConfig {
	code := c.ProcessingNoderedJS
	pipeline := c.codeToPipeline(code)

	return DataflowComponentServiceConfig{
		BenthosConfig: BenthosConfig{
			Pipeline: pipeline,
			Output:   c.Output,
			Buffer:   c.Buffer,
		},
	}
}

// codeToPipeline converts a code string to a Benthos pipeline configuration.
func (c DataflowComponentWriteConfig) codeToPipeline(code string) map[string]any {
	if code == "" {
		code = "return msg;"
	}
	pipeline := map[string]any{
		"processors": []any{
			map[string]any{
				"nodered_js": map[string]any{
					"code": code,
				},
			},
		},
	}
	return pipeline
}
