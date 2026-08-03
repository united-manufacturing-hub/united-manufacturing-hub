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

// Every registered worker type gets framework metrics on its Observation,
// whatever its TDeps and whatever GetDependenciesAny returns. The collector
// injects framework metrics from its own locals before the deps guard, so a
// struct{}-deps worker (nmap) carries them like any other.
//
// The registry is populated by package init(). A missing blank import leaves it
// empty, so the HaveLen(15) floor below is what makes a forgotten import fail
// loudly instead of passing trivially; a 16th registration also reddens it.

package collection_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/collection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"

	// Named imports for the config types these workers' DesiredState requires;
	// the import itself triggers init(), which is the registration these package
	// names contribute to the factory registry.
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic"
	// helloworld and exampleslow declare their package names as hello_world and
	// example_slow respectively; the aliases below match the config type names.
	exampleslow "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow"
	helloworld "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"

	// Blank imports populate the factory registry via init(). If any of these is
	// forgotten, ListRegisteredTypes() returns fewer types and the HaveLen(15)
	// floor below fails loudly instead of letting the test pass trivially.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/historian"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/nmap"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
)

// registeredFloor is the hard lower bound on the number of registered worker
// types. factory.ListRegisteredTypes() is populated by package init(), so a
// test that forgets a blank import returns zero types and passes trivially;
// asserting an exact count catches both a missing import (returns 0) and a new
// 16th registration (returns 16). Measured at this HEAD: exactly 15, and the
// registered historian type is "historian-timescale", not "historian".
const registeredFloor = 15

// panicOnConstruction names the five types that panic rather than error when
// built via factory.NewWorkerByType in this isolated test process, because
// register.Worker wraps constructor failure in a panic. Each constructor's
// dependency is published by a parent or the transport channel singleton that
// this test does not wire up. They are skipped below with the stated reason, and
// the recovered panic is asserted to match that reason, so a skip cannot hide a
// constructor regression inside one of them.
var panicOnConstruction = map[string]string{
	"communicator": "ChannelProvider must be set",
	"transport":    "ChannelProvider must be set",
	"persistence":  "requires a store",
	"pull":         "deps builder returned",
	"push":         "deps builder returned",
}

// targetWorkers are the three workers the design exists to fix: they are the
// types whose Observation lacked framework metrics before this change
// (application and configworker return nil deps, nmap returns struct{}). They
// are named explicitly here rather than relying on the enumeration to reach
// them silently.
var targetWorkers = []string{"application", "configworker", "nmap"}

type observedFrameworkProbe struct {
	Metrics deps.MetricsContainer `json:"metrics"`
}

// desiredProviderFor returns a DesiredStateProvider that yields the concrete
// desired type each worker's CollectObservedState expects. Workers that call
// fsmv2.ExtractConfig panic if handed the wrong WrappedDesiredState, so the
// provider must match per type; workers that ignore the desired state accept
// the generic DesiredState.
func desiredProviderFor(workerType string) func() (fsmv2.DesiredState, error) {
	generic := func() (fsmv2.DesiredState, error) {
		return &fsmv2config.DesiredState{BaseDesiredState: fsmv2config.BaseDesiredState{}}, nil
	}

	switch workerType {
	case "helloworld":
		return func() (fsmv2.DesiredState, error) {
			return &fsmv2.WrappedDesiredState[helloworld.HelloworldConfig]{Config: helloworld.HelloworldConfig{}}, nil
		}
	case "examplepanic":
		return func() (fsmv2.DesiredState, error) {
			return &fsmv2.WrappedDesiredState[examplepanic.ExamplepanicConfig]{Config: examplepanic.ExamplepanicConfig{}}, nil
		}
	case "examplefailing":
		return func() (fsmv2.DesiredState, error) {
			return &fsmv2.WrappedDesiredState[examplefailing.ExamplefailingConfig]{Config: examplefailing.ExamplefailingConfig{}}, nil
		}
	case "exampleslow":
		return func() (fsmv2.DesiredState, error) {
			return &fsmv2.WrappedDesiredState[exampleslow.ExampleslowConfig]{Config: exampleslow.ExampleslowConfig{}}, nil
		}
	case "nmap":
		return func() (fsmv2.DesiredState, error) {
			return &fsmv2.WrappedDesiredState[config.NmapConfig]{Config: config.NmapConfig{}}, nil
		}
	case "historian-timescale":
		return func() (fsmv2.DesiredState, error) {
			return &fsmv2.WrappedDesiredState[config.HistorianConfig]{Config: config.HistorianConfig{}}, nil
		}
	default:
		return generic
	}
}

// constructWorker builds the worker for workerType through the real factory
// registry, recovering any construction panic (register.Worker wraps
// constructor failure in a panic). It never lets the panic escape; the caller
// decides whether a recovered panic is a named skip or an unexpected failure.
func constructWorker(workerType string) (fsmv2.Worker, interface{}) {
	id := deps.Identity{ID: workerType + "-probe", WorkerType: workerType, Name: workerType + "-probe"}

	var (
		worker fsmv2.Worker
		panick interface{}
	)

	func() {
		defer func() {
			if r := recover(); r != nil {
				panick = r
			}
		}()

		w, err := factory.NewWorkerByType(workerType, id, deps.NewNopFSMLogger(), nil, nil)
		if err != nil {
			panick = fmt.Sprintf("NewWorkerByType(%q): %v", workerType, err)
			return
		}
		worker = w
	}()

	return worker, panick
}

