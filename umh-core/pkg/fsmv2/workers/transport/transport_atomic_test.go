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

package transport_test

import (
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// TestTransportConfigDefaultsTimeout verifies GetTimeout() returns a non-zero default
// when Timeout==0. The accessor centralises the defaulting that callers rely on.
func TestTransportConfigDefaultsTimeout(t *testing.T) {
	cfg := transport.TransportConfig{}
	if cfg.GetTimeout() == 0 {
		t.Fatal("GetTimeout() must return non-zero default when Timeout is 0")
	}
}

// TestTransportEmptyCredsReachAuthAction verifies DeriveDesiredState succeeds with
// empty relayURL/instanceUUID/authToken even when state=running. Empty creds flow
// through to AuthenticateAction and surface as classified errors via AuthFailedState.
func TestTransportEmptyCredsReachAuthAction(t *testing.T) {
	w := &transport.TransportWorker{}
	spec := config.UserSpec{
		Config: "state: running",
	}
	if _, err := w.DeriveDesiredState(spec); err != nil {
		t.Fatalf("DeriveDesiredState with empty creds must not error: %v", err)
	}
}
