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
// torn down and respawned, so a held instance would go stale after the first
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
