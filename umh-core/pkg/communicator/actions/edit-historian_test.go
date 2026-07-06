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

var _ = Describe("EditHistorian", func() {
	var (
		action          *actions.EditHistorianAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		messages        []*models.UMHMessage
		mu              sync.Mutex
	)

	validPayload := func() map[string]interface{} {
		return map[string]interface{}{
			"host":     "new-host.example.com",
			"password": "new-secret",
			"port":     6543,
		}
	}

	// configWithHistorian returns a config that already has a historian section.
	configWithHistorian := func() config.FullConfig {
		return config.FullConfig{
			Historian: &config.HistorianConfig{
				Host:     "old-host.example.com",
				Password: "old-secret",
			},
		}
	}

	BeforeEach(func() {
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10)

		mockConfig = config.NewMockConfigManager().WithConfig(configWithHistorian())

		action = actions.NewEditHistorianAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig)

		messages = nil
		go actions.ConsumeOutboundMessages(outboundChannel, &messages, &mu, true)
	})

	AfterEach(func() {
		for len(outboundChannel) > 0 {
			<-outboundChannel
		}
		close(outboundChannel)
	})

	Describe("Validate", func() {
		It("should fail validation when required fields are missing", func() {
			payload := validPayload()
			delete(payload, "password")

			Expect(action.Parse(payload)).To(Succeed())

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field password"))
		})
	})

	Describe("Execute", func() {
		It("should overwrite an existing historian config", func() {
			Expect(action.Parse(validPayload())).To(Succeed())

			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			Expect(mockConfig.AtomicEditHistorianCalled).To(BeTrue())

			cfg, ok := result.(config.HistorianConfig)
			Expect(ok).To(BeTrue(), "Result should be a HistorianConfig")
			Expect(cfg.Host).To(Equal("new-host.example.com"))
			Expect(cfg.Port).To(Equal(uint16(6543)))

			Expect(mockConfig.Config.Historian.Host).To(Equal("new-host.example.com"))

			// Execute emits only the progress replies; the terminal success reply is
			// the dispatcher's job. A self-sent ActionFinishedSuccessfull here would be
			// the double-reply regression.
			Eventually(func() []models.ActionReplyState {
				return historianReplyStates(&messages, &mu)
			}).Should(Equal([]models.ActionReplyState{models.ActionConfirmed, models.ActionExecuting}))
		})

		It("should fail when no historian is configured yet", func() {
			mockConfig.WithConfig(config.FullConfig{})

			Expect(action.Parse(validPayload())).To(Succeed())

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("use deploy-historian to create it first"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			Expect(mockConfig.AtomicEditHistorianCalled).To(BeTrue())
			Expect(mockConfig.Config.Historian).To(BeNil())

			Eventually(func() []models.ActionReplyState {
				return historianReplyStates(&messages, &mu)
			}).Should(Equal([]models.ActionReplyState{models.ActionConfirmed, models.ActionExecuting, models.ActionFinishedWithFailure}))
		})

		It("should handle AtomicEditHistorian failure", func() {
			mockConfig.WithAtomicEditHistorianError(errors.New("mock write failure"))

			Expect(action.Parse(validPayload())).To(Succeed())

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to update Historian configuration"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			Eventually(func() []models.ActionReplyState {
				return historianReplyStates(&messages, &mu)
			}).Should(Equal([]models.ActionReplyState{models.ActionConfirmed, models.ActionExecuting, models.ActionFinishedWithFailure}))
		})
	})
})
