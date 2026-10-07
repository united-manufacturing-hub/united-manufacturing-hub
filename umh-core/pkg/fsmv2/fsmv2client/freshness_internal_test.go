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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
)

func TestFreshnessAt_ClassifiesGetResult(t *testing.T) {
	const maxAge = 10 * time.Second

	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	fresh := fsmv2.Observation[struct{}]{CollectedAt: now.Add(-time.Second)}
	atMaxAge := fsmv2.Observation[struct{}]{CollectedAt: now.Add(-maxAge)}
	stale := fsmv2.Observation[struct{}]{CollectedAt: now.Add(-maxAge - time.Nanosecond)}
	zeroCollectedAt := fsmv2.Observation[struct{}]{}
	notFound := fmt.Errorf("%w: transport/transport-001", ErrNotFound)
	deleted := &WorkerDeletedError{Ref: dynamicchildren.Ref{WorkerType: "transport", Name: "transport"}, DeletedAt: now}
	storeErr := errors.New("store failure")

	cases := []struct {
		name    string
		obs     fsmv2.Observation[struct{}]
		err     error
		want    Freshness
		wantErr error
	}{
		{"another error is Unknown and returned", fresh, storeErr, Unknown, storeErr},
		{"another error wins over a stale observation", stale, storeErr, Unknown, storeErr},
		{"a removed worker is Deleted", zeroCollectedAt, deleted, Deleted, nil},
		{"a wrapped removed-worker error is Deleted", zeroCollectedAt, fmt.Errorf("read: %w", deleted), Deleted, nil},
		{"nothing stored is NotFound", zeroCollectedAt, notFound, NotFound, nil},
		{"ErrNotFound wins over a fresh observation", fresh, ErrNotFound, NotFound, nil},
		{"age exactly maxAge is Fresh", atMaxAge, nil, Fresh, nil},
		{"age maxAge plus 1ns is Stale", stale, nil, Stale, nil},
		{"zero CollectedAt is Stale", zeroCollectedAt, nil, Stale, nil},
		{"CollectedAt after now is Fresh", fsmv2.Observation[struct{}]{CollectedAt: now.Add(time.Minute)}, nil, Fresh, nil},
		{"age under maxAge is Fresh", fresh, nil, Fresh, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := freshnessAt(tc.obs, tc.err, maxAge, now)
			if got != tc.want {
				t.Fatalf("freshness = %v, want %v", got, tc.want)
			}

			if tc.wantErr == nil && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestFreshnessAt_NonPositiveMaxAgeIsStale(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	obs := fsmv2.Observation[struct{}]{CollectedAt: now.Add(-time.Nanosecond)}

	for _, maxAge := range []time.Duration{0, -time.Second} {
		got, err := freshnessAt(obs, nil, maxAge, now)
		if err != nil || got != Stale {
			t.Fatalf("maxAge %v: freshness = %v, err = %v, want Stale and nil", maxAge, got, err)
		}
	}
}

func TestFreshness_UnknownIsTheZeroValue(t *testing.T) {
	var f Freshness
	if f != Unknown {
		t.Fatalf("zero Freshness = %v, want Unknown", f)
	}
}