// probeFrameworkMetrics drives one real synchronous collection tick through the
// real collector, loading the saved Observation back out of the real store, and
// returns the framework TimeInCurrentStateMs on it. The provider
// injects sentinel into the collector's FrameworkMetricsProvider; recovering
// exactly sentinel back proves framework metrics are present on the
// Observation, not merely that the worker produced an observation.
//
// TObserved is fsmv2.ObservedState (the interface): every worker's
// CollectObservedState returns the interface, so observed.(fsmv2.ObservedState)
// always holds, and the generic save path marshals the underlying concrete
// Observation, the same non-generic path the collector uses in production.
//
// This assumes the worker is a NewObservation worker (zero CollectedAt), so the
// collector's zero-time gate fires and framework metrics are injected. All ten
// buildable types are NewObservation workers today; a migration to WrapStatus
// would fail this probe with a sentinel mismatch rather than a clear message.
func probeFrameworkMetrics(workerType string, w fsmv2.Worker, sentinel int64) int64 {
	id := deps.Identity{ID: workerType + "-probe", WorkerType: workerType, Name: workerType + "-probe"}
	store := supervisor.CreateTestTriangularStoreForWorkerType(workerType)

	cfg := collection.CollectorConfig[fsmv2.ObservedState]{
		Worker:               w,
		Identity:             id,
		Store:                store,
		Logger:               deps.NewNopFSMLogger(),
		ObservationInterval:  time.Hour, // No background ticks; exactly one synchronous tick below.
		ObservationTimeout:   3 * time.Second,
		DesiredStateProvider: desiredProviderFor(workerType),
		FrameworkMetricsProvider: func() *deps.FrameworkMetrics {
			return &deps.FrameworkMetrics{TimeInCurrentStateMs: sentinel}
		},
	}

	c := collection.NewCollector[fsmv2.ObservedState](cfg)

	ctx := context.Background()
	Expect(c.Start(ctx)).To(Succeed())
	defer c.Stop(context.Background())
	Expect(c.CollectFinalObservation(context.Background())).To(Succeed())

	var probe observedFrameworkProbe
	Expect(store.LoadObservedTyped(ctx, workerType, id.ID, &probe)).To(Succeed())

	return probe.Metrics.Framework.TimeInCurrentStateMs
}

var _ = Describe("Framework metrics on every registered worker type", func() {
	It("enumerates exactly the registered worker types (hard floor: catches a missing blank import or a 16th type)", func() {
		Expect(factory.ListRegisteredTypes()).To(HaveLen(registeredFloor))
	})

	It("attaches framework metrics to the Observation of every buildable registered type", func() {
		// configworker is the one buildable worker whose constructor first needs
		// one line of setup: a published dynamic-children registry. Without it
		// the constructor fails (wrapped in a panic). Its ConfigManager is
		// optional, so nil is fine.
		register.SetDeps[*dynamicchildren.Registry]("configworker", &dynamicchildren.Registry{})

		types := factory.ListRegisteredTypes()
		Expect(types).To(HaveLen(registeredFloor))

		covered := map[string]int64{}
		skipped := map[string]interface{}{}

		for _, workerType := range types {
			worker, panick := constructWorker(workerType)
			if panick != nil {
				reason, expected := panicOnConstruction[workerType]
				if !expected {
					// A construction panic we did not anticipate must fail loudly,
					// not be swallowed as a skip.
					Fail(fmt.Sprintf("unexpected construction panic for %q: %v", workerType, panick))
				}

				// The recovered panic must match the expected dependency-absence
				// cause, so a constructor regression inside one of these workers
				// cannot hide behind a silent skip.
				Expect(fmt.Sprint(panick)).To(ContainSubstring(reason),
					"worker %q panicked for an unexpected reason (want %q); actual: %q", workerType, reason, fmt.Sprint(panick))

				skipped[workerType] = reason

				continue
			}

			if worker == nil {
				Fail(fmt.Sprintf("construction of %q produced a nil worker", workerType))
			}

			Expect(worker).NotTo(BeNil(), "construction of %q produced a nil worker", workerType)

			// Deterministic, type-distinct sentinel: proves the exact injected value
			// is present, not merely that some metric is non-zero.
			sentinel := int64(900000) + int64(len(workerType))
			Expect(probeFrameworkMetrics(workerType, worker, sentinel)).To(Equal(sentinel),
				"framework TimeInCurrentStateMs sentinel is not present on %q's Observation", workerType)

			covered[workerType] = sentinel
		}

		// The two expectation halves must agree: all five dependency-bound types
		// were skipped, and each was skipped for the reason this test states.
		Expect(skipped).To(HaveLen(len(panicOnConstruction)))
		for workerType, reason := range panicOnConstruction {
			Expect(skipped).To(HaveKey(workerType), "expected %q to be skipped (%s)", workerType, reason)
		}

		// The three workers this design exists to fix must carry framework
		// metrics, named explicitly rather than trusting the enumeration.
		for _, workerType := range targetWorkers {
			Expect(covered).To(HaveKey(workerType),
				"target worker %q did not carry framework metrics on its Observation", workerType)
		}

		// The other seven buildable types prove the property generalises: ten
		// covered in total (all fifteen registered, minus the five skipped).
		Expect(covered).To(HaveLen(registeredFloor - len(panicOnConstruction)))
	})
})
