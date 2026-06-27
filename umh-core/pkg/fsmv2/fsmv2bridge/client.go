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

// Package fsmv2bridge bridges benthos_monitor (and other legacy monitors) to
// the FSMv2 child-observation read path.
package fsmv2bridge

import (
	"context"
	"errors"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
)

// Freshness is the read-side reason GetFresh assigns to a child observation.
type Freshness int

const (
	// Fresh means the child was observed within maxAge.
	Fresh Freshness = iota
	// Unregistered means the ref was never Upserted into the writer.
	Unregistered
	// NeverObserved means the ref is registered but no observation exists yet.
	NeverObserved
	// Stale means the child was observed but CollectedAt is older than maxAge.
	Stale
)

// Client wraps an FSMv2Client for reads and a Writer for registration checks.
// New builds the FSMv2Client from the same Writer the Client holds, so the
// registration checks and the reads always see the same writer instance and a
// caller cannot pair a mismatched writer with the read path.
type Client struct {
	c *fsmv2client.FSMv2Client
	w *dynamicchildren.Writer
}

// New returns a Client that reads observed state through an FSMv2Client built
// from w and sr, and checks registration through w.
func New(w *dynamicchildren.Writer, sr deps.StateReader) *Client {
	return &Client{
		c: fsmv2client.NewFSMv2Client(w, sr),
		w: w,
	}
}

// GetFresh reads the observed state for ref's spawned child and maps it to a
// Freshness reason. A ref that was never Upserted is Unregistered; a registered
// ref with no persisted observation is NeverObserved; an observation older than
// maxAge is Stale; otherwise Fresh.
//
// A non-ErrNotObserved read error is returned verbatim alongside the zero
// Freshness. Callers must check err before reading Freshness or the returned
// status: both are meaningless when err is non-nil, and the returned status is
// only meaningful when Freshness is Fresh or Stale.
func GetFresh[TStatus any](ctx context.Context, c *Client, ref dynamicchildren.Ref, maxAge time.Duration) (TStatus, Freshness, error) {
	var zero TStatus

	if _, ok := c.w.Registry().Lookup(ref); !ok {
		return zero, Unregistered, nil
	}

	obs, err := fsmv2client.Get[TStatus](ctx, c.c, ref)
	if err != nil {
		if errors.Is(err, fsmv2client.ErrNotObserved) {
			return zero, NeverObserved, nil
		}

		return zero, Fresh, err
	}

	if time.Since(obs.CollectedAt) > maxAge {
		return obs.Status, Stale, nil
	}

	return obs.Status, Fresh, nil
}
