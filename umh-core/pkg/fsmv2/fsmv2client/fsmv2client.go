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
// a ConfigWorker for writes.
package fsmv2client

import (
	"context"
	"errors"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// ErrNotObserved reports that no observed state exists for the ref: the child
// was never spawned, or the collector has not yet persisted its first
// observation. It is distinct from a decode or transient store failure, so a
// caller can treat absence as "appears on a later tick" without swallowing a
// real read error.
var ErrNotObserved = errors.New("fsmv2client: ref not observed")

// FSMv2Client delegates child-spec writes to the ConfigWorker it wraps and
// reads child observed state through the read-only StateReader it holds (see
// the Get function).
type FSMv2Client struct {
	cw *configworker.ConfigWorker
	sr deps.StateReader
}

// NewFSMv2Client returns an FSMv2Client that writes through cw and reads
// observed state through sr.
func NewFSMv2Client(cw *configworker.ConfigWorker, sr deps.StateReader) *FSMv2Client {
	return &FSMv2Client{cw: cw, sr: sr}
}

// Upsert records cfg for ref in the wrapped ConfigWorker.
func (c *FSMv2Client) Upsert(ref configworker.Ref, cfg map[string]any) error {
	return c.cw.Upsert(ref, cfg)
}

// Delete removes ref from the wrapped ConfigWorker.
func (c *FSMv2Client) Delete(ref configworker.Ref) {
	c.cw.Delete(ref)
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
func Get[TStatus any](ctx context.Context, c *FSMv2Client, ref configworker.Ref) (fsmv2.Observation[TStatus], error) {
	var obs fsmv2.Observation[TStatus]

	if err := c.sr.LoadObservedTyped(ctx, ref.WorkerType, config.ChildID(ref.Name), &obs); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return obs, fmt.Errorf("%w: %s/%s", ErrNotObserved, ref.WorkerType, config.ChildID(ref.Name))
		}

		return obs, err
	}

	return obs, nil
}
