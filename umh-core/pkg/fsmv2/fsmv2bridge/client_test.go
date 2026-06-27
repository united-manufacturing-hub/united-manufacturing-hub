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

// Package fsmv2bridge_test exercises the FSMv2 child-observation read bridge as
// an external caller would, through the exported New/GetFresh seam only.
package fsmv2bridge_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2bridge"
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
		want       fsmv2bridge.Freshness
		wantStatus testStatus
	}{
		{
			name:       "Unregistered when ref was never Upserted",
			upsert:     false,
			want:       fsmv2bridge.Unregistered,
			wantStatus: testStatus{},
		},
		{
			name:       "NeverObserved when ref Upserted but store returns ErrNotFound",
			upsert:     true,
			stubErr:    persistence.ErrNotFound,
			want:       fsmv2bridge.NeverObserved,
			wantStatus: testStatus{},
		},
		{
			name:       "Stale when CollectedAt is older than maxAge",
			upsert:     true,
			collected:  time.Now().Add(-3 * maxAge),
			want:       fsmv2bridge.Stale,
			wantStatus: testStatus{V: "observed"},
		},
		{
			name:       "Fresh when CollectedAt is within maxAge",
			upsert:     true,
			collected:  time.Now().Add(-1 * time.Second),
			want:       fsmv2bridge.Fresh,
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
			client := fsmv2bridge.New(writer, stubSr)

			gotStatus, got, err := fsmv2bridge.GetFresh[testStatus](context.Background(), client, ref, maxAge)
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

// TestGetFresh_RespawnGuardServesStaleNotFresh asserts the lastEnsure respawn
// guard: after Remove + re-Ensure, GetFresh must not serve the pre-Remove
// observation as Fresh even when it is still within maxAge. The store does not
// clear a despawned child's observation (ENG-5107 is not built), so the guard
// is what prevents a stale leftover from a previous incarnation being reported
// as healthy.
func TestGetFresh_RespawnGuardServesStaleNotFresh(t *testing.T) {
	const maxAge = 10 * time.Second

	ref := dynamicchildren.Ref{WorkerType: "benthos_monitor", Name: "benthos-bridge-1"}

	writer := dynamicchildren.NewWriter()
	stubSr := &stubStateReader{}
	client := fsmv2bridge.New(writer, stubSr)

	ctx := context.Background()

	// First Ensure spawns the child. A freshly-collected observation arrives
	// AFTER the Ensure (CollectedAt is set here, post-Ensure), so the baseline
	// read is Fresh — proving the guard does not false-fire on a normal first
	// Ensure.
	if err := client.Ensure(ref, map[string]any{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	stubSr.obs = &fsmv2.Observation[testStatus]{
		CollectedAt: time.Now(),
		Status:      testStatus{V: "pre-remove"},
	}

	if _, got, err := fsmv2bridge.GetFresh[testStatus](ctx, client, ref, maxAge); err != nil {
		t.Fatalf("baseline GetFresh: %v", err)
	} else if got != fsmv2bridge.Fresh {
		t.Fatalf("baseline GetFresh = %v, want Fresh", got)
	}

	// Remove + re-Ensure. The store still holds the pre-Remove observation
	// (within maxAge), but its CollectedAt now predates the new lastEnsure.
	client.Remove(ref)

	if err := client.Ensure(ref, map[string]any{}); err != nil {
		t.Fatalf("re-Ensure: %v", err)
	}

	_, got, err := fsmv2bridge.GetFresh[testStatus](ctx, client, ref, maxAge)
	if err != nil {
		t.Fatalf("post-respawn GetFresh: %v", err)
	}

	if got == fsmv2bridge.Fresh {
		t.Fatalf("respawn guard failed: GetFresh = Fresh, want Stale (pre-Remove observation must not be served as Fresh after re-Ensure)")
	}

	if got != fsmv2bridge.Stale {
		t.Fatalf("post-respawn GetFresh = %v, want Stale", got)
	}
}

// TestSetGet_ProcessScopedAccessor asserts the process-scoped singleton: Get
// returns nil before Set and after Set(nil), and returns the published Client
// after Set. This is the seam any FSMv1 benthos manager reads via
// fsmv2bridge.Get() regardless of which manager constructed it.
func TestSetGet_ProcessScopedAccessor(t *testing.T) {
	fsmv2bridge.Set(nil)

	if got := fsmv2bridge.Get(); got != nil {
		t.Fatalf("Get before Set = %v, want nil", got)
	}

	client := fsmv2bridge.New(dynamicchildren.NewWriter(), &stubStateReader{})
	fsmv2bridge.Set(client)

	if got := fsmv2bridge.Get(); got != client {
		t.Fatalf("Get after Set = %p, want %p", got, client)
	}

	fsmv2bridge.Set(nil)

	if got := fsmv2bridge.Get(); got != nil {
		t.Fatalf("Get after Set(nil) = %v, want nil", got)
	}
}
