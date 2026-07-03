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

// Package fsmv2client exposes the migration-API seam: a thin client that wraps
// a Writer for writes.
package fsmv2client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// ErrNotObserved reports that no observed state exists for the ref: the child
// was never spawned, or the collector has not yet persisted its first
// observation. It is distinct from a decode or transient store failure, so a
// caller can treat absence as "appears on a later tick" without swallowing a
// real read error.
var ErrNotObserved = errors.New("fsmv2client: ref not observed")

// FSMv2Client delegates child-spec writes to the Writer it wraps and
// reads child observed state through the read-only StateReader it holds (see
// the Get function).
type FSMv2Client struct {
	w  *dynamicchildren.Writer
	sr deps.StateReader
}

// NewFSMv2Client returns an FSMv2Client that writes through w and reads
// observed state through sr. The client deliberately holds the plain Writer,
// never the supervisor-managed config worker instance: worker instances can be
// torn down and recreated, so a held instance would go stale after the first
// restart.
func NewFSMv2Client(w *dynamicchildren.Writer, sr deps.StateReader) *FSMv2Client {
	return &FSMv2Client{w: w, sr: sr}
}

// Upsert records cfg for ref in the wrapped Writer. Validation errors return
// synchronously from this call; callers rely on rejecting a bad spec at the
// call site. When spec writes move into the config worker's tick (ENG-4400),
// this client is the layer that absorbs the change, preserving or
// renegotiating the synchronous error contract.
func (c *FSMv2Client) Upsert(ref dynamicchildren.Ref, cfg map[string]any) error {
	return c.w.Upsert(ref, cfg)
}

// Delete removes ref from the wrapped Writer.
func (c *FSMv2Client) Delete(ref dynamicchildren.Ref) {
	c.w.Delete(ref)
}

// Get reads the observed state the collector persisted for ref's spawned child
// and returns it as an Observation[TStatus]. The collection is ref.WorkerType
// and the child id is config.ChildID(ref.Name). When no observed state exists
// for the ref it returns ErrNotObserved, so an unobserved ref surfaces as a
// recognizable not-found error a caller can distinguish from a decode or
// transient store failure, rather than as a zero-value observation. Any other
// reader error is returned verbatim.
//
// Get does not verify that TStatus matches ref.WorkerType. Pairing a TStatus
// that does not match the worker type decodes whatever fields overlap and is
// the caller's responsibility; the worker registry that would enforce the
// pairing is not wired here.
func Get[TStatus any](ctx context.Context, c *FSMv2Client, ref dynamicchildren.Ref) (fsmv2.Observation[TStatus], error) {
	var obs fsmv2.Observation[TStatus]

	// A write-only client (built with a nil StateReader) has no read path. Return
	// an error rather than dereferencing a nil reader, so the caller sees a
	// diagnosable failure instead of a panic.
	if c == nil || c.sr == nil {
		return obs, fmt.Errorf("fsmv2client: Get requires a client with a StateReader (ref %s/%s)", ref.WorkerType, config.ChildID(ref.Name))
	}

	if err := c.sr.LoadObservedTyped(ctx, ref.WorkerType, config.ChildID(ref.Name), &obs); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return obs, fmt.Errorf("%w: %s/%s", ErrNotObserved, ref.WorkerType, config.ChildID(ref.Name))
		}

		return obs, err
	}

	return obs, nil
}

// Freshness is the read-side reason GetFresh assigns to a child observation.
// It lets a caller map an absent or stale read to a distinct recovery policy
// instead of collapsing every non-fresh case into a single error.
type Freshness int

const (
	// Unknown is the zero value of Freshness. It is returned when a read error
	// prevents GetFresh from classifying the observation, so the healthy reason
	// Fresh is never the default. Freshness is only meaningful when the
	// accompanying error is nil.
	Unknown Freshness = iota
	// Fresh means the child was observed within maxAge.
	Fresh
	// Unregistered means the ref was never Upserted into the writer.
	Unregistered
	// NeverObserved means the ref is registered but no observation exists yet.
	NeverObserved
	// Stale means the child was observed but CollectedAt is older than maxAge
	// (the watcher is wedged or slow).
	Stale
)

