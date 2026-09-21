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
	"context"
	"errors"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/historian/timescalemetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("GetHistorianMetrics", func() {
	var (
		outboundChannel chan *models.UMHMessage
		configured      config.FullConfig
	)

	BeforeEach(func() {
		outboundChannel = make(chan *models.UMHMessage, 10)
		configured = config.FullConfig{
			Historian: &config.HistorianConfig{
				Timescale: config.TimescaleConfig{
					Host:     "timescale.example.com",
					Password: "secret",
					Port:     5432,
					Database: "umh",
					Username: "umh_owner",
					SSLMode:  config.HistorianSSLModeRequire,
				},
			},
		}
	})

	newAction := func(cfg config.FullConfig, collect actions.HistorianMetricsCollector) *actions.GetHistorianMetricsAction {
		action := actions.NewGetHistorianMetricsAction(
			"test@example.com", uuid.New(), uuid.New(), outboundChannel,
			config.NewMockConfigManager().WithConfig(cfg))
		action.SetCollector(collect)

		return action
	}

	It("returns the figures the collector read from the database", func() {
		collected := timescalemetrics.Metrics{
			TimescaleVersion: "2.24.0",
			DatabaseBytes:    961000000,
			Hypertables:      4,
			Tables:           []timescalemetrics.Table{{Name: "value_pump", Chunks: 105}},
		}
		action := newAction(configured, func(context.Context, string) (timescalemetrics.Metrics, error) {
			return collected, nil
		})

		result, _, err := action.Execute()

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(collected))
	})

	It("dials the host the historian config names", func() {
		var dialled string
		action := newAction(configured, func(_ context.Context, dsn string) (timescalemetrics.Metrics, error) {
			dialled = dsn

			return timescalemetrics.Metrics{}, nil
		})

		_, _, err := action.Execute()

		Expect(err).NotTo(HaveOccurred())
		Expect(dialled).To(ContainSubstring("timescale.example.com"))
	})

	It("reports a collection failure instead of returning empty figures", func() {
		action := newAction(configured, func(context.Context, string) (timescalemetrics.Metrics, error) {
			return timescalemetrics.Metrics{}, errors.New("permission denied for schema umh")
		})

		_, _, err := action.Execute()

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("permission denied"))
	})

	It("fails when no historian is configured, because there is nothing to read", func() {
		action := newAction(config.FullConfig{}, func(context.Context, string) (timescalemetrics.Metrics, error) {
			Fail("the collector must not run without a configured historian")

			return timescalemetrics.Metrics{}, nil
		})

		_, _, err := action.Execute()

		Expect(err).To(HaveOccurred())
	})
})
