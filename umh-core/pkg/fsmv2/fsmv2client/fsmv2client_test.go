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

package fsmv2client

import (
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/configworker"
)

// TestUpsertAndDeletePassThroughToConfigWorker verifies the FSMv2Client delegates
// writes to the ConfigWorker it wraps: Upsert records the ref in the underlying
// shared registry (Lookup ok==true) and Delete removes it (Lookup ok==false). The
// client is constructed with a nil StateReader because A11 only stores the reader
// for a later typed Get and does not read it here.
func TestUpsertAndDeletePassThroughToConfigWorker(t *testing.T) {
	cw := configworker.NewConfigWorker()
	client := NewFSMv2Client(cw, nil)

	ref := configworker.Ref{WorkerType: "example", Name: "foo"}
	cfg := map[string]any{"greeting": "hello"}

	if err := client.Upsert(ref, cfg); err != nil {
		t.Fatalf("client.Upsert returned error: %v", err)
	}

	if _, ok := cw.Registry().Lookup(ref); !ok {
		t.Fatalf("ConfigWorker registry has no entry for ref %+v after client.Upsert", ref)
	}

	client.Delete(ref)

	if _, ok := cw.Registry().Lookup(ref); ok {
		t.Fatalf("ConfigWorker registry still holds ref %+v after client.Delete", ref)
	}
}
