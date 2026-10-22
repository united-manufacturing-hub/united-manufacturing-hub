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

package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var (
	deliveryParentKey  = config.NewDependencyKey[string]("supervisor.test.parent")
	deliveryChildKey   = config.NewDependencyKey[string]("supervisor.test.child")
	deliveryRestartKey = config.NewDependencyKey[string]("supervisor.test.restart")
)

const (
	deliveryChildType   = "delivery-child"
	deliveryRestartType = "delivery-restart"
)

func registerDeliveryType(workerType string, record func(map[string]any)) {
	Expect(factory.RegisterFactoryByType(workerType, func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, dependencies map[string]any) fsmv2.Worker {
		record(dependencies)

		state := &mockState{}
		state.nextState = state

		return &mockWorker{initialState: state}
	})).To(Succeed())

	Expect(factory.RegisterSupervisorFactoryByType(workerType, func(cfg interface{}) interface{} {
		return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg.(supervisor.Config))
	})).To(Succeed())
}

var _ = Describe("dependency delivery from the supervisor", func() {
	It("hands a child its parent's dependencies merged with its own spec's", func() {
		var delivered map[string]any
		registerDeliveryType(deliveryChildType, func(d map[string]any) { delivered = d })

		parentDeps := map[string]any{}
		config.SetDependency(parentDeps, deliveryParentKey, "from-parent")

		childDeps := map[string]any{}
		config.SetDependency(childDeps, deliveryChildKey, "from-child-spec")

		store := newMockTriangularStore()
		parent := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:   "delivery-parent",
			Logger:       deps.NewNopFSMLogger(),
			Store:        store,
			Dependencies: parentDeps,
		})

		identity := deps.Identity{ID: "parent-worker", Name: "Parent Worker", WorkerType: "delivery-parent"}
		Expect(parent.AddWorker(identity, &hierarchicalWorker{
			id:     identity.ID,
			logger: newTickLogger(),
			observed: &mockObservedState{
				ID:          identity.ID,
				CollectedAt: time.Now(),
				Desired:     &mockDesiredState{},
			},
			childrenSpecs: []config.ChildSpec{{
				Name:         "child1",
				WorkerType:   deliveryChildType,
				UserSpec:     config.UserSpec{Config: "child-config"},
				Dependencies: childDeps,
			}},
		})).To(Succeed())

		parent.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

		_, err := store.SaveDesired(context.Background(), "delivery-parent", identity.ID, persistence.Document{
			"id":                identity.ID,
			"ShutdownRequested": false,
		})
		Expect(err).NotTo(HaveOccurred())

		store.Observed["delivery-parent"] = map[string]interface{}{
			identity.ID: persistence.Document{"id": identity.ID, "collectedAt": time.Now()},
		}

		Expect(parent.TestTick(context.Background())).To(Succeed())
		Expect(delivered).NotTo(BeNil(), "the child was built, so the assertions below are not vacuous")

		fromParent, ok := config.LookupDependency(delivered, deliveryParentKey)
		Expect(ok).To(BeTrue(), "the parent's dependency reached the child")
		Expect(fromParent).To(Equal("from-parent"))

		fromSpec, ok := config.LookupDependency(delivered, deliveryChildKey)
		Expect(ok).To(BeTrue(), "the child spec's own dependency reached the child")
		Expect(fromSpec).To(Equal("from-child-spec"))
	})

	It("hands a restarted worker its supervisor's dependencies", func() {
		var delivered map[string]any
		registerDeliveryType(deliveryRestartType, func(d map[string]any) { delivered = d })

		supervisorDeps := map[string]any{}
		config.SetDependency(supervisorDeps, deliveryRestartKey, "from-supervisor")

		ctx := context.Background()
		store := newMockTriangularStore()
		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              deliveryRestartType,
			Logger:                  deps.NewNopFSMLogger(),
			Store:                   store,
			GracefulShutdownTimeout: 100 * time.Millisecond,
			Dependencies:            supervisorDeps,
		})

		// processSignal rebuilds a worker that signals removal while pending restart.
		stopped := &mockState{signal: fsmv2.SignalNeedsRemoval}
		stopped.nextState = stopped

		identity := deps.Identity{ID: "restart-worker", Name: "Restart Worker", WorkerType: deliveryRestartType}
		Expect(s.AddWorker(identity, &mockWorker{initialState: stopped})).To(Succeed())

		_, err := store.SaveDesired(ctx, deliveryRestartType, identity.ID, persistence.Document{
			"id":                identity.ID,
			"ShutdownRequested": false,
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = store.SaveObserved(ctx, deliveryRestartType, identity.ID, persistence.Document{
			"id":          identity.ID,
			"collectedAt": time.Now(),
			"desired":     persistence.Document{"ShutdownReq": false},
		})
		Expect(err).NotTo(HaveOccurred())

		s.TestSetPendingRestart(identity.ID)
		s.TestMarkAsStarted()

		Expect(s.TestTick(ctx)).To(Succeed())
		Expect(delivered).NotTo(BeNil(), "the worker was rebuilt, so the assertions below are not vacuous")

		fromSupervisor, ok := config.LookupDependency(delivered, deliveryRestartKey)
		Expect(ok).To(BeTrue(), "the supervisor's dependency reached the rebuilt worker")
		Expect(fromSupervisor).To(Equal("from-supervisor"))
	})
})
