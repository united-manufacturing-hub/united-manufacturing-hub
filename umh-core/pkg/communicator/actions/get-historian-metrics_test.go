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

package actions_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("GetHistorianMetrics", func() {
	var outboundChannel chan *models.UMHMessage

	BeforeEach(func() {
		outboundChannel = make(chan *models.UMHMessage, 10)
	})

	newAction := func(cfg config.FullConfig) *actions.GetHistorianMetricsAction {
		return actions.NewGetHistorianMetricsAction(
			"test@example.com", uuid.New(), uuid.New(), outboundChannel,
			config.NewMockConfigManager().WithConfig(cfg))
	}

	unreachable := config.FullConfig{
		Historian: &config.HistorianConfig{
			Timescale: config.TimescaleConfig{
				Host:     "127.0.0.1",
				Port:     1,
				Database: "umh",
				Username: "umh_owner",
				Password: "secret",
				SSLMode:  config.HistorianSSLModeDisable,
			},
		},
	}

	It("reports a database it cannot reach instead of returning empty figures", func() {
		result, _, err := newAction(unreachable).Execute()

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Failed to read the historian database"))
		Expect(result).To(BeNil(), "a caller must not mistake a failed read for an empty database")
	})

	It("names the host it could not reach, so a misconfigured port is visible", func() {
		_, _, err := newAction(unreachable).Execute()

		Expect(err.Error()).To(ContainSubstring("127.0.0.1"))
	})

	It("fails when no historian is configured, because there is nothing to read", func() {
		_, _, err := newAction(config.FullConfig{}).Execute()

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("No historian is configured"))
	})
})
