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
	"errors"
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

func TestRegister(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Register Suite")
}

type regTestConfig struct {
	Host string `json:"host" yaml:"host"`
	Port int    `json:"port" yaml:"port"`
}

type regTestStatus struct {
	Reachable bool  `json:"reachable"`
	LatencyMs int64 `json:"latencyMs"`
}

type collidingStatus struct {
	State string `json:"state"`
}

type regTestWorker struct {
	fsmv2.WorkerBase[regTestConfig, regTestStatus]
}

func newRegTestWorker(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, _ register.NoDeps) (fsmv2.Worker, error) {
	w := &regTestWorker{}
	w.InitBase(id, logger, sr)

	return w, nil
}

func (w *regTestWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return fsmv2.NewObservation(regTestStatus{}), nil
}

var _ = Describe("register.Worker", func() {
	BeforeEach(func() {
		factory.ResetRegistry()
		storage.ResetGlobalRegistry()
	})

	It("registers successfully and factory lookup works", func() {
		register.Worker[regTestConfig, regTestStatus, register.NoDeps]("regtest", newRegTestWorker)

		types := factory.ListRegisteredTypes()
		Expect(types).To(ContainElement("regtest"))

		supervisorTypes := factory.ListSupervisorTypes()
		Expect(supervisorTypes).To(ContainElement("regtest"))

		nopLogger := deps.NewNopFSMLogger()
		worker, err := factory.NewWorkerByType("regtest", deps.Identity{
			ID:         "test-1",
			Name:       "test",
			WorkerType: "regtest",
		}, nopLogger, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(worker).NotTo(BeNil())
	})

	It("populates CSE TypeRegistry with correct types", func() {
		register.Worker[regTestConfig, regTestStatus, register.NoDeps]("regtest-cse", newRegTestWorker)

		obsType := storage.GlobalRegistry().GetObservedType("regtest-cse")
		desType := storage.GlobalRegistry().GetDesiredType("regtest-cse")

		Expect(obsType).NotTo(BeNil())
		Expect(desType).NotTo(BeNil())
		Expect(obsType).To(Equal(reflect.TypeOf(fsmv2.Observation[regTestStatus]{})))
		Expect(desType).To(Equal(reflect.TypeOf(fsmv2.WrappedDesiredState[regTestConfig]{})))
	})

	It("panics on field name collision", func() {
		Expect(func() {
			register.Worker[regTestConfig, collidingStatus, register.NoDeps]("regtest-collision", func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, _ register.NoDeps) (fsmv2.Worker, error) {
				return nil, nil
			})
		}).To(PanicWith(ContainSubstring("collide")))
	})

	It("panics on duplicate worker type", func() {
		register.Worker[regTestConfig, regTestStatus, register.NoDeps]("regtest-dup", newRegTestWorker)

		Expect(func() {
			register.Worker[regTestConfig, regTestStatus, register.NoDeps]("regtest-dup", newRegTestWorker)
		}).To(PanicWith(ContainSubstring("already registered")))
	})

	It("panics on empty worker type", func() {
		Expect(func() {
			register.Worker[regTestConfig, regTestStatus, register.NoDeps]("", newRegTestWorker)
		}).To(PanicWith(ContainSubstring("non-empty")))
	})

	It("panics on nil constructor", func() {
		Expect(func() {
			register.Worker[regTestConfig, regTestStatus, register.NoDeps]("regtest-nil", nil)
		}).To(PanicWith(ContainSubstring("non-nil")))
	})

	It("panics when constructor returns an error at factory call time", func() {
		constructorErr := errors.New("device unreachable")
		register.Worker[regTestConfig, regTestStatus, register.NoDeps]("regtest-errconstructor",
			func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ register.NoDeps) (fsmv2.Worker, error) {
				return nil, constructorErr
			},
		)

		nopLogger := deps.NewNopFSMLogger()
		Expect(func() {
			_, _ = factory.NewWorkerByType("regtest-errconstructor", deps.Identity{
				ID:         "err-1",
				Name:       "err-test",
				WorkerType: "regtest-errconstructor",
			}, nopLogger, nil)
		}).To(PanicWith(And(
			ContainSubstring("constructor failed"),
			ContainSubstring("err-1"),
		)))
	})

	It("panics when constructor returns nil worker without error", func() {
		register.Worker[regTestConfig, regTestStatus, register.NoDeps]("regtest-nilworker",
			func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ register.NoDeps) (fsmv2.Worker, error) {
				return nil, nil
			},
		)

		nopLogger := deps.NewNopFSMLogger()
		Expect(func() {
			_, _ = factory.NewWorkerByType("regtest-nilworker", deps.Identity{
				ID:         "nil-1",
				Name:       "nil-test",
				WorkerType: "regtest-nilworker",
			}, nopLogger, nil)
		}).To(PanicWith(And(
			ContainSubstring("returned nil worker"),
			ContainSubstring("nil-1"),
		)))
	})

	It("registers with TDeps generic and passes zero-value TDeps to constructor", func() {
		type regTestDeps struct {
			Marker string
		}

		var capturedDeps regTestDeps
		var constructorRan bool

		register.Worker[regTestConfig, regTestStatus, regTestDeps]("regtest-tdeps",
			func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, d regTestDeps) (fsmv2.Worker, error) {
				constructorRan = true
				capturedDeps = d
				w := &regTestWorker{}
				w.InitBase(id, logger, sr)

				return w, nil
			},
		)

		nopLogger := deps.NewNopFSMLogger()
		worker, err := factory.NewWorkerByType("regtest-tdeps", deps.Identity{
			ID:         "tdeps-1",
			Name:       "tdeps-test",
			WorkerType: "regtest-tdeps",
		}, nopLogger, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(worker).NotTo(BeNil())
		Expect(constructorRan).To(BeTrue())
		Expect(capturedDeps).To(Equal(regTestDeps{}))
	})

	It("accepts pointer TDeps and passes typed nil to constructor", func() {
		type regTestPtrDeps *struct{ X int }

		var capturedDeps regTestPtrDeps
		var constructorRan bool

		register.Worker[regTestConfig, regTestStatus, regTestPtrDeps]("regtest-ptrdeps",
			func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, d regTestPtrDeps) (fsmv2.Worker, error) {
				constructorRan = true
				capturedDeps = d
				w := &regTestWorker{}
				w.InitBase(id, logger, sr)

				return w, nil
			},
		)

		nopLogger := deps.NewNopFSMLogger()
		worker, err := factory.NewWorkerByType("regtest-ptrdeps", deps.Identity{
			ID:         "ptrdeps-1",
			Name:       "ptrdeps-test",
			WorkerType: "regtest-ptrdeps",
		}, nopLogger, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(worker).NotTo(BeNil())
		Expect(constructorRan).To(BeTrue())
		Expect(capturedDeps).To(BeNil())
	})

	It("supports NoDeps alias", func() {
		var capturedDeps register.NoDeps
		var constructorRan bool

		register.Worker[regTestConfig, regTestStatus, register.NoDeps]("regtest-nodeps-alias",
			func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, d register.NoDeps) (fsmv2.Worker, error) {
				constructorRan = true
				capturedDeps = d
				w := &regTestWorker{}
				w.InitBase(id, logger, sr)

				return w, nil
			},
		)

		nopLogger := deps.NewNopFSMLogger()
		worker, err := factory.NewWorkerByType("regtest-nodeps-alias", deps.Identity{
			ID:         "nodeps-1",
			Name:       "nodeps-test",
			WorkerType: "regtest-nodeps-alias",
		}, nopLogger, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(worker).NotTo(BeNil())
		Expect(constructorRan).To(BeTrue())
		Expect(capturedDeps).To(Equal(struct{}{}))
	})
})
