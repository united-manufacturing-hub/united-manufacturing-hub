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
	"reflect"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
)

// TestRefsListsRegisteredWorkersOfType verifies that Refs lists exactly the
// registered workers of the asked-for worker type, ordered by name: an other
// type's worker is absent, a deleted worker is absent, and a worker of the
// asked-for type registered last still lists in name order.
func TestRefsListsRegisteredWorkersOfType(t *testing.T) {
	w := dynamicchildren.NewWriter()
	client := NewFSMv2Client(w, nil)

	refs := []dynamicchildren.Ref{
		{WorkerType: "benthos_metrics", Name: "benthos-b"},
		{WorkerType: "benthos_metrics", Name: "benthos-a"},
		{WorkerType: "other", Name: "other-a"},
		{WorkerType: "benthos_metrics", Name: "benthos-deleted"},
	}
	for _, ref := range refs {
		if err := client.Upsert(ref, map[string]any{}); err != nil {
			t.Fatalf("client.Upsert(%+v) returned error: %v", ref, err)
		}
	}

	client.Delete(dynamicchildren.Ref{WorkerType: "benthos_metrics", Name: "benthos-deleted"})

	got := client.Refs("benthos_metrics")
	want := []dynamicchildren.Ref{
		{WorkerType: "benthos_metrics", Name: "benthos-a"},
		{WorkerType: "benthos_metrics", Name: "benthos-b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("client.Refs(\"benthos_metrics\") = %+v, want %+v", got, want)
	}

	if got := client.Refs("nobody-registered-this"); len(got) != 0 {
		t.Fatalf("client.Refs of an unregistered type = %+v, want none", got)
	}
}

// TestRefsOnClientWithoutWriter verifies that Refs on a client built without
// a Writer answers no refs instead of panicking.
func TestRefsOnClientWithoutWriter(t *testing.T) {
	client := NewFSMv2Client(nil, nil)

	if got := client.Refs("benthos_metrics"); len(got) != 0 {
		t.Fatalf("Refs on a client without a Writer = %+v, want none", got)
	}

	var nilClient *FSMv2Client
	if got := nilClient.Refs("benthos_metrics"); len(got) != 0 {
		t.Fatalf("Refs on a nil client = %+v, want none", got)
	}
}
