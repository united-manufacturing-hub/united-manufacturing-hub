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
	"errors"
	"sync"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("GetHistorian", func() {
	var (
		action          *actions.GetHistorianAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		messages        []*models.UMHMessage
		mu              sync.Mutex
	)

	BeforeEach(func() {
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10)

		mockConfig = config.NewMockConfigManager().WithConfig(config.FullConfig{
			Historian: &config.HistorianConfig{
				Host:     "timescale.example.com",
				Password: "secret",
				Port:     5432,
				Database: "umh",
				Username: "umh_owner",
				SSLMode:  config.HistorianSSLModeRequire,
			},
		})

		action = actions.NewGetHistorianAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig)

		messages = nil
		go actions.ConsumeOutboundMessages(outboundChannel, &messages, &mu, true)
	})

	AfterEach(func() {
		for len(outboundChannel) > 0 {
			<-outboundChannel
		}
		close(outboundChannel)
	})

	Describe("Parse and Validate", func() {
		It("should accept an empty payload", func() {
			Expect(action.Parse(nil)).To(Succeed())
			Expect(action.Validate()).To(Succeed())
		})
	})

	Describe("Execute", func() {
		It("should return the current historian config", func() {
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			cfg, ok := result.(config.HistorianConfig)
			Expect(ok).To(BeTrue(), "Result should be a HistorianConfig")
			Expect(cfg.Host).To(Equal("timescale.example.com"))
			Expect(cfg.Port).To(Equal(uint16(5432)))
			Expect(cfg.Database).To(Equal("umh"))
			Expect(cfg.SSLMode).To(Equal(config.HistorianSSLModeRequire))

			// A successful read emits no reply of its own; the dispatcher sends the sole
			// terminal ActionFinishedSuccessfull carrying the returned config.
			Consistently(func() []models.ActionReplyState {
				return historianReplyStates(&messages, &mu)
			}).Should(BeEmpty())
		})

		It("should fail when no historian is configured", func() {
			mockConfig.WithConfig(config.FullConfig{})

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not configured"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// The not-configured branch must return a non-nil error so the dispatcher
			// short-circuits; otherwise it would append a spurious success reply after
			// this failure.
			Eventually(func() []models.ActionReplyState {
				return historianReplyStates(&messages, &mu)
			}).Should(Equal([]models.ActionReplyState{models.ActionFinishedWithFailure}))
		})

		It("should fail when the config cannot be read", func() {
			mockConfig.WithConfigError(errors.New("mock read failure"))

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to read configuration"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			Eventually(func() []models.ActionReplyState {
				return historianReplyStates(&messages, &mu)
			}).Should(Equal([]models.ActionReplyState{models.ActionFinishedWithFailure}))
		})
	})
})
