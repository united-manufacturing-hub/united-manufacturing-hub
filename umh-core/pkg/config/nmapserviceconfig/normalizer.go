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

package nmapserviceconfig

// Normalizer handles the normalization of Nmap configurations.
type Normalizer struct{}

// NewNormalizer creates a new configuration normalizer for Nmap.
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeConfig doesn't do anything, there is no normalization needed here.
func (n *Normalizer) NormalizeConfig(cfg NmapServiceConfig) NmapServiceConfig {
	return cfg
}
