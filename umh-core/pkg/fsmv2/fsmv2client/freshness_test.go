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

// Package fsmv2client_test exercises the FSMv2 client's read-side Freshness
// mapping and process-scoped singleton as an external caller would, through
// the exported NewFSMv2Client/GetFresh/SetClient/GetClient seam only.
package fsmv2client_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// testStatus is the typed child status GetFresh is parameterized over in these
// cases. Its contents are irrelevant to the reason mapping; only CollectedAt
// and the store's presence/absence drive the result.
type testStatus struct {
	V string
}

// stubStateReader is a tiny deps.StateReader the test drives to return either
// persistence.ErrNotFound (child never observed) or a populated Observation
// whose CollectedAt the case chooses.
type stubStateReader struct {
	obs *fsmv2.Observation[testStatus]
	err error
}

func (s *stubStateReader) LoadObservedTyped(_ context.Context, _, _ string, result interface{}) error {
	if s.err != nil {
		return s.err
	}

	if s.obs == nil {
		return nil
	}

	out, ok := result.(*fsmv2.Observation[testStatus])
	if !ok {
		return errors.New("stubStateReader: result is not *fsmv2.Observation[testStatus]")
	}

	*out = *s.obs

	return nil
}

// TestGetFresh_MapsChildObservationToReason asserts GetFresh maps a child
// observation to the correct Freshness reason for each read-side case and that
// the returned status is the staged observation for Fresh/Stale and the zero
// value otherwise.
func TestGetFresh_MapsChildObservationToReason(t *testing.T) {
	const maxAge = 10 * time.Second

	ref := dynamicchildren.Ref{WorkerType: "benthos_monitor", Name: "benthos-bridge-1"}

	cases := []struct {
		name       string
		upsert     bool // whether the ref is registered via writer.Upsert
		stubErr    error
		collected  time.Time // CollectedAt of the staged observation when stubErr == nil
		want       fsmv2client.Freshness
		wantStatus testStatus
	}{
		{
			name:       "Unregistered when ref was never Upserted",
			upsert:     false,
			want:       fsmv2client.Unregistered,
			wantStatus: testStatus{},
		},
		{
			name:       "NeverObserved when ref Upserted but store returns ErrNotFound",
			upsert:     true,
			stubErr:    persistence.ErrNotFound,
			want:       fsmv2client.NeverObserved,
			wantStatus: testStatus{},
		},
		{
			name:       "Stale when CollectedAt is older than maxAge",
			upsert:     true,
			collected:  time.Now().Add(-3 * maxAge),
			want:       fsmv2client.Stale,
			wantStatus: testStatus{V: "observed"},
		},
		{
			name:       "Fresh when CollectedAt is within maxAge",
			upsert:     true,
			collected:  time.Now().Add(-1 * time.Second),
			want:       fsmv2client.Fresh,
			wantStatus: testStatus{V: "observed"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writer := dynamicchildren.NewWriter()
			if tc.upsert {
				if err := writer.Upsert(ref, map[string]any{}); err != nil {
					t.Fatalf("writer.Upsert: %v", err)
				}
			}

			var staged *fsmv2.Observation[testStatus]
			if tc.stubErr == nil && tc.upsert {
				staged = &fsmv2.Observation[testStatus]{
					CollectedAt: tc.collected,
					Status:      testStatus{V: "observed"},
				}
			}

			stubSr := &stubStateReader{obs: staged, err: tc.stubErr}
			client := fsmv2client.NewFSMv2Client(writer, stubSr)

			gotStatus, got, err := fsmv2client.GetFresh[testStatus](context.Background(), client, ref, maxAge)
			if err != nil {
				t.Fatalf("GetFresh returned unexpected error: %v", err)
			}

			if got != tc.want {
				t.Fatalf("GetFresh reason = %v, want %v", got, tc.want)
			}

			if gotStatus != tc.wantStatus {
				t.Fatalf("GetFresh status = %+v, want %+v", gotStatus, tc.wantStatus)
			}
		})
	}
}

// TestSetClientGetClient_ProcessScopedAccessor asserts the process-scoped
// singleton: GetClient returns nil before SetClient and after SetClient(nil),
// and returns the published FSMv2Client after SetClient. This is the seam any
// FSMv1 benthos manager reads via fsmv2client.GetClient() regardless of which
// manager constructed it.
func TestSetClientGetClient_ProcessScopedAccessor(t *testing.T) {
	fsmv2client.SetClient(nil)

	if got := fsmv2client.GetClient(); got != nil {
		t.Fatalf("GetClient before SetClient = %v, want nil", got)
	}

	client := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), &stubStateReader{})
	fsmv2client.SetClient(client)

	if got := fsmv2client.GetClient(); got != client {
		t.Fatalf("GetClient after SetClient = %p, want %p", got, client)
	}

	fsmv2client.SetClient(nil)

	if got := fsmv2client.GetClient(); got != nil {
		t.Fatalf("GetClient after SetClient(nil) = %v, want nil", got)
	}
}
