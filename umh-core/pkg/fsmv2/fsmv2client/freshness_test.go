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
// cases. Its contents do not affect the Freshness value; CollectedAt,
// DeletedAt and whether anything is stored do.
type testStatus struct {
	V string
}

// stubStateReader is a tiny deps.StateReader the test drives to return an
// error (persistence.ErrNotFound or another) or a populated Observation whose
// CollectedAt and DeletedAt the case chooses.
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

func TestGetFresh_ClassifiesEachReadResult(t *testing.T) {
	const maxAge = 10 * time.Second

	ref := dynamicchildren.Ref{WorkerType: "transport", Name: "transport"}
	deletedAt := time.Now().Add(-time.Minute)
	storeErr := errors.New("generic store failure")

	cases := []struct {
		name    string
		stubErr error
		staged  *fsmv2.Observation[testStatus]
		want    fsmv2client.Freshness
		wantObs bool // whether GetFresh returns the staged observation
		wantErr error
	}{
		{
			name:    "Unknown when the store returns another error",
			stubErr: storeErr,
			want:    fsmv2client.Unknown,
			wantErr: storeErr,
		},
		{
			name:   "Deleted when the stored observation carries a removal time",
			staged: &fsmv2.Observation[testStatus]{CollectedAt: time.Now(), Status: testStatus{V: "observed"}, DeletedAt: &deletedAt},
			want:   fsmv2client.Deleted,
		},
		{
			name:    "NotFound when nothing is stored",
			stubErr: persistence.ErrNotFound,
			want:    fsmv2client.NotFound,
		},
		{
			name:    "Stale when CollectedAt is older than maxAge",
			staged:  &fsmv2.Observation[testStatus]{CollectedAt: time.Now().Add(-3 * maxAge), Status: testStatus{V: "observed"}},
			want:    fsmv2client.Stale,
			wantObs: true,
		},
		{
			name:    "Fresh when CollectedAt is within maxAge, although the ref was never Upserted",
			staged:  &fsmv2.Observation[testStatus]{CollectedAt: time.Now().Add(-time.Second), State: "Running", Status: testStatus{V: "observed"}},
			want:    fsmv2client.Fresh,
			wantObs: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), &stubStateReader{obs: tc.staged, err: tc.stubErr})

			gotObs, got, err := fsmv2client.GetFresh[testStatus](context.Background(), client, ref, maxAge)

			if tc.wantErr == nil && err != nil {
				t.Fatalf("GetFresh returned unexpected error: %v", err)
			}

			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("GetFresh err = %v, want %v returned verbatim", err, tc.wantErr)
			}

			if got != tc.want {
				t.Fatalf("GetFresh freshness = %v, want %v", got, tc.want)
			}

			if tc.wantObs {
				if gotObs.Status != tc.staged.Status || gotObs.State != tc.staged.State || !gotObs.CollectedAt.Equal(tc.staged.CollectedAt) {
					t.Fatalf("GetFresh observation = %+v, want the staged observation %+v", gotObs, *tc.staged)
				}
			} else if gotObs.Status != (testStatus{}) || !gotObs.CollectedAt.IsZero() || gotObs.DeletedAt != nil {
				t.Fatalf("GetFresh observation = %+v, want the zero observation", gotObs)
			}
		})
	}
}

// partialDecodeReader fills the result and then fails, the way a decode error
// can leave part of an observation behind.
type partialDecodeReader struct {
	obs fsmv2.Observation[testStatus]
	err error
}

func (p *partialDecodeReader) LoadObservedTyped(_ context.Context, _, _ string, result interface{}) error {
	out, ok := result.(*fsmv2.Observation[testStatus])
	if !ok {
		return errors.New("partialDecodeReader: result is not *fsmv2.Observation[testStatus]")
	}

	*out = p.obs

	return p.err
}

func TestGetFresh_UnknownReturnsTheZeroObservation(t *testing.T) {
	decodeErr := errors.New("decode failed half-way")
	reader := &partialDecodeReader{
		obs: fsmv2.Observation[testStatus]{CollectedAt: time.Now(), Status: testStatus{V: "partial"}},
		err: decodeErr,
	}
	client := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader)
	ref := dynamicchildren.Ref{WorkerType: "transport", Name: "transport"}

	obs, freshness, err := fsmv2client.GetFresh[testStatus](context.Background(), client, ref, time.Minute)

	if freshness != fsmv2client.Unknown || !errors.Is(err, decodeErr) {
		t.Fatalf("GetFresh = (%v, %v), want (Unknown, %v)", freshness, err, decodeErr)
	}

	if obs.Status != (testStatus{}) || !obs.CollectedAt.IsZero() {
		t.Fatalf("GetFresh observation = %+v, want the zero observation", obs)
	}
}

func TestGet_ErrorReturnsTheZeroObservation(t *testing.T) {
	ref := dynamicchildren.Ref{WorkerType: "transport", Name: "transport"}
	partial := fsmv2.Observation[testStatus]{CollectedAt: time.Now(), Status: testStatus{V: "partial"}}

	for _, readErr := range []error{errors.New("decode failed half-way"), persistence.ErrNotFound} {
		client := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), &partialDecodeReader{obs: partial, err: readErr})

		obs, err := fsmv2client.Get[testStatus](context.Background(), client, ref)
		if err == nil {
			t.Fatalf("Get with read error %v returned nil error", readErr)
		}

		if obs.Status != (testStatus{}) || !obs.CollectedAt.IsZero() {
			t.Fatalf("Get with read error %v returned observation %+v, want the zero observation", readErr, obs)
		}
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
