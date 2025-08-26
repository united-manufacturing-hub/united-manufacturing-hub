// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package permission_validator_test

import (
	"crypto/x509"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	validator "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions/permission"
	"go.uber.org/zap"
)

var _ = Describe("UserCertificate", func() {
	Describe("ValidateUserCertificateForAction", func() {
		var mockLogger *zap.SugaredLogger

		BeforeEach(func() {
			mockLogger = zap.NewNop().Sugar()
		})

		Context("when certificate is nil", func() {
			It("should return nil (no authorization)", func() {
				actionType := models.GetProtocolConverter
				err := validator.ValidateUserCertificateForAction(
					mockLogger,
					nil,
					&actionType,
					models.Action,
					map[int]string{0: "test-enterprise"},
				)
				Expect(err).To(BeNil())
			})
		})

		Context("when certificate has no role extension", func() {
			It("should return error when role extraction fails", func() {
				actionType := models.GetProtocolConverter
				err := validator.ValidateUserCertificateForAction(
					mockLogger,
					&x509.Certificate{},
					&actionType,
					models.Action,
					map[int]string{0: "test-enterprise"},
				)
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(ContainSubstring("no role information found in certificate"))
			})
		})

		Context("when action type is nil", func() {
			It("should allow for non-Action message types", func() {
				err := validator.ValidateUserCertificateForAction(
					mockLogger,
					nil,
					nil,
					models.Subscribe,
					map[int]string{0: "test-enterprise"},
				)
				Expect(err).To(BeNil())
			})
		})
	})

	DescribeTable("IsRoleAllowedForActionAndMessageType when action type is nil",
		func(role validator.Role, messageType models.MessageType, expectedResult bool, description string) {
			result := validator.IsRoleAllowedForActionAndMessageType(role, nil, messageType)
			Expect(result).To(Equal(expectedResult), "Role %s should %s for message type %s when action type is nil",
				role, map[bool]string{true: "be allowed", false: "be denied"}[expectedResult], messageType)
		},

		// non-action message types - all roles should be allowed
		Entry("Subscribe [Admin]", validator.RoleAdmin, models.Subscribe, true, "subscribe message type"),
		Entry("Subscribe [Editor]", validator.RoleEditor, models.Subscribe, true, "subscribe message type"),
		Entry("Subscribe [Viewer]", validator.RoleViewer, models.Subscribe, true, "subscribe message type"),

		Entry("Status [Admin]", validator.RoleAdmin, models.Status, true, "status message type"),
		Entry("Status [Editor]", validator.RoleEditor, models.Status, true, "status message type"),
		Entry("Status [Viewer]", validator.RoleViewer, models.Status, true, "status message type"),

		Entry("ActionReply [Admin]", validator.RoleAdmin, models.ActionReply, true, "action-reply message type"),
		Entry("ActionReply [Editor]", validator.RoleEditor, models.ActionReply, true, "action-reply message type"),
		Entry("ActionReply [Viewer]", validator.RoleViewer, models.ActionReply, true, "action-reply message type"),

		Entry("EncryptedContent [Admin]", validator.RoleAdmin, models.EncryptedContent, true, "encrypted-content message type"),
		Entry("EncryptedContent [Editor]", validator.RoleEditor, models.EncryptedContent, true, "encrypted-content message type"),
		Entry("EncryptedContent [Viewer]", validator.RoleViewer, models.EncryptedContent, true, "encrypted-content message type"),

		// action message type - all roles should be denied
		Entry("Action [Admin]", validator.RoleAdmin, models.Action, false, "action message type"),
		Entry("Action [Editor]", validator.RoleEditor, models.Action, false, "action message type"),
		Entry("Action [Viewer]", validator.RoleViewer, models.Action, false, "action message type"),
	)

	DescribeTable("IsRoleAllowedForActionAndMessageType - Comprehensive Action Permission Matrix",
		func(action models.ActionType, role validator.Role, expectedResult bool, description string) {
			result := validator.IsRoleAllowedForActionAndMessageType(role, &action, models.Action)
			Expect(result).To(Equal(expectedResult), "Role %s should %s for action %s", role, map[bool]string{true: "be allowed", false: "be denied"}[expectedResult], description)
		},

		Entry("get-protocol-converter [Admin]", models.GetProtocolConverter, validator.RoleAdmin, true, "get-protocol-converter"),
		Entry("get-protocol-converter [Editor]", models.GetProtocolConverter, validator.RoleEditor, true, "get-protocol-converter"),
		Entry("get-protocol-converter [Viewer]", models.GetProtocolConverter, validator.RoleViewer, true, "get-protocol-converter"),

		Entry("get-data-flow-component [Admin]", models.GetDataFlowComponent, validator.RoleAdmin, true, "get-data-flow-component"),
		Entry("get-data-flow-component [Editor]", models.GetDataFlowComponent, validator.RoleEditor, true, "get-data-flow-component"),
		Entry("get-data-flow-component [Viewer]", models.GetDataFlowComponent, validator.RoleViewer, true, "get-data-flow-component"),

		Entry("get-data-flow-component-log [Admin]", models.GetDataFlowComponentLog, validator.RoleAdmin, true, "get-data-flow-component-log"),
		Entry("get-data-flow-component-log [Editor]", models.GetDataFlowComponentLog, validator.RoleEditor, true, "get-data-flow-component-log"),
		Entry("get-data-flow-component-log [Viewer]", models.GetDataFlowComponentLog, validator.RoleViewer, true, "get-data-flow-component-log"),

		Entry("get-logs [Admin]", models.GetLogs, validator.RoleAdmin, true, "get-logs"),
		Entry("get-logs [Editor]", models.GetLogs, validator.RoleEditor, true, "get-logs"),
		Entry("get-logs [Viewer]", models.GetLogs, validator.RoleViewer, true, "get-logs"),

		Entry("get-datamodel [Admin]", models.GetDataModel, validator.RoleAdmin, true, "get-datamodel"),
		Entry("get-datamodel [Editor]", models.GetDataModel, validator.RoleEditor, true, "get-datamodel"),
		Entry("get-datamodel [Viewer]", models.GetDataModel, validator.RoleViewer, true, "get-datamodel"),

		Entry("get-stream-processor [Admin]", models.GetStreamProcessor, validator.RoleAdmin, true, "get-stream-processor"),
		Entry("get-stream-processor [Editor]", models.GetStreamProcessor, validator.RoleEditor, true, "get-stream-processor"),
		Entry("get-stream-processor [Viewer]", models.GetStreamProcessor, validator.RoleViewer, true, "get-stream-processor"),

		Entry("edit-protocol-converter [Admin]", models.EditProtocolConverter, validator.RoleAdmin, true, "edit-protocol-converter"),
		Entry("edit-protocol-converter [Editor]", models.EditProtocolConverter, validator.RoleEditor, true, "edit-protocol-converter"),
		Entry("edit-protocol-converter [Viewer]", models.EditProtocolConverter, validator.RoleViewer, false, "edit-protocol-converter"),

		Entry("edit-data-flow-component [Admin]", models.EditDataFlowComponent, validator.RoleAdmin, true, "edit-data-flow-component"),
		Entry("edit-data-flow-component [Editor]", models.EditDataFlowComponent, validator.RoleEditor, true, "edit-data-flow-component"),
		Entry("edit-data-flow-component [Viewer]", models.EditDataFlowComponent, validator.RoleViewer, false, "edit-data-flow-component"),

		Entry("edit-datamodel [Admin]", models.EditDataModel, validator.RoleAdmin, true, "edit-datamodel"),
		Entry("edit-datamodel [Editor]", models.EditDataModel, validator.RoleEditor, true, "edit-datamodel"),
		Entry("edit-datamodel [Viewer]", models.EditDataModel, validator.RoleViewer, false, "edit-datamodel"),

		Entry("edit-stream-processor [Admin]", models.EditStreamProcessor, validator.RoleAdmin, true, "edit-stream-processor"),
		Entry("edit-stream-processor [Editor]", models.EditStreamProcessor, validator.RoleEditor, true, "edit-stream-processor"),
		Entry("edit-stream-processor [Viewer]", models.EditStreamProcessor, validator.RoleViewer, false, "edit-stream-processor"),

		Entry("edit-instance [Admin]", models.EditInstance, validator.RoleAdmin, true, "edit-instance"),
		Entry("edit-instance [Editor]", models.EditInstance, validator.RoleEditor, false, "edit-instance"),
		Entry("edit-instance [Viewer]", models.EditInstance, validator.RoleViewer, false, "edit-instance"),

		Entry("edit-instance-location [Admin]", models.EditInstanceLocation, validator.RoleAdmin, true, "edit-instance-location"),
		Entry("edit-instance-location [Editor]", models.EditInstanceLocation, validator.RoleEditor, false, "edit-instance-location"),
		Entry("edit-instance-location [Viewer]", models.EditInstanceLocation, validator.RoleViewer, false, "edit-instance-location"),

		Entry("delete-protocol-converter [Admin]", models.DeleteProtocolConverter, validator.RoleAdmin, true, "delete-protocol-converter"),
		Entry("delete-protocol-converter [Editor]", models.DeleteProtocolConverter, validator.RoleEditor, true, "delete-protocol-converter"),
		Entry("delete-protocol-converter [Viewer]", models.DeleteProtocolConverter, validator.RoleViewer, false, "delete-protocol-converter"),

		Entry("delete-data-flow-component [Admin]", models.DeleteDataFlowComponent, validator.RoleAdmin, true, "delete-data-flow-component"),
		Entry("delete-data-flow-component [Editor]", models.DeleteDataFlowComponent, validator.RoleEditor, true, "delete-data-flow-component"),
		Entry("delete-data-flow-component [Viewer]", models.DeleteDataFlowComponent, validator.RoleViewer, false, "delete-data-flow-component"),

		Entry("delete-datamodel [Admin]", models.DeleteDataModel, validator.RoleAdmin, true, "delete-datamodel"),
		Entry("delete-datamodel [Editor]", models.DeleteDataModel, validator.RoleEditor, true, "delete-datamodel"),
		Entry("delete-datamodel [Viewer]", models.DeleteDataModel, validator.RoleViewer, false, "delete-datamodel"),

		Entry("delete-stream-processor [Admin]", models.DeleteStreamProcessor, validator.RoleAdmin, true, "delete-stream-processor"),
		Entry("delete-stream-processor [Editor]", models.DeleteStreamProcessor, validator.RoleEditor, true, "delete-stream-processor"),
		Entry("delete-stream-processor [Viewer]", models.DeleteStreamProcessor, validator.RoleViewer, false, "delete-stream-processor"),

		Entry("deploy-protocol-converter [Admin]", models.DeployProtocolConverter, validator.RoleAdmin, true, "deploy-protocol-converter"),
		Entry("deploy-protocol-converter [Editor]", models.DeployProtocolConverter, validator.RoleEditor, true, "deploy-protocol-converter"),
		Entry("deploy-protocol-converter [Viewer]", models.DeployProtocolConverter, validator.RoleViewer, false, "deploy-protocol-converter"),

		Entry("deploy-data-flow-component [Admin]", models.DeployDataFlowComponent, validator.RoleAdmin, true, "deploy-data-flow-component"),
		Entry("deploy-data-flow-component [Editor]", models.DeployDataFlowComponent, validator.RoleEditor, true, "deploy-data-flow-component"),
		Entry("deploy-data-flow-component [Viewer]", models.DeployDataFlowComponent, validator.RoleViewer, false, "deploy-data-flow-component"),

		Entry("deploy-stream-processor [Admin]", models.DeployStreamProcessor, validator.RoleAdmin, true, "deploy-stream-processor"),
		Entry("deploy-stream-processor [Editor]", models.DeployStreamProcessor, validator.RoleEditor, true, "deploy-stream-processor"),
		Entry("deploy-stream-processor [Viewer]", models.DeployStreamProcessor, validator.RoleViewer, false, "deploy-stream-processor"),

		Entry("upgrade-companion [Admin]", models.UpgradeCompanion, validator.RoleAdmin, true, "upgrade-companion"),
		Entry("upgrade-companion [Editor]", models.UpgradeCompanion, validator.RoleEditor, false, "upgrade-companion"),
		Entry("upgrade-companion [Viewer]", models.UpgradeCompanion, validator.RoleViewer, false, "upgrade-companion"),

		Entry("create-invite [Admin]", models.ActionType("create-invite"), validator.RoleAdmin, true, "create-invite"),
		Entry("create-invite [Editor]", models.ActionType("create-invite"), validator.RoleEditor, false, "create-invite"),
		Entry("create-invite [Viewer]", models.ActionType("create-invite"), validator.RoleViewer, false, "create-invite"),

		Entry("get-company-invites [Admin]", models.ActionType("get-company-invites"), validator.RoleAdmin, true, "get-company-invites"),
		Entry("get-company-invites [Editor]", models.ActionType("get-company-invites"), validator.RoleEditor, false, "get-company-invites"),
		Entry("get-company-invites [Viewer]", models.ActionType("get-company-invites"), validator.RoleViewer, false, "get-company-invites"),

		Entry("delete-invite [Admin]", models.ActionType("delete-invite"), validator.RoleAdmin, true, "delete-invite"),
		Entry("delete-invite [Editor]", models.ActionType("delete-invite"), validator.RoleEditor, false, "delete-invite"),
		Entry("delete-invite [Viewer]", models.ActionType("delete-invite"), validator.RoleViewer, false, "delete-invite"),

		Entry("get-company-users [Admin]", models.ActionType("get-company-users"), validator.RoleAdmin, true, "get-company-users"),
		Entry("get-company-users [Editor]", models.ActionType("get-company-users"), validator.RoleEditor, false, "get-company-users"),
		Entry("get-company-users [Viewer]", models.ActionType("get-company-users"), validator.RoleViewer, false, "get-company-users"),

		Entry("delete-company-user [Admin]", models.ActionType("delete-company-user"), validator.RoleAdmin, true, "delete-company-user"),
		Entry("delete-company-user [Editor]", models.ActionType("delete-company-user"), validator.RoleEditor, false, "delete-company-user"),
		Entry("delete-company-user [Viewer]", models.ActionType("delete-company-user"), validator.RoleViewer, false, "delete-company-user"),

		Entry("change-user-role [Admin]", models.ActionType("change-user-role"), validator.RoleAdmin, true, "change-user-role"),
		Entry("change-user-role [Editor]", models.ActionType("change-user-role"), validator.RoleEditor, false, "change-user-role"),
		Entry("change-user-role [Viewer]", models.ActionType("change-user-role"), validator.RoleViewer, false, "change-user-role"),

		Entry("change-user-permissions [Admin]", models.ActionType("change-user-permissions"), validator.RoleAdmin, true, "change-user-permissions"),
		Entry("change-user-permissions [Editor]", models.ActionType("change-user-permissions"), validator.RoleEditor, false, "change-user-permissions"),
		Entry("change-user-permissions [Viewer]", models.ActionType("change-user-permissions"), validator.RoleViewer, false, "change-user-permissions"),

		// ungrouped actions - all roles allowed
		Entry("unknown [Admin]", models.ActionType("unknown"), validator.RoleAdmin, true, "unknown"),
		Entry("unknown [Editor]", models.ActionType("unknown"), validator.RoleEditor, true, "unknown"),
		Entry("unknown [Viewer]", models.ActionType("unknown"), validator.RoleViewer, true, "unknown"),

		Entry("dummy [Admin]", models.ActionType("dummy"), validator.RoleAdmin, true, "dummy"),
		Entry("dummy [Editor]", models.ActionType("dummy"), validator.RoleEditor, true, "dummy"),
		Entry("dummy [Viewer]", models.ActionType("dummy"), validator.RoleViewer, true, "dummy"),

		// non-existent actions - all roles denied
		Entry("non-existent-action [Admin]", models.ActionType("non-existent-action"), validator.RoleAdmin, false, "non-existent-action"),
		Entry("non-existent-action [Editor]", models.ActionType("non-existent-action"), validator.RoleEditor, false, "non-existent-action"),
		Entry("non-existent-action [Viewer]", models.ActionType("non-existent-action"), validator.RoleViewer, false, "non-existent-action"),

		Entry("fake-action [Admin]", models.ActionType("fake-action"), validator.RoleAdmin, false, "fake-action"),
		Entry("fake-action [Editor]", models.ActionType("fake-action"), validator.RoleEditor, false, "fake-action"),
		Entry("fake-action [Viewer]", models.ActionType("fake-action"), validator.RoleViewer, false, "fake-action"),
	)
})
