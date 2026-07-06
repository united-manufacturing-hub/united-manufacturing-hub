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

var _ = Describe("DeployHistorian", func() {
	var (
		action          *actions.DeployHistorianAction
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
			"host":     "timescale.example.com",
			"password": "secret",
		}
	}

	BeforeEach(func() {
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10)

		mockConfig = config.NewMockConfigManager().WithConfig(config.FullConfig{})

		action = actions.NewDeployHistorianAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig)

		go actions.ConsumeOutboundMessages(outboundChannel, &messages, &mu, true)
	})

	AfterEach(func() {
		for len(outboundChannel) > 0 {
			<-outboundChannel
		}
		close(outboundChannel)
	})

	Describe("Parse", func() {
		It("should parse a valid historian payload", func() {
			err := action.Parse(validPayload())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error for an invalid payload format", func() {
			err := action.Parse(make(chan int))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse payload"))
		})
	})

	Describe("Validate", func() {
		It("should pass validation for a valid payload", func() {
			Expect(action.Parse(validPayload())).To(Succeed())
			Expect(action.Validate()).To(Succeed())
		})

		It("should fail validation when host is missing", func() {
			payload := validPayload()
			delete(payload, "host")

			Expect(action.Parse(payload)).To(Succeed())

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field host"))
		})

		It("should fail validation when password is missing", func() {
			payload := validPayload()
			delete(payload, "password")

			Expect(action.Parse(payload)).To(Succeed())

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field password"))
		})

		It("should fail validation for an invalid sslmode", func() {
			payload := validPayload()
			payload["sslmode"] = "bogus"

			Expect(action.Parse(payload)).To(Succeed())

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid sslmode"))
		})
	})

	Describe("Execute", func() {
		It("should write the historian config with defaults applied", func() {
			Expect(action.Parse(validPayload())).To(Succeed())

			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			Expect(mockConfig.AtomicSetHistorianCalled).To(BeTrue())

			cfg, ok := result.(config.HistorianConfig)
			Expect(ok).To(BeTrue(), "Result should be a HistorianConfig")
			Expect(cfg.Host).To(Equal("timescale.example.com"))
			Expect(cfg.Port).To(Equal(uint16(5432)))
			Expect(cfg.Database).To(Equal("umh"))
			Expect(cfg.Username).To(Equal("umh_owner"))
			Expect(cfg.SSLMode).To(Equal(config.HistorianSSLModeRequire))

			Expect(mockConfig.Config.Historian).NotTo(BeNil())
			Expect(mockConfig.Config.Historian.Host).To(Equal("timescale.example.com"))
		})

		It("should handle AtomicSetHistorian failure", func() {
			mockConfig.WithAtomicSetHistorianError(errors.New("mock write failure"))

			Expect(action.Parse(validPayload())).To(Succeed())

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to write Historian configuration"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())
		})
	})
})
