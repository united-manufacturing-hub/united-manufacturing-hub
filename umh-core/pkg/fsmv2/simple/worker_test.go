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

// Internal (white-box) tests: exercise the unexported worker + Register wiring.
package simple

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
)

type probeConfig struct {
	Target string `json:"target"`
}

type probeStatus struct {
	Reachable bool `json:"reachable"`
}

func newProbeWorker(spec MonitorSpec[probeConfig, probeStatus, struct{}]) (*simpleWorker[probeConfig, probeStatus, struct{}], error) {
	return newSimpleWorker(spec,
		deps.Identity{ID: "probe", WorkerType: spec.WorkerType},
		deps.NewNopFSMLogger(), nil)
}

var _ = Describe("simpleWorker", func() {
	Describe("CollectObservedState", func() {
		It("runs Poll and lands its status on the Observation", func() {
			var gotCfg probeConfig

			spec := MonitorSpec[probeConfig, probeStatus, struct{}]{
				WorkerType: "simpleworker_collect",
				Poll: func(_ context.Context, _ struct{}, cfg probeConfig) (probeStatus, error) {
					gotCfg = cfg

					return probeStatus{Reachable: true}, nil
				},
			}

			w, err := newProbeWorker(spec)
			Expect(err).NotTo(HaveOccurred())

			desired := &fsmv2.WrappedDesiredState[probeConfig]{Config: probeConfig{Target: "1.2.3.4"}}

			obs, err := w.CollectObservedState(context.Background(), desired)
			Expect(err).NotTo(HaveOccurred())
			Expect(gotCfg.Target).To(Equal("1.2.3.4"), "Poll receives the developer's config")

			o, ok := obs.(fsmv2.Observation[Status[probeStatus]])
			Expect(ok).To(BeTrue(), "observation is wrapped by the framework")
			Expect(o.Status.Result.Reachable).To(BeTrue())
		})

		It("persists a Poll error as a degraded verdict instead of returning it", func() {
			healthCalled := false

			spec := MonitorSpec[probeConfig, probeStatus, struct{}]{
				WorkerType: "simpleworker_pollerr",
				Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
					return probeStatus{}, errors.New("dial timeout")
				},
				Health: func(_ probeConfig, _ probeStatus) Health {
					healthCalled = true

					return Healthy("unreachable")
				},
			}

			w, err := newProbeWorker(spec)
			Expect(err).NotTo(HaveOccurred())

			obs, err := w.CollectObservedState(context.Background(), &fsmv2.WrappedDesiredState[probeConfig]{})
			Expect(err).NotTo(HaveOccurred(), "poll error becomes a verdict, not a returned error")

			o := obs.(fsmv2.Observation[Status[probeStatus]])
			Expect(o.Status.Degraded).To(BeTrue())
			Expect(o.Status.Reason).To(Equal("poll error: dial timeout"))
			Expect(healthCalled).To(BeFalse(), "Health is not called on a poll error")
		})

		It("stamps the Health verdict on the Observation after a good poll", func() {
			spec := MonitorSpec[probeConfig, probeStatus, struct{}]{
				WorkerType: "simpleworker_health",
				Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
					return probeStatus{Reachable: false}, nil
				},
				Health: func(_ probeConfig, s probeStatus) Health {
					if !s.Reachable {
						return Degraded("port 502 unreachable")
					}

					return Healthy("reachable")
				},
			}

			w, err := newProbeWorker(spec)
			Expect(err).NotTo(HaveOccurred())

			obs, err := w.CollectObservedState(context.Background(), &fsmv2.WrappedDesiredState[probeConfig]{})
			Expect(err).NotTo(HaveOccurred())

			o := obs.(fsmv2.Observation[Status[probeStatus]])
			Expect(o.Status.Degraded).To(BeTrue())
			Expect(o.Status.Reason).To(Equal("port 502 unreachable"))
		})

		It("defaults to healthy with a fixed reason when Health is omitted", func() {
			spec := MonitorSpec[probeConfig, probeStatus, struct{}]{
				WorkerType: "simpleworker_nohealth",
				Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
					return probeStatus{Reachable: true}, nil
				},
			}

			w, err := newProbeWorker(spec)
			Expect(err).NotTo(HaveOccurred())

			obs, err := w.CollectObservedState(context.Background(), &fsmv2.WrappedDesiredState[probeConfig]{})
			Expect(err).NotTo(HaveOccurred())

			o := obs.(fsmv2.Observation[Status[probeStatus]])
			Expect(o.Status.Degraded).To(BeFalse())
			Expect(o.Status.Reason).To(Equal("running (no health check)"))
		})

		It("passes context cancellation to Poll, surfacing it as a degraded verdict", func() {
			spec := MonitorSpec[probeConfig, probeStatus, struct{}]{
				WorkerType: "simpleworker_ctx",
				Poll: func(ctx context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
					return probeStatus{}, ctx.Err()
				},
			}

			w, err := newProbeWorker(spec)
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			obs, err := w.CollectObservedState(ctx, &fsmv2.WrappedDesiredState[probeConfig]{})
			Expect(err).NotTo(HaveOccurred())

			o := obs.(fsmv2.Observation[Status[probeStatus]])
			Expect(o.Status.Degraded).To(BeTrue())
			Expect(o.Status.Reason).To(Equal("poll error: context canceled"))
		})
	})

	Describe("NewDeps", func() {
		type probeDeps struct {
			token string
		}

		It("builds the deps from the worker's identity and passes them to Poll", func() {
			var (
				gotToken string
				gotID    deps.Identity
			)

			spec := MonitorSpec[probeConfig, probeStatus, probeDeps]{
				WorkerType: "simpleworker_newdeps",
				NewDeps: func(id deps.Identity) probeDeps {
					gotID = id

					return probeDeps{token: "token-for-" + id.ID}
				},
				Poll: func(_ context.Context, d probeDeps, _ probeConfig) (probeStatus, error) {
					gotToken = d.token

					return probeStatus{}, nil
				},
			}

			w, err := newSimpleWorker(spec,
				deps.Identity{ID: "probe", WorkerType: spec.WorkerType},
				deps.NewNopFSMLogger(), nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = w.CollectObservedState(context.Background(), &fsmv2.WrappedDesiredState[probeConfig]{})
			Expect(err).NotTo(HaveOccurred())
			Expect(gotID).To(Equal(deps.Identity{ID: "probe", WorkerType: spec.WorkerType}),
				"NewDeps receives the worker's own identity, not a zero one")
			Expect(gotToken).To(Equal("token-for-probe"),
				"Poll receives NewDeps' return, built from the identity NewDeps was handed")
		})

		It("calls NewDeps once at construction and reuses the same deps every tick", func() {
			type mutableDeps struct {
				state int
			}

			var (
				calls int
				held  *mutableDeps
			)

			spec := MonitorSpec[probeConfig, probeStatus, *mutableDeps]{
				WorkerType: "simpleworker_newdeps_persist",
				NewDeps: func(deps.Identity) *mutableDeps {
					calls++
					held = &mutableDeps{}

					return held
				},
				Poll: func(_ context.Context, d *mutableDeps, _ probeConfig) (probeStatus, error) {
					d.state++

					return probeStatus{}, nil
				},
			}

			w, err := newSimpleWorker(spec,
				deps.Identity{ID: "probe", WorkerType: spec.WorkerType},
				deps.NewNopFSMLogger(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(Equal(1),
				"NewDeps runs at construction, before any tick")

			_, err = w.CollectObservedState(context.Background(), &fsmv2.WrappedDesiredState[probeConfig]{})
			Expect(err).NotTo(HaveOccurred())

			_, err = w.CollectObservedState(context.Background(), &fsmv2.WrappedDesiredState[probeConfig]{})
			Expect(err).NotTo(HaveOccurred())

			Expect(calls).To(Equal(1),
				"NewDeps runs once per instance, not once per tick")
			Expect(held.state).To(Equal(2),
				"both ticks mutated the same NewDeps value, so state accumulates across ticks")
		})

		It("turns a panicking NewDeps into a construction error", func() {
			spec := MonitorSpec[probeConfig, probeStatus, *probeDeps]{
				WorkerType: "simpleworker_newdeps_panic",
				NewDeps: func(deps.Identity) *probeDeps {
					panic("dsn missing")
				},
				Poll: func(_ context.Context, _ *probeDeps, _ probeConfig) (probeStatus, error) {
					return probeStatus{}, nil
				},
			}

			w, err := newSimpleWorker(spec,
				deps.Identity{ID: "probe", WorkerType: spec.WorkerType},
				deps.NewNopFSMLogger(), nil)
			Expect(err).To(MatchError(ContainSubstring("dsn missing")),
				"construction runs inside the parent's tick, where an escaping panic would trip its panic circuit and suppress every sibling")
			Expect(w).To(BeNil(), "no half-built worker escapes")
		})

		It("passes the zero value to Poll when the spec builds no deps", func() {
			var (
				polled bool
				gotD   *probeDeps
			)

			// The spec deliberately declares no dependencies: their absence is the test.
			spec := MonitorSpec[probeConfig, probeStatus, *probeDeps]{
				WorkerType: "simpleworker_nodeps_unset",
				Poll: func(_ context.Context, d *probeDeps, _ probeConfig) (probeStatus, error) {
					polled = true
					gotD = d

					return probeStatus{}, nil
				},
			}

			w, err := newSimpleWorker(spec,
				deps.Identity{ID: "probe", WorkerType: spec.WorkerType},
				deps.NewNopFSMLogger(), nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = w.CollectObservedState(context.Background(), &fsmv2.WrappedDesiredState[probeConfig]{})
			Expect(err).NotTo(HaveOccurred())
			Expect(polled).To(BeTrue(),
				"Poll ran, so the nil assertion below is not vacuous")
			Expect(gotD).To(BeNil(),
				"Poll receives TDeps' zero value when the spec declares no dependencies")
		})

		It("gives every instance its own deps value, even for a repeated identity", func() {
			// One instance's state must not leak into another's. A throughput
			// window stands in for that state: the kind of per-instance mutable
			// value NewDeps exists for.
			type window struct {
				polls int
			}

			type windowStatus struct {
				Polls int `json:"polls"`
			}

			// built records one entry per NewDeps call, in call order, so the
			// assertions can name each instance's own value even when two
			// instances carry the same identity.
			var built []*window

			spec := MonitorSpec[probeConfig, windowStatus, *window]{
				WorkerType: "simpleworker_newdeps_isolation",
				NewDeps: func(deps.Identity) *window {
					w := &window{}
					built = append(built, w)

					return w
				},
				Poll: func(_ context.Context, w *window, _ probeConfig) (windowStatus, error) {
					w.polls++

					return windowStatus{Polls: w.polls}, nil
				},
			}

			newInstance := func(id deps.Identity) *simpleWorker[probeConfig, windowStatus, *window] {
				w, err := newSimpleWorker(spec, id, deps.NewNopFSMLogger(), nil)
				Expect(err).NotTo(HaveOccurred())

				return w
			}

			// pollCount ticks one instance and returns the poll count its own deps
			// value reported, which is how a developer's code observes that state.
			pollCount := func(w *simpleWorker[probeConfig, windowStatus, *window]) int {
				obs, err := w.CollectObservedState(context.Background(), &fsmv2.WrappedDesiredState[probeConfig]{})
				Expect(err).NotTo(HaveOccurred())

				o, ok := obs.(fsmv2.Observation[Status[windowStatus]])
				Expect(ok).To(BeTrue(), "observation is wrapped by the framework")

				return o.Status.Result.Polls
			}

			// The supervisor builds a child's ID as "<name>-001", so two parents
			// that give their child the same name hand out the same ID and Name,
			// and only the hierarchy path tells the two children apart.
			idA := deps.Identity{
				ID:            "monitor-001",
				Name:          "monitor",
				WorkerType:    spec.WorkerType,
				HierarchyPath: "parent-a(parent)/monitor-001(" + spec.WorkerType + ")",
			}
			idB := idA
			idB.HierarchyPath = "parent-b(parent)/monitor-001(" + spec.WorkerType + ")"

			// aAgain is idA a second time: a child respawned under its old name
			// must not inherit the dead instance's state.
			a, b, aAgain := newInstance(idA), newInstance(idB), newInstance(idA)

			Expect(built).To(HaveLen(3),
				"NewDeps runs once per instance, including for an instance whose identity another instance already used")
			Expect(built[0]).NotTo(BeIdenticalTo(built[1]),
				"two instances that differ only in hierarchy path get separate deps values")
			Expect(built[0]).NotTo(BeIdenticalTo(built[2]),
				"a respawned instance gets a fresh deps value, not the previous instance's")

			Expect(pollCount(a)).To(Equal(1), "a's first poll mutates a's deps value")
			Expect(pollCount(a)).To(Equal(2), "a keeps the same deps value across ticks")

			Expect(built[0].polls).To(Equal(2),
				"a's polls landed on the value NewDeps built for a")
			Expect(built[1].polls).To(BeZero(), "a's two polls left b's deps value untouched")
			Expect(built[2].polls).To(BeZero(), "a's two polls left the respawn's deps value untouched")

			Expect(pollCount(b)).To(Equal(1),
				"b counts its first poll as its first, unaffected by a's two polls")
			Expect(built[1].polls).To(Equal(1), "b's poll landed on b's own deps value")

			Expect(pollCount(aAgain)).To(Equal(1),
				"the respawn starts from zero, not from a's count")
			Expect(built[2].polls).To(Equal(1), "the respawn's poll landed on its own deps value")
			Expect(built[0].polls).To(Equal(2), "neither b nor the respawn touched a's deps value")
		})
	})

	Describe("dependencies", func() {
		It("reports a true-nil GetDependenciesAny so metrics injection is not skipped", func() {
			w, err := newProbeWorker(MonitorSpec[probeConfig, probeStatus, struct{}]{
				WorkerType: "simpleworker_deps",
				Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
					return probeStatus{}, nil
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(w.GetDependenciesAny()).To(BeNil())
		})
	})
})

var _ = Describe("Register", func() {
	AfterEach(func() {
		fsmv2.ResetInitialStateRegistry()
	})

	It("panics when WorkerType is empty", func() {
		Expect(func() {
			Register(MonitorSpec[probeConfig, probeStatus, struct{}]{
				Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
					return probeStatus{}, nil
				},
			})
		}).To(PanicWith(ContainSubstring("WorkerType")))
	})

	It("panics when Poll is nil", func() {
		Expect(func() {
			Register(MonitorSpec[probeConfig, probeStatus, struct{}]{WorkerType: "simpleworker_nopoll"})
		}).To(PanicWith(ContainSubstring("Poll")))
	})

	It("panics when TStatus is not a struct", func() {
		Expect(func() {
			Register(MonitorSpec[probeConfig, map[string]any, struct{}]{
				WorkerType: "simpleworker_mapstatus",
				Poll: func(_ context.Context, _ struct{}, _ probeConfig) (map[string]any, error) {
					return nil, nil
				},
			})
		}).To(PanicWith(ContainSubstring("struct")))
	})

	It("registers an initial state for the worker type", func() {
		Register(MonitorSpec[probeConfig, probeStatus, struct{}]{
			WorkerType: "simpleworker_register",
			Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
				return probeStatus{}, nil
			},
		})
		Expect(fsmv2.LookupInitialState("simpleworker_register")).NotTo(BeNil())
	})

	It("records MonitorSpec.Interval as the worker type's collection cadence", func() {
		Register(MonitorSpec[probeConfig, probeStatus, struct{}]{
			WorkerType: "simpleworker_interval",
			Interval:   5 * time.Second,
			Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
				return probeStatus{}, nil
			},
		})

		got, ok := fsmv2.ObservationIntervalFor("simpleworker_interval")
		Expect(ok).To(BeTrue())
		Expect(got).To(Equal(5 * time.Second))
	})

	It("leaves the cadence unset when Interval is zero so the collector defaults", func() {
		Register(MonitorSpec[probeConfig, probeStatus, struct{}]{
			WorkerType: "simpleworker_nointerval",
			Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
				return probeStatus{}, nil
			},
		})

		_, ok := fsmv2.ObservationIntervalFor("simpleworker_nointerval")
		Expect(ok).To(BeFalse())
	})

	It("gives every instance built through the registered factory its own deps value", func() {
		// Production never calls newSimpleWorker: it reaches the constructor
		// Register stored, through the factory, once per worker instance. A poll
		// counter stands in for the per-instance mutable state NewDeps exists for,
		// so each deps value can be traced back to the instance that received it.
		type window struct {
			polls int
		}

		const workerType = "simpleworker_register_factory"

		var built []*window

		Register(MonitorSpec[probeConfig, probeStatus, *window]{
			WorkerType: workerType,
			NewDeps: func(deps.Identity) *window {
				w := &window{}
				built = append(built, w)

				return w
			},
			Poll: func(_ context.Context, w *window, _ probeConfig) (probeStatus, error) {
				w.polls++

				return probeStatus{}, nil
			},
		})

		// The supervisor builds a child's ID as "<name>-001", so two children
		// given the same name under different parents share ID and Name, and only
		// the hierarchy path tells them apart.
		idA := deps.Identity{
			ID:            "monitor-001",
			Name:          "monitor",
			WorkerType:    workerType,
			HierarchyPath: "parent-a(parent)/monitor-001(" + workerType + ")",
		}
		idB := idA
		idB.HierarchyPath = "parent-b(parent)/monitor-001(" + workerType + ")"

		newInstance := func(id deps.Identity) fsmv2.Worker {
			w, err := factory.NewWorkerByType(workerType, id, deps.NewNopFSMLogger(), nil, nil)
			Expect(err).NotTo(HaveOccurred(), "Register left an instantiable factory for the worker type")
			Expect(w).NotTo(BeNil(), "an instance exists, so the assertions below are not vacuous")

			return w
		}

		a, b := newInstance(idA), newInstance(idB)
		Expect(a).NotTo(BeIdenticalTo(b),
			"the factory builds a worker per instantiation, not one memoised per worker type")

		Expect(built).To(HaveLen(2),
			"NewDeps runs once per instance built through the factory, not once per worker type")
		Expect(built[0]).NotTo(BeIdenticalTo(built[1]),
			"two instances that differ only in hierarchy path get separate deps values")

		_, err := a.CollectObservedState(context.Background(), &fsmv2.WrappedDesiredState[probeConfig]{})
		Expect(err).NotTo(HaveOccurred())
		Expect(built[0].polls).To(Equal(1), "a's poll landed on the value NewDeps built for a")
		Expect(built[1].polls).To(BeZero(), "a's poll left b's deps value untouched")
	})
})
