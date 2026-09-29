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

package fakebox

import (
	"context"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// HangingFS wraps a filesystem.Service and can hold up reads of one path until
// a caller releases them. A Box can serve, change or refuse a file, but it
// cannot make a read block.
//
// Wrap it outside any lock the inner service's reads take, or a hung read
// holds that lock too. Unlike a Box, a HangingFS is safe for concurrent use.
type HangingFS struct {
	filesystem.Service

	// mu guards hung against the collector's reads and a scenario's Hang
	// calls.
	mu   sync.Mutex
	hung map[string]chan struct{}
}

// NewHangingFS returns a HangingFS serving inner. Until Hang is called, every
// read passes straight through to it.
func NewHangingFS(inner filesystem.Service) *HangingFS {
	return &HangingFS{Service: inner, hung: map[string]chan struct{}{}}
}

// Hang holds up every read of exactly path until the returned release runs.
// Reads of other paths, and reads of path after the release, pass through to
// the inner service. The returned release is idempotent, so a deferred copy
// and a copy the scenario calls by hand may both run.
func (h *HangingFS) Hang(path string) (release func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	released := make(chan struct{})
	h.hung[path] = released

	var once sync.Once

	return func() {
		once.Do(func() { close(released) })
	}
}

// ReadFile blocks while a Hang holds the path, then serves the inner service.
//
// The read's ctx is ignored while the read is held. The collector cancels a
// read at its ObservationTimeout (supervisor.CollectorHealthConfig); a read
// that returned then would save a poll-error reading, and the reading would
// never go Stale.
func (h *HangingFS) ReadFile(ctx context.Context, path string) ([]byte, error) {
	h.mu.Lock()
	released, hung := h.hung[path]
	h.mu.Unlock()

	if hung {
		<-released
	}

	return h.Service.ReadFile(ctx, path)
}
