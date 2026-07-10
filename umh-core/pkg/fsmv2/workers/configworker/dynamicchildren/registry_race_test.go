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

package dynamicchildren

import (
	"fmt"
	"sync"
	"testing"
)

// TestRegistryConcurrentAccessIsRaceFree drives the registry from many
// goroutines at once: writers Upsert/Delete refs while readers Snapshot, Specs,
// and Lookup. The shared registry is written by the config worker and read by
// the application worker's CollectObservedState on a different goroutine, so the
// mutex guarding it is load-bearing. Run under `go test -race`: a regression
// that drops the lock surfaces here as a data race, not a rare production
// `fatal error: concurrent map read and map write`.
func TestRegistryConcurrentAccessIsRaceFree(t *testing.T) {
	cw := NewWriter()
	reg := cw.Registry()

	const goroutines = 8
	const iterations = 200

	var wg sync.WaitGroup

	for w := 0; w < goroutines; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ref := Ref{WorkerType: "example", Name: fmt.Sprintf("w%d", id)}
			for i := 0; i < iterations; i++ {
				if err := cw.Upsert(ref, map[string]any{"v": i}); err != nil {
					t.Errorf("Upsert(%+v) returned error: %v", ref, err)
					return
				}
				cw.Delete(ref)
			}
		}(w)
	}

	for r := 0; r < goroutines; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			probe := Ref{WorkerType: "example", Name: "w0"}
			for i := 0; i < iterations; i++ {
				_ = reg.Snapshot()
				_ = reg.Specs()
				_, _ = reg.Lookup(probe)
			}
		}()
	}

	wg.Wait()
}
