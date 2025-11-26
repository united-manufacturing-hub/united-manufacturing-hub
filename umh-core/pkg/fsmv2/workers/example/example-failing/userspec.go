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

package example_failing

// FailingUserSpec defines the typed configuration for the failing worker.
// This is parsed from the UserSpec.Config YAML/JSON string.
type FailingUserSpec struct {
	// ShouldFail controls whether the connect action should fail
	ShouldFail bool `yaml:"shouldFail" json:"shouldFail"`
}
