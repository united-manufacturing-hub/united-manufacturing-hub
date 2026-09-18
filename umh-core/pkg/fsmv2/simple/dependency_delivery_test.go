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

package simple

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
)

// A monitor worker does not reach its dependencies the way every other worker
// does. Register builds its own constructor closure and hands that to
// register.Worker, so the run's dependency map has a second place it can be
// dropped -- and dropping it there excludes every monitor worker, cpu,
// historian and nmap among them, while the framework looks like it delivers.
//
// These specs go through Register and the factory rather than calling
// newSimpleWorker, because that closure is the thing under test. A spec that
// called the constructor itself would step over it and pass either way.

var deliveryLabelKey = config.NewDependencyKey[string]("simple.test.label")

type deliveryDeps struct {
	label string
}

var _ = Describe("dependency delivery to a monitor worker", func() {
	// buildWith registers a monitor worker under its own type name and builds
	// one instance with the dependencies a run would have handed in. Each spec
	// needs its own name: Register also publishes an initial state, and that
	// registry has no reset.
	buildWith := func(workerType string, runDeps map[string]any) *deliveryDeps {
		var built *deliveryDeps

		Register(MonitorSpec[probeConfig, probeStatus, *deliveryDeps]{
			WorkerType: workerType,
			NewDeps: func(_ deps.Identity, _ *deps.BaseDependencies, rd map[string]any) *deliveryDeps {
				label, _ := config.GetDependency(rd, deliveryLabelKey)
				built = &deliveryDeps{label: label}

				return built
			},
			Poll: func(context.Context, *deliveryDeps, probeConfig) (probeStatus, error) {
				return probeStatus{}, nil
			},
		})

		w, err := factory.NewWorkerByType(workerType, deps.Identity{
			ID:         "monitor-001",
			Name:       "monitor",
			WorkerType: workerType,
		}, deps.NewNopFSMLogger(), nil, runDeps)
		Expect(err).NotTo(HaveOccurred())
		Expect(w).NotTo(BeNil(), "an instance exists, so the assertion below is not vacuous")
		Expect(built).NotTo(BeNil(), "NewDeps ran")

		return built
	}

	It("hands NewDeps the dependencies the run supplied", func() {
		runDeps := map[string]any{}
		config.PutDependency(runDeps, deliveryLabelKey, "from-the-run")

		Expect(buildWith("simpleworker_delivery_supplied", runDeps).label).To(Equal("from-the-run"))
	})

	It("hands NewDeps nothing readable when the run supplied none", func() {
		Expect(buildWith("simpleworker_delivery_none", nil).label).To(BeEmpty())
	})
})
