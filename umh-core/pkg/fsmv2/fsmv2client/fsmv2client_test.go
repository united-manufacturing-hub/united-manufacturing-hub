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
	"context"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
)

// TestUpsertAndDeletePassThroughToWriter verifies the FSMv2Client delegates
// writes to the Writer it wraps: Upsert records the ref in the underlying
// shared registry (Lookup ok==true) and Delete removes it (Lookup ok==false). The
// client is constructed with a nil StateReader because A11 only stores the reader
// for a later typed Get and does not read it here.
func TestUpsertAndDeletePassThroughToWriter(t *testing.T) {
	w := dynamicchildren.NewWriter()
	client := NewFSMv2Client(w, nil)

	ref := dynamicchildren.Ref{WorkerType: "example", Name: "foo"}
	cfg := map[string]any{"greeting": "hello"}

	if err := client.Upsert(ref, cfg); err != nil {
		t.Fatalf("client.Upsert returned error: %v", err)
	}

	if _, ok := w.Registry().Lookup(ref); !ok {
		t.Fatalf("Writer registry has no entry for ref %+v after client.Upsert", ref)
	}

	client.Delete(ref)

	if _, ok := w.Registry().Lookup(ref); ok {
		t.Fatalf("Writer registry still holds ref %+v after client.Delete", ref)
	}
}

// TestGetReturnsErrorOnNilStateReader verifies that Get on a write-only client
// (one built with a nil StateReader, a documented and used construction) returns
// an error instead of panicking on the nil dereference.
func TestGetReturnsErrorOnNilStateReader(t *testing.T) {
	w := dynamicchildren.NewWriter()
	client := NewFSMv2Client(w, nil)

	ref := dynamicchildren.Ref{WorkerType: "example", Name: "foo"}

	if _, err := Get[struct{}](context.Background(), client, ref); err == nil {
		t.Fatalf("Get on a nil-StateReader client returned nil error, want a non-nil error rather than a panic")
	}
}
