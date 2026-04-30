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

package register_test

import (
	"context"
	"strconv"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// depsRegistryConfig is a minimal TConfig type used by the registry tests. It
// is distinct from regTestConfig in register_test.go so the two test files can
// share the same ginkgo suite without worker-type cross-talk.
type depsRegistryConfig struct {
	Tag string `json:"tag" yaml:"tag"`
}

type depsRegistryStatus struct {
	Ready bool `json:"ready"`
}

// depsRegistryDeps is a concrete pointer-shaped TDeps used to exercise I2
// (pointer-nil fidelity). Fields are intentionally exported so the factory
// integration test can assert round-trip identity.
type depsRegistryDeps struct {
	Label string
	Value int
}

type depsRegistryWorker struct {
	fsmv2.WorkerBase[depsRegistryConfig, depsRegistryStatus]
	receivedDeps *depsRegistryDeps
}

func (w *depsRegistryWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return fsmv2.NewObservation(depsRegistryStatus{}), nil
}

var _ = Describe("register deps registry", func() {
	BeforeEach(func() {
		register.ResetRegistry()
	})

	AfterEach(func() {
		register.ResetRegistry()
	})

	Context("CRUD (I5 bullet 1)", func() {
		It("SetDeps stores, GetDeps reads, ClearDeps removes", func() {
			d := &depsRegistryDeps{Label: "primary", Value: 7}

			register.SetDeps[*depsRegistryDeps]("crud-worker", d)
			Expect(register.GetDeps[*depsRegistryDeps]("crud-worker")).To(BeIdenticalTo(d))

			register.ClearDeps("crud-worker")
			Expect(register.GetDeps[*depsRegistryDeps]("crud-worker")).To(BeNil())
		})

		It("ResetRegistry clears all published keys", func() {
			register.SetDeps[*depsRegistryDeps]("reset-a", &depsRegistryDeps{Label: "a"})
			register.SetDeps[*depsRegistryDeps]("reset-b", &depsRegistryDeps{Label: "b"})

			register.ResetRegistry()

			Expect(register.GetDeps[*depsRegistryDeps]("reset-a")).To(BeNil())
			Expect(register.GetDeps[*depsRegistryDeps]("reset-b")).To(BeNil())
		})
	})

	Context("round-trip identity (I5 bullet 2)", func() {
		It("returns the exact same pointer instance for pointer TDeps", func() {
			original := &depsRegistryDeps{Label: "identity", Value: 42}

			register.SetDeps[*depsRegistryDeps]("roundtrip-ptr", original)
			got := register.GetDeps[*depsRegistryDeps]("roundtrip-ptr")

			Expect(got).To(BeIdenticalTo(original))
		})

		It("round-trips value-type TDeps by value", func() {
			type valueDeps struct{ N int }

			register.SetDeps[valueDeps]("roundtrip-val", valueDeps{N: 99})
			got := register.GetDeps[valueDeps]("roundtrip-val")

			Expect(got).To(Equal(valueDeps{N: 99}))
		})
	})

	Context("zero-value fallback (I5 bullet 3 — I1/I2)", func() {
		It("returns Go-native nil for pointer TDeps when never SetDeps", func() {
			got := register.GetDeps[*depsRegistryDeps]("never-set")

			// Direct nil comparison — not reflect.DeepEqual — verifies there
			// is no typed-nil-in-interface oddity from the map[string]any path.
			Expect(got == nil).To(BeTrue())
		})

		It("returns struct{} zero value for register.NoDeps when never SetDeps", func() {
			got := register.GetDeps[register.NoDeps]("never-set-nodeps")

			Expect(got).To(Equal(register.NoDeps{}))
		})

		It("returns value-type zero for value TDeps when never SetDeps", func() {
			type zeroCheck struct {
				S string
				I int
			}

			got := register.GetDeps[zeroCheck]("never-set-value")

			Expect(got).To(Equal(zeroCheck{}))
		})

		It("returns nil interface for interface-typed TDeps when never SetDeps", func() {
			type someIface interface{ Ping() }

			got := register.GetDeps[someIface]("never-set-iface")

			Expect(got == nil).To(BeTrue())
		})
	})

	Context("concurrent Set/Get (I5 bullet 4 — I4)", func() {
		It("is race-free under concurrent goroutines", func() {
			const goroutines = 50
			const keysPerGoroutine = 20

			var wg sync.WaitGroup
			wg.Add(goroutines * 2)

			for g := 0; g < goroutines; g++ {
				go func(id int) {
					defer wg.Done()

					for k := 0; k < keysPerGoroutine; k++ {
						key := "concurrent-writer-" + strconv.Itoa(id) + "-" + strconv.Itoa(k)
						register.SetDeps[*depsRegistryDeps](key, &depsRegistryDeps{
							Label: key,
							Value: id*100 + k,
						})
					}
				}(g)

				go func(id int) {
					defer wg.Done()

					for k := 0; k < keysPerGoroutine; k++ {
						key := "concurrent-reader-" + strconv.Itoa(id) + "-" + strconv.Itoa(k)
						_ = register.GetDeps[*depsRegistryDeps](key)
					}
				}(g)
			}

			wg.Wait()
		})
	})

	Context("factory-closure integration (I5 bullet 5)", func() {
		BeforeEach(func() {
			factory.ResetRegistry()
			storage.ResetGlobalRegistry()
		})

		It("SetDeps publishes deps that reach the user constructor via the factory closure", func() {
			const workerType = "depsreg-factory-positive"

			published := &depsRegistryDeps{Label: "from-parent-wiring", Value: 123}

			var capturedDeps *depsRegistryDeps
			var constructorRan bool

			register.Worker[depsRegistryConfig, depsRegistryStatus, *depsRegistryDeps](workerType,
				func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, d *depsRegistryDeps) (fsmv2.Worker, error) {
					constructorRan = true
					capturedDeps = d
					w := &depsRegistryWorker{receivedDeps: d}
					w.InitBase(id, logger, sr)

					return w, nil
				},
			)

			// Publish deps BEFORE the factory call, mirroring how parent
			// wiring (cmd/main.go) will use the registry.
			register.SetDeps[*depsRegistryDeps](workerType, published)

			nopLogger := deps.NewNopFSMLogger()
			w, err := factory.NewWorkerByType(workerType, deps.Identity{
				ID:         "dr-factory-1",
				Name:       "dr-factory-1",
				WorkerType: workerType,
			}, nopLogger, nil, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(w).NotTo(BeNil())
			Expect(constructorRan).To(BeTrue())
			Expect(capturedDeps).To(BeIdenticalTo(published))
		})

		It("constructor receives typed nil when SetDeps is never called (I3 regression guard)", func() {
			const workerType = "depsreg-factory-nildefault"

			var capturedDeps *depsRegistryDeps
			var constructorRan bool

			register.Worker[depsRegistryConfig, depsRegistryStatus, *depsRegistryDeps](workerType,
				func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, d *depsRegistryDeps) (fsmv2.Worker, error) {
					constructorRan = true
					capturedDeps = d
					w := &depsRegistryWorker{receivedDeps: d}
					w.InitBase(id, logger, sr)

					return w, nil
				},
			)

			// Deliberately DO NOT call SetDeps — mirrors today's PR2 workers
			// (transport/push/pull/persistence) that rely on singletons.

			nopLogger := deps.NewNopFSMLogger()
			w, err := factory.NewWorkerByType(workerType, deps.Identity{
				ID:         "dr-factory-nil-1",
				Name:       "dr-factory-nil-1",
				WorkerType: workerType,
			}, nopLogger, nil, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(w).NotTo(BeNil())
			Expect(constructorRan).To(BeTrue())
			// Direct nil comparison — I2 invariant.
			Expect(capturedDeps == nil).To(BeTrue())
		})
	})
})

