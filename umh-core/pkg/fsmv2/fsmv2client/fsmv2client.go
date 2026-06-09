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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// FSMv2Client delegates child-spec writes to the ConfigWorker it wraps. It also
// holds a read-only StateReader for the typed Get added in a later rung; the
// client stores it but does not read it yet.
type FSMv2Client struct {
	cw *configworker.ConfigWorker
	sr deps.StateReader
}

// NewFSMv2Client returns an FSMv2Client that writes through cw and holds sr for
// a later typed Get. sr is stored only; the client does not read it yet.
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
