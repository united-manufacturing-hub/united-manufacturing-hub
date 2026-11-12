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

package fsmv2

import (
	"reflect"
	"testing"
)

// TestObservationFrameworkFieldsMirrorObservation checks that every JSON field
// of Observation, other than the flattened Status, is also in
// observationFrameworkFields. MarshalJSON and UnmarshalJSON copy fields
// through that type, so a field missing there is silently dropped.
func TestObservationFrameworkFieldsMirrorObservation(t *testing.T) {
	observation := collectJSONFieldNames(reflect.TypeOf(Observation[struct{}]{}))
	framework := collectJSONFieldNames(reflect.TypeOf(observationFrameworkFields{}))

	if len(observation) == 0 {
		t.Fatal("found no JSON fields on Observation; the check would pass vacuously")
	}

	if !reflect.DeepEqual(observation, framework) {
		t.Fatalf("Observation JSON fields %v differ from observationFrameworkFields %v", observation, framework)
	}
}
