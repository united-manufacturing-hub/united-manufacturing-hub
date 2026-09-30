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
// a Writer for writes and reads workers' observations from the store.
//
// # Dynamic and static workers
//
// FSMv2 runs each component as a worker, and workers form a tree. The
// application worker is the root. A worker's observation is what it last
// reported; the store keeps it.
//
// Dynamic workers exist only at the root. They are the application worker's
// direct children, added and removed at runtime through Upsert and Delete.
// The CPU monitor and the historian monitor are dynamic workers.
//
// Every worker below them is static. Its parent declares it in code, in the
// parent's list of child specs, and the parent's supervisor starts and stops
// it. The communicator's transport worker, and the push and pull workers
// under it, are static workers. Upsert and Delete never add or remove a
// static worker.
//
// Get and GetFresh read the store, not the Upsert list. So they read a static
// worker's observation the same way as a dynamic worker's. A Ref names either
// kind: its WorkerType, and its Name as the parent declared it.
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

// ErrNotFound reports that nothing is stored for the ref: the worker has not
// started yet, or the ref names no worker. It is distinct from a decode or
// transient store failure, so a caller can treat absence as "appears on a
// later tick" without swallowing a real read error.
var ErrNotFound = errors.New("fsmv2client: nothing stored for ref")

// ErrWorkerDeleted reports that the ref's worker was removed. The store keeps
// the removed worker's last observation as history, but Get does not return
// it. Match it with errors.Is; the error Get returns is a *WorkerDeletedError.
var ErrWorkerDeleted = errors.New("fsmv2client: worker was removed")

// WorkerDeletedError is the error Get returns for a removed worker. It carries
// when the supervisor removed it.
type WorkerDeletedError struct {
	Ref       dynamicchildren.Ref
	DeletedAt time.Time
}

func (e *WorkerDeletedError) Error() string {
	return fmt.Sprintf("%s: %s/%s at %s", ErrWorkerDeleted, e.Ref.WorkerType, config.ChildID(e.Ref.Name), e.DeletedAt.Format(time.RFC3339))
}

// Is makes errors.Is(err, ErrWorkerDeleted) match a *WorkerDeletedError.
func (e *WorkerDeletedError) Is(target error) bool {
	return target == ErrWorkerDeleted
}

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
// and the child id is config.ChildID(ref.Name). When nothing is stored for the
// ref it returns ErrNotFound. When the worker was removed it returns a
// *WorkerDeletedError, which matches ErrWorkerDeleted, and the zero
// observation. Any other reader error is returned verbatim. In each error
// case the observation is the zero value.
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
			return obs, fmt.Errorf("%w: %s/%s", ErrNotFound, ref.WorkerType, config.ChildID(ref.Name))
		}

		return obs, err
	}

	if obs.DeletedAt != nil {
		return fsmv2.Observation[TStatus]{}, &WorkerDeletedError{Ref: ref, DeletedAt: *obs.DeletedAt}
	}

	return obs, nil
}

// Freshness says what GetFresh found for a ref. GetFresh checks the values in
// the order listed here, and the first that applies wins.
type Freshness int

const (
	// Unknown means the read failed, so nothing can be decided. GetFresh
	// returns the error alongside. It is the zero value, so an unclassified
	// result never reads as healthy.
	Unknown Freshness = iota
	// Deleted means the supervisor removed the worker. The store keeps its
	// last observation as history, but GetFresh does not return it.
	Deleted
	// NotFound means nothing is stored for the ref: the worker has not started
	// yet, or the ref names no worker.
	NotFound
	// Stale means an observation exists and is older than maxAge. An
	// observation with a zero CollectedAt is Stale.
	Stale
	// Fresh means an observation exists and is at most maxAge old.
	Fresh
)

// freshnessAt classifies a Get result, with the clock as a parameter so tests
// can pin the age boundary. It returns the error only for Unknown.
func freshnessAt[TStatus any](obs fsmv2.Observation[TStatus], err error, maxAge time.Duration, now time.Time) (Freshness, error) {
	switch {
	case errors.Is(err, ErrWorkerDeleted):
		return Deleted, nil
	case errors.Is(err, ErrNotFound):
		return NotFound, nil
	case err != nil:
		return Unknown, err
	case now.Sub(obs.CollectedAt) > maxAge:
		return Stale, nil
	default:
		return Fresh, nil
	}
}

// GetFresh is Get plus a freshness check. It reads ref's observation with Get
// and says, as a Freshness value, whether it can be used. It returns the
// observation only for Fresh and Stale, and the zero observation otherwise.
// It returns a non-nil error only with Unknown.
//
// GetFresh reads the store only. It does not check whether the ref was
// Upserted, so it works for static workers too (see the package doc).
//
// Between Delete and the supervisor removing the worker, the worker still
// runs for a few ticks. In that window GetFresh classifies its latest
// observation by age, usually as Fresh.
//
// The store read is bounded by ctx; callers SHOULD pass a deadline-bounded
// ctx (see the StateReader non-blocking contract).
func GetFresh[TStatus any](ctx context.Context, c *FSMv2Client, ref dynamicchildren.Ref, maxAge time.Duration) (fsmv2.Observation[TStatus], Freshness, error) {
	obs, err := Get[TStatus](ctx, c, ref)

	freshness, err := freshnessAt(obs, err, maxAge, time.Now())
	if freshness != Fresh && freshness != Stale {
		return fsmv2.Observation[TStatus]{}, freshness, err
	}

	return obs, freshness, nil
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
