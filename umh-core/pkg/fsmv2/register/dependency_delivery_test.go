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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// The supervisor hands a worker's dependencies to factory.NewWorkerByType, and
// register.Worker is what stands between that call and the developer's
// constructor. These specs go through both, because the closure register.Worker
// builds is the thing that used to drop the map. A spec that called the
// constructor itself would step over that closure and pass either way.

type deliveryConfig struct {
	Host string `json:"host" yaml:"host"`
}

type deliveryStatus struct {
	Reachable bool `json:"reachable"`
}

var deliveryLabelKey = config.NewDependencyKey[string]("register.test.label")

type deliveryWorker struct {
	fsmv2.WorkerBase[deliveryConfig, deliveryStatus, register.NoDeps]

	Label string
}

func (w *deliveryWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return fsmv2.NewObservation(deliveryStatus{}), nil
}

var _ = Describe("dependency delivery through register.Worker", func() {
	const workerType = "delivery-probe"

	// buildWith registers the probe worker afresh and builds one instance with
	// the dependencies a run would have handed in.
	buildWith := func(runDeps map[string]any) *deliveryWorker {
		factory.ResetRegistry()
		storage.ResetGlobalRegistry()

		register.Worker[deliveryConfig, deliveryStatus, register.NoDeps](workerType,
			func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, rd map[string]any) (fsmv2.Worker, error) {
				label, _ := config.GetDependency(rd, deliveryLabelKey)

				w := &deliveryWorker{Label: label}
				w.InitBase(id, logger, sr)

				return w, nil
			})

		w, err := factory.NewWorkerByType(workerType, deps.Identity{
			ID:         "probe-1",
			Name:       "probe",
			WorkerType: workerType,
		}, deps.NewNopFSMLogger(), nil, runDeps)
		Expect(err).NotTo(HaveOccurred())

		probe, ok := w.(*deliveryWorker)
		Expect(ok).To(BeTrue())

		return probe
	}

	It("hands the constructor the dependencies the caller supplied", func() {
		runDeps := map[string]any{}
		config.PutDependency(runDeps, deliveryLabelKey, "from-the-run")

		Expect(buildWith(runDeps).Label).To(Equal("from-the-run"))
	})

	It("hands the constructor nothing readable when the caller supplied none", func() {
		Expect(buildWith(nil).Label).To(BeEmpty())
	})
})
