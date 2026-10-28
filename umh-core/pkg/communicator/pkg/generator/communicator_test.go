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

package generator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/generator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

var _ = Describe("DeriveCoreHealth", func() {
	derive := func(communicatorHealth *models.Health) *models.Health {
		return generator.DeriveCoreHealth(nil, nil, nil, nil, nil, communicatorHealth, nil, zap.NewNop().Sugar())
	}

	It("degrades the core when the communicator is degraded", func() {
		health := derive(&models.Health{Category: models.Degraded, Message: "Communicator status is stale"})

		Expect(health.Category).To(Equal(models.Degraded))
		Expect(health.Message).To(ContainSubstring("Communicator: Communicator status is stale"))
	})

	DescribeTable("keeps the core active when the communicator is not degraded",
		func(communicatorHealth *models.Health) {
			Expect(derive(communicatorHealth).Category).To(Equal(models.Active))
		},
		Entry("active", &models.Health{Category: models.Active}),
		Entry("neutral", &models.Health{Category: models.Neutral}),
		Entry("not reported", nil),
	)
})
