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

package variables

var (
	defaultGenerator  = NewGenerator()
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// RenderVariablesYAML is a package-level function for easy YAML generation.
func RenderVariablesYAML(vb VariableBundle) (string, error) {
	return defaultGenerator.RenderConfig(vb)
}

// NormalizeVariables is a package-level function for easy config normalization.
func NormalizeVariables(vb VariableBundle) VariableBundle {
	return defaultNormalizer.NormalizeConfig(vb)
}

// ConfigsEqual is a package-level function for easy config comparison.
func ConfigsEqual(desired, observed VariableBundle) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation.
func ConfigDiff(desired, observed VariableBundle) string {
	return defaultComparator.ConfigDiff(desired, observed)
}
