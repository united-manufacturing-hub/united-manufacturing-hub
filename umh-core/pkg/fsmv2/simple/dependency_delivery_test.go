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

// These specs build through Register and factory.NewWorkerByType, because the
// constructor closure Register builds is what hands the map to newSimpleWorker.
// A spec that called newSimpleWorker directly would pass even with that closure
// broken.

var deliveryLabelKey = config.NewDependencyKey[string]("simple.test.label")

type deliveryDeps struct {
	label string
}

var _ = Describe("dependency delivery to a monitor worker", func() {
	// Each spec passes its own workerType, because Register panics on a worker type
	// that is already registered.
	registerAndBuild := func(workerType string, dependencies map[string]any) *deliveryDeps {
		var built *deliveryDeps

		Register(MonitorSpec[probeConfig, probeStatus, *deliveryDeps]{
			WorkerType: workerType,
			NewDeps: func(_ deps.Identity, _ *deps.BaseDependencies, rd map[string]any) *deliveryDeps {
				label, _ := config.LookupDependency(rd, deliveryLabelKey)
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
		}, deps.NewNopFSMLogger(), nil, dependencies)
		Expect(err).NotTo(HaveOccurred())
		Expect(w).NotTo(BeNil(), "an instance exists, so the assertion below is not vacuous")
		Expect(built).NotTo(BeNil(), "NewDeps ran")

		return built
	}

	It("hands NewDeps the dependencies the run supplied", func() {
		dependencies := map[string]any{}
		config.SetDependency(dependencies, deliveryLabelKey, "from-the-run")

		Expect(registerAndBuild("simpleworker_delivery_supplied", dependencies).label).To(Equal("from-the-run"))
	})

	It("hands NewDeps nothing readable when the run supplied none", func() {
		Expect(registerAndBuild("simpleworker_delivery_none", nil).label).To(BeEmpty())
	})
})
