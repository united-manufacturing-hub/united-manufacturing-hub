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

package types_test

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

func TestChildAuthUserSpecRoundTrip(t *testing.T) {
	exp := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	in := types.ChildAuthUserSpec{AuthSession: types.AuthSession{Token: "jwt-abc", Expiry: exp, InstanceUUID: "be-uuid"}}

	data, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out types.ChildAuthUserSpec
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.AuthSession.Token != "jwt-abc" || out.AuthSession.InstanceUUID != "be-uuid" || !out.AuthSession.Expiry.Equal(exp) {
		t.Fatalf("round-trip mismatch: got %+v", out.AuthSession)
	}
}
