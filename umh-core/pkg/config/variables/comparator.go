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

import (
	"reflect"
)

// Comparator handles comparison of VariableBundle.
type Comparator struct{}

// NewComparator creates a new Comparator instance.
func NewComparator() *Comparator {
	return &Comparator{}
}

// ConfigsEqual checks if two VariableBundles are equal.
func (c *Comparator) ConfigsEqual(desired, observed VariableBundle) bool {
	return reflect.DeepEqual(desired, observed)
}

// ConfigDiff generates a diff between two VariableBundles.
func (c *Comparator) ConfigDiff(desired, observed VariableBundle) string {
	if c.ConfigsEqual(desired, observed) {
		return "Configs are identical"
	}

	diff := "Differences found:\n"

	// Compare User maps
	if !reflect.DeepEqual(desired.User, observed.User) {
		diff += "- User variables differ\n"
	}

	// Compare Global maps
	if !reflect.DeepEqual(desired.Global, observed.Global) {
		diff += "- Global variables differ\n"
	}

	// Compare Internal maps
	if !reflect.DeepEqual(desired.Internal, observed.Internal) {
		diff += "- Internal variables differ\n"
	}

	return diff
}
