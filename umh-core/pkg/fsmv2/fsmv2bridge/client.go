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
	"sync"
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

	mu         sync.Mutex
	lastEnsure map[dynamicchildren.Ref]time.Time
}

// New returns a Client that reads observed state through an FSMv2Client built
// from w and sr, and checks registration through w.
func New(w *dynamicchildren.Writer, sr deps.StateReader) *Client {
	return &Client{
		c:          fsmv2client.NewFSMv2Client(w, sr),
		w:          w,
		lastEnsure: make(map[dynamicchildren.Ref]time.Time),
	}
}

// Ensure Upserts the child spec for ref and records the Ensure time as the
// ref's generation marker. GetFresh treats any observation whose CollectedAt
// predates the most recent Ensure as Stale, so a leftover observation from a
// previous incarnation (after Remove + re-Ensure) is never served as Fresh.
func (c *Client) Ensure(ref dynamicchildren.Ref, spec map[string]any) error {
	if err := c.c.Upsert(ref, spec); err != nil {
		return err
	}

	c.mu.Lock()
	c.lastEnsure[ref] = time.Now()
	c.mu.Unlock()

	return nil
}

// Remove deletes the child spec for ref and drops its generation marker.
func (c *Client) Remove(ref dynamicchildren.Ref) {
	c.c.Delete(ref)

	c.mu.Lock()
	delete(c.lastEnsure, ref)
	c.mu.Unlock()
}

// ensureGeneration returns the most recent Ensure time for ref, or the zero
// time if Ensure was never called for it (e.g. the ref was registered directly
// via the writer).
func (c *Client) ensureGeneration(ref dynamicchildren.Ref) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.lastEnsure[ref]
}

// globalClient is the process-scoped Client published once at startup so any
// FSMv1 component (regardless of which benthos manager constructed it) can
// reach the FSMv2 child-observation bridge via Get. NewBenthosManager is built
// at three independent sites, so threading the handle through a single
// constructor would miss most instances; a process-scoped accessor is the only
// thing that reaches them all.
var (
	globalMu  sync.RWMutex
	globalCli *Client
)

// Set publishes c as the process-scoped Client. Pass nil to clear it (e.g. on
// shutdown). Not safe for concurrent re-publication; call once at startup and
// once on shutdown.
func Set(c *Client) {
	globalMu.Lock()

	globalCli = c

	globalMu.Unlock()
}

// Get returns the process-scoped Client, or nil if Set has not been called (or
// was cleared). FF-off paths never call Set, so Get returns nil and callers
// must treat nil as "FSMv2 bridge unavailable".
func Get() *Client {
	globalMu.RLock()
	defer globalMu.RUnlock()

	return globalCli
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

	// Respawn guard: an observation older than the most recent Ensure is from a
	// previous incarnation (the store does not clear a despawned child's
	// observation). Serve it as Stale so a leftover is never reported Fresh.
	if obs.CollectedAt.Before(c.ensureGeneration(ref)) {
		return obs.Status, Stale, nil
	}

	if time.Since(obs.CollectedAt) > maxAge {
		return obs.Status, Stale, nil
	}

	return obs.Status, Fresh, nil
}