// GetFresh reads the observed state for ref's spawned child and maps it to a
// Freshness reason. A ref that was never Upserted is Unregistered; a registered
// ref with no persisted observation is NeverObserved; an observation older than
// maxAge is Stale; otherwise Fresh.
//
// A non-ErrNotObserved read error is returned verbatim alongside the Unknown
// Freshness. Callers must check err before reading Freshness or the returned
// status: both are meaningless when err is non-nil, and the returned status is
// only meaningful when Freshness is Fresh or Stale.
//
// The store read is bounded by the passed ctx; callers SHOULD pass a
// deadline-bounded ctx (see the StateReader non-blocking contract).
//
// GetFresh does NOT detect a stale observation left over from a previous
// incarnation after Delete + re-Upsert: the CSE store does not clear a
// despawned child's observation until ENG-5107 (store-side despawn tombstone)
// lands. Until then, such a leftover within maxAge is served as Fresh. When
// ENG-5107 lands, the store will return a typed ErrWorkerDeleted on a
// despawned ref; Get/GetFresh must then map ErrWorkerDeleted to NeverObserved
// (today Get only maps persistence.ErrNotFound → ErrNotObserved, so a
// tombstone read would currently surface as Unknown+err, not NeverObserved).
func GetFresh[TStatus any](ctx context.Context, c *FSMv2Client, ref dynamicchildren.Ref, maxAge time.Duration) (TStatus, Freshness, error) {
	var zero TStatus

	if !c.w.Registry().Contains(ref) {
		return zero, Unregistered, nil
	}

	obs, err := Get[TStatus](ctx, c, ref)
	if err != nil {
		if errors.Is(err, ErrNotObserved) {
			return zero, NeverObserved, nil
		}

		return zero, Unknown, err
	}

	if time.Since(obs.CollectedAt) > maxAge {
		return obs.Status, Stale, nil
	}

	return obs.Status, Fresh, nil
}

// GetFreshObs mirrors GetFresh but returns the full fsmv2.Observation[TStatus]
// instead of just its Status, so callers see the framework fields (the
// Degraded/Reason health verdict and CollectedAt) alongside the Freshness
// classification. The Freshness ladder is identical to GetFresh: a ref that was
// never Upserted is Unregistered; a registered ref with no persisted
// observation is NeverObserved; an observation older than maxAge is Stale;
// otherwise Fresh.
//
// A non-ErrNotObserved read error is returned verbatim alongside the Unknown
// Freshness. Callers must check err before reading Freshness or the returned
// Observation: both are meaningless when err is non-nil, and the returned
// Observation is only meaningful when Freshness is Fresh or Stale.
//
// The store read is bounded by the passed ctx; callers SHOULD pass a
// deadline-bounded ctx (see the StateReader non-blocking contract). The
// ENG-5107 stale-tombstone caveat documented on GetFresh applies identically.
func GetFreshObs[TStatus any](ctx context.Context, c *FSMv2Client, ref dynamicchildren.Ref, maxAge time.Duration) (fsmv2.Observation[TStatus], Freshness, error) {
	var zero fsmv2.Observation[TStatus]

	if !c.w.Registry().Contains(ref) {
		return zero, Unregistered, nil
	}

	obs, err := Get[TStatus](ctx, c, ref)
	if err != nil {
		if errors.Is(err, ErrNotObserved) {
			return zero, NeverObserved, nil
		}

		return zero, Unknown, err
	}

	if time.Since(obs.CollectedAt) > maxAge {
		return obs, Stale, nil
	}

	return obs, Fresh, nil
}

// globalCli is the process-scoped FSMv2Client published once at startup so any
// FSMv1 component (regardless of which benthos manager constructed it) can
// reach the FSMv2 child-observation read path via GetClient. NewBenthosManager
// is built at three independent sites, so threading the handle through a single
// constructor would miss most instances; a process-scoped accessor is the only
// thing that reaches them all.
var (
	globalMu  sync.RWMutex
	globalCli *FSMv2Client
)

// SetClient publishes c as the process-scoped FSMv2Client. Pass nil to clear it
// (e.g. on shutdown). Not safe for concurrent re-publication; call once at
// startup and once on shutdown.
func SetClient(c *FSMv2Client) {
	globalMu.Lock()

	globalCli = c

	globalMu.Unlock()
}

// GetClient returns the process-scoped FSMv2Client, or nil if SetClient has not
// been called (or was cleared). FF-off paths never call SetClient, so GetClient
// returns nil and callers must treat nil as "FSMv2 client unavailable".
func GetClient() *FSMv2Client {
	globalMu.RLock()
	defer globalMu.RUnlock()

	return globalCli
}
