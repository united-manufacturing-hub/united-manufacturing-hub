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

var _ = Describe("DeleteHistorian", func() {
	var (
		action          *actions.DeleteHistorianAction
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
			},
		})

		action = actions.NewDeleteHistorianAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig)

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
		It("should remove an existing historian config", func() {
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			Expect(mockConfig.AtomicDeleteHistorianCalled).To(BeTrue())
			Expect(mockConfig.Config.Historian).To(BeNil())
		})

		It("should succeed when no historian is configured (idempotent)", func() {
			mockConfig.WithConfig(config.FullConfig{})

			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			Expect(mockConfig.AtomicDeleteHistorianCalled).To(BeTrue())
			Expect(mockConfig.Config.Historian).To(BeNil())
		})

		It("should handle AtomicDeleteHistorian failure", func() {
			mockConfig.WithAtomicDeleteHistorianError(errors.New("mock delete failure"))

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to delete Historian configuration"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())
		})
	})
})
