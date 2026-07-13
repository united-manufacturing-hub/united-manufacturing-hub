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
		consumerDone    chan struct{}
	)

	BeforeEach(func() {
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10)

		mockConfig = config.NewMockConfigManager().WithConfig(config.FullConfig{
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
		})

		action = actions.NewGetHistorianAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig)

		messages = nil
		consumerDone = make(chan struct{})
		go func() {
			defer GinkgoRecover()
			actions.ConsumeOutboundMessages(outboundChannel, &messages, &mu, true)
			close(consumerDone)
		}()
	})

	AfterEach(func() {
		// Close the channel and wait for the consumer to drain and exit before the
		// next spec resets `messages`, so the reset cannot race a running consumer.
		close(outboundChannel)
		<-consumerDone
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
			Expect(cfg.Timescale).NotTo(Equal(config.TimescaleConfig{}))
			Expect(cfg.Timescale.Host).To(Equal("timescale.example.com"))
			Expect(cfg.Timescale.Port).To(Equal(uint16(5432)))
			Expect(cfg.Timescale.Database).To(Equal("umh"))
			Expect(cfg.Timescale.SSLMode).To(Equal(config.HistorianSSLModeRequire))
			// The password is write-only: get-historian never returns it, so it
			// cannot leak through reply logs, the cloud backend, or the browser.
			Expect(cfg.Timescale.Password).To(BeEmpty())

			// A successful read emits no reply of its own; the dispatcher sends the sole
			// terminal ActionFinishedSuccessfull carrying the returned config.
			Consistently(func() []models.ActionReplyState {
				return historianReplyStates(&messages, &mu)
			}).Should(BeEmpty())
		})

		It("should succeed with an absent result when no historian is configured", func() {
			mockConfig.WithConfig(config.FullConfig{})

			result, metadata, err := action.Execute()
			// A missing historian is the normal empty state, not a failure: Execute
			// succeeds with a nil result so the dispatcher sends a success reply
			// carrying a null historian, and the frontend need not string-match errors.
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// Execute emits no reply of its own; the dispatcher sends the sole terminal
			// ActionFinishedSuccessfull.
			Consistently(func() []models.ActionReplyState {
				return historianReplyStates(&messages, &mu)
			}).Should(BeEmpty())
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
