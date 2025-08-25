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

package topicbrowserserviceconfig

import (
	benthossvccfg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
)

// In the TopicBrowser we do not have any user configurable settings, therefore these packages are bare-bones

// Normalizer handles the normalization of Topic Browser configurations.
type Normalizer struct{}

// NewNormalizer creates a new configuration normalizer for Topic Browser.
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeConfig doesn't do anything, there is no normalization needed here.
func (n *Normalizer) NormalizeConfig(cfg Config) Config {
	// Our normalizer will setup a default if not set (just like redpanda)
	normalized := cfg

	if len(normalized.BenthosConfig.Input) == 0 || len(normalized.BenthosConfig.Pipeline) == 0 || len(normalized.BenthosConfig.Output) == 0 {
		normalized.BenthosConfig = benthossvccfg.DefaultTopicBrowserBenthosServiceConfig
	}

	return normalized
}
