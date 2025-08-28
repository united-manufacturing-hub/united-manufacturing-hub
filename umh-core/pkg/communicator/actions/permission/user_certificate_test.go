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
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	validator "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions/permission"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// createCertificateWithRoleExtension creates a test certificate with the specified location roles.
func createCertificateWithRoleExtension(locationRoles map[string]string) (*x509.Certificate, error) {
	// Generate Ed25519 key pair (matching production code)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	// Generate serial number (matching production code)
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, err
	}

	// Create certificate template (matching production code structure)
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Test Organization"},
			CommonName:   "test-user",
		},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().AddDate(100, 0, 0), // Valid for 100 years
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            10,
	}

	// Convert locationRoles to the format expected by the extension
	locationRolesMap := make(map[string]validator.Role)
	for location, roleStr := range locationRoles {
		locationRolesMap[location] = validator.Role(roleStr)
	}

	// Add the location role extension manually (based on production AddLocationRoleExtension)
	if len(locationRolesMap) == 0 {
		return nil, fmt.Errorf("must have locationRoles")
	}

	// Validate all roles and locations
	for location, role := range locationRolesMap {
		if role != validator.RoleAdmin && role != validator.RoleViewer && role != validator.RoleEditor {
			return nil, fmt.Errorf("invalid role: %s", role)
		}
		if len(strings.Split(location, ".")) <= 0 {
			return nil, fmt.Errorf("invalid location: %s", location)
		}
	}

	// Marshal the LocationRoles to YAML
	locationRolesBytes, err := yaml.Marshal(locationRolesMap)
	if err != nil {
		return nil, fmt.Errorf("failed to yaml marshal locationRoles: %w", err)
	}

	// ASN.1 encode the YAML string
	value, err := asn1.Marshal(string(locationRolesBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to asn1 marshal locationRoles: %w", err)
	}

	// Add the extension to the template using the same OID as production code
	oidExtensionRoleLocation := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59193, 2, 1}
	template.ExtraExtensions = append(template.ExtraExtensions, pkix.Extension{
		Id:       oidExtensionRoleLocation,
		Critical: false,
		Value:    value,
	})

	// Self-sign the certificate (for testing purposes)
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		return nil, err
	}

	// Parse the certificate
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}

	return cert, nil
}

var _ = Describe("UserCertificate", func() {
	Describe("ValidateUserCertificateForAction", func() {
		var mockLogger *zap.SugaredLogger

		// Create certificates at declaration time, not in BeforeEach
		adminCert, _ := createCertificateWithRoleExtension(map[string]string{
			"test-enterprise": "Admin",
		})
		editorCert, _ := createCertificateWithRoleExtension(map[string]string{
			"test-enterprise": "Editor",
		})
		viewerCert, _ := createCertificateWithRoleExtension(map[string]string{
			"test-enterprise": "Viewer",
		})

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
				Expect(err).ToNot(HaveOccurred())
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
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get locationRoles from certificate"))
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
				Expect(err).ToNot(HaveOccurred())
			})
		})

		DescribeTable("ValidateUserCertificateForAction with X.509 certificates",
			func(cert *x509.Certificate, actionTypeValue models.ActionType, useActionType bool, messageType models.MessageType, instanceLocation map[int]string, shouldSucceed bool, expectedErrorSubstring string, description string) {
				var actionType *models.ActionType
				if useActionType {
					actionType = &actionTypeValue
				}

				err := validator.ValidateUserCertificateForAction(
					mockLogger,
					cert,
					actionType,
					messageType,
					instanceLocation,
				)

				if shouldSucceed {
					Expect(err).ToNot(HaveOccurred(), "Expected success for %s", description)
				} else {
					Expect(err).To(HaveOccurred(), "Expected failure for %s", description)
					if expectedErrorSubstring != "" {
						Expect(err.Error()).To(ContainSubstring(expectedErrorSubstring), "Expected error message to contain '%s' for %s", expectedErrorSubstring, description)
					}
				}
			},

			// GET ACTIONS - All roles allowed
			Entry("Admin - get-protocol-converter - allowed", adminCert, models.GetProtocolConverter, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - get-protocol-converter - allowed", editorCert, models.GetProtocolConverter, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - get-protocol-converter - allowed", viewerCert, models.GetProtocolConverter, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),

			Entry("Admin - get-data-flow-component - allowed", adminCert, models.GetDataFlowComponent, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - get-data-flow-component - allowed", editorCert, models.GetDataFlowComponent, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - get-data-flow-component - allowed", viewerCert, models.GetDataFlowComponent, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),

			Entry("Admin - get-data-flow-component-log - allowed", adminCert, models.GetDataFlowComponentLog, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - get-data-flow-component-log - allowed", editorCert, models.GetDataFlowComponentLog, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - get-data-flow-component-log - allowed", viewerCert, models.GetDataFlowComponentLog, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),

			Entry("Admin - get-logs - allowed", adminCert, models.GetLogs, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - get-logs - allowed", editorCert, models.GetLogs, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - get-logs - allowed", viewerCert, models.GetLogs, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),

			Entry("Admin - get-datamodel - allowed", adminCert, models.GetDataModel, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - get-datamodel - allowed", editorCert, models.GetDataModel, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - get-datamodel - allowed", viewerCert, models.GetDataModel, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),

			Entry("Admin - get-stream-processor - allowed", adminCert, models.GetStreamProcessor, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - get-stream-processor - allowed", editorCert, models.GetStreamProcessor, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - get-stream-processor - allowed", viewerCert, models.GetStreamProcessor, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),

			// SYSTEM ACTIONS - Only Admin allowed
			Entry("Admin - upgrade-companion - allowed", adminCert, models.UpgradeCompanion, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - upgrade-companion - declined", editorCert, models.UpgradeCompanion, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Viewer - upgrade-companion - declined", viewerCert, models.UpgradeCompanion, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			// COMPANY ACTIONS - Only Admin allowed
			Entry("Admin - create-invite - allowed", adminCert, models.ActionType("create-invite"), true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - create-invite - declined", editorCert, models.ActionType("create-invite"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Viewer - create-invite - declined", viewerCert, models.ActionType("create-invite"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			// MORE COMPANY ACTIONS - Only Admin allowed
			Entry("Admin - get-company-invites - allowed", adminCert, models.ActionType("get-company-invites"), true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - get-company-invites - declined", editorCert, models.ActionType("get-company-invites"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Viewer - get-company-invites - declined", viewerCert, models.ActionType("get-company-invites"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - delete-invite - allowed", adminCert, models.ActionType("delete-invite"), true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - delete-invite - declined", editorCert, models.ActionType("delete-invite"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Viewer - delete-invite - declined", viewerCert, models.ActionType("delete-invite"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - get-company-users - allowed", adminCert, models.ActionType("get-company-users"), true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - get-company-users - declined", editorCert, models.ActionType("get-company-users"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Viewer - get-company-users - declined", viewerCert, models.ActionType("get-company-users"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - delete-company-user - allowed", adminCert, models.ActionType("delete-company-user"), true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - delete-company-user - declined", editorCert, models.ActionType("delete-company-user"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Viewer - delete-company-user - declined", viewerCert, models.ActionType("delete-company-user"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - change-user-role - allowed", adminCert, models.ActionType("change-user-role"), true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - change-user-role - declined", editorCert, models.ActionType("change-user-role"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Viewer - change-user-role - declined", viewerCert, models.ActionType("change-user-role"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - change-user-permissions - allowed", adminCert, models.ActionType("change-user-permissions"), true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - change-user-permissions - declined", editorCert, models.ActionType("change-user-permissions"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Viewer - change-user-permissions - declined", viewerCert, models.ActionType("change-user-permissions"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			// EDIT ACTIONS - Admin and Editor allowed, Viewer declined
			Entry("Admin - edit-protocol-converter - allowed", adminCert, models.EditProtocolConverter, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - edit-protocol-converter - allowed", editorCert, models.EditProtocolConverter, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - edit-protocol-converter - declined", viewerCert, models.EditProtocolConverter, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - edit-data-flow-component - allowed", adminCert, models.EditDataFlowComponent, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - edit-data-flow-component - allowed", editorCert, models.EditDataFlowComponent, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - edit-data-flow-component - declined", viewerCert, models.EditDataFlowComponent, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - edit-datamodel - allowed", adminCert, models.EditDataModel, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - edit-datamodel - allowed", editorCert, models.EditDataModel, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - edit-datamodel - declined", viewerCert, models.EditDataModel, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - edit-stream-processor - allowed", adminCert, models.EditStreamProcessor, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - edit-stream-processor - allowed", editorCert, models.EditStreamProcessor, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - edit-stream-processor - declined", viewerCert, models.EditStreamProcessor, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			// ADMIN-ONLY EDIT ACTIONS - Only Admin allowed
			Entry("Admin - edit-instance-location - allowed", adminCert, models.EditInstanceLocation, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - edit-instance-location - declined", editorCert, models.EditInstanceLocation, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Viewer - edit-instance-location - declined", viewerCert, models.EditInstanceLocation, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - edit-instance - allowed", adminCert, models.EditInstance, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - edit-instance - declined", editorCert, models.EditInstance, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Viewer - edit-instance - declined", viewerCert, models.EditInstance, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			// DELETE ACTIONS - Admin and Editor allowed, Viewer declined
			Entry("Admin - delete-protocol-converter - allowed", adminCert, models.DeleteProtocolConverter, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - delete-protocol-converter - allowed", editorCert, models.DeleteProtocolConverter, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - delete-protocol-converter - declined", viewerCert, models.DeleteProtocolConverter, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - delete-data-flow-component - allowed", adminCert, models.DeleteDataFlowComponent, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - delete-data-flow-component - allowed", editorCert, models.DeleteDataFlowComponent, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - delete-data-flow-component - declined", viewerCert, models.DeleteDataFlowComponent, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - delete-datamodel - allowed", adminCert, models.DeleteDataModel, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - delete-datamodel - allowed", editorCert, models.DeleteDataModel, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - delete-datamodel - declined", viewerCert, models.DeleteDataModel, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - delete-stream-processor - allowed", adminCert, models.DeleteStreamProcessor, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - delete-stream-processor - allowed", editorCert, models.DeleteStreamProcessor, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - delete-stream-processor - declined", viewerCert, models.DeleteStreamProcessor, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			// DEPLOY ACTIONS - Admin and Editor allowed, Viewer declined
			Entry("Admin - deploy-protocol-converter - allowed", adminCert, models.DeployProtocolConverter, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - deploy-protocol-converter - allowed", editorCert, models.DeployProtocolConverter, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - deploy-protocol-converter - declined", viewerCert, models.DeployProtocolConverter, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - deploy-data-flow-component - allowed", adminCert, models.DeployDataFlowComponent, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - deploy-data-flow-component - allowed", editorCert, models.DeployDataFlowComponent, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - deploy-data-flow-component - declined", viewerCert, models.DeployDataFlowComponent, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			Entry("Admin - deploy-stream-processor - allowed", adminCert, models.DeployStreamProcessor, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - deploy-stream-processor - allowed", editorCert, models.DeployStreamProcessor, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - deploy-stream-processor - declined", viewerCert, models.DeployStreamProcessor, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			// UNGROUPED ACTIONS - All roles allowed
			Entry("Admin - unknown - allowed", adminCert, models.UnknownAction, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - unknown - allowed", editorCert, models.UnknownAction, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - unknown - allowed", viewerCert, models.UnknownAction, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),

			Entry("Admin - dummy - allowed", adminCert, models.DummyAction, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - dummy - allowed", editorCert, models.DummyAction, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - dummy - allowed", viewerCert, models.DummyAction, true, models.Action, map[int]string{0: "test-enterprise"}, true, "", ""),

			// EDGE CASES - All roles declined for undefined actions
			Entry("Admin - non-existent-action - declined", adminCert, models.ActionType("non-existent-action"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Editor - non-existent-action - declined", editorCert, models.ActionType("non-existent-action"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Viewer - non-existent-action - declined", viewerCert, models.ActionType("non-existent-action"), true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			// TEST ACTIONS (empty group) - All roles declined
			Entry("Admin - test-network-connection - declined", adminCert, models.TestNetworkConnection, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Editor - test-network-connection - declined", editorCert, models.TestNetworkConnection, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),
			Entry("Viewer - test-network-connection - declined", viewerCert, models.TestNetworkConnection, true, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of type", ""),

			// LOCATION-BASED ACCESS CONTROL - All roles declined for wrong location
			Entry("Admin - nonexistent-location - declined", adminCert, models.GetProtocolConverter, true, models.Action, map[int]string{0: "nonexistent-location"}, false, "did not find this location", ""),
			Entry("Editor - nonexistent-location - declined", editorCert, models.GetProtocolConverter, true, models.Action, map[int]string{0: "nonexistent-location"}, false, "did not find this location", ""),
			Entry("Viewer - nonexistent-location - declined", viewerCert, models.GetProtocolConverter, true, models.Action, map[int]string{0: "nonexistent-location"}, false, "did not find this location", ""),

			// MESSAGE TYPE TESTS - Non-Action message types allowed for all roles with nil action type
			Entry("Admin - Subscribe message - allowed", adminCert, models.ActionType(""), false, models.Subscribe, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - Subscribe message - allowed", editorCert, models.ActionType(""), false, models.Subscribe, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - Subscribe message - allowed", viewerCert, models.ActionType(""), false, models.Subscribe, map[int]string{0: "test-enterprise"}, true, "", ""),

			Entry("Admin - Status message - allowed", adminCert, models.ActionType(""), false, models.Status, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - Status message - allowed", editorCert, models.ActionType(""), false, models.Status, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - Status message - allowed", viewerCert, models.ActionType(""), false, models.Status, map[int]string{0: "test-enterprise"}, true, "", ""),

			Entry("Admin - ActionReply message - allowed", adminCert, models.ActionType(""), false, models.ActionReply, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - ActionReply message - allowed", editorCert, models.ActionType(""), false, models.ActionReply, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - ActionReply message - allowed", viewerCert, models.ActionType(""), false, models.ActionReply, map[int]string{0: "test-enterprise"}, true, "", ""),

			Entry("Admin - EncryptedContent message - allowed", adminCert, models.ActionType(""), false, models.EncryptedContent, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Editor - EncryptedContent message - allowed", editorCert, models.ActionType(""), false, models.EncryptedContent, map[int]string{0: "test-enterprise"}, true, "", ""),
			Entry("Viewer - EncryptedContent message - allowed", viewerCert, models.ActionType(""), false, models.EncryptedContent, map[int]string{0: "test-enterprise"}, true, "", ""),

			// Action message type with nil action type should be denied for all roles
			Entry("Admin - Action message with nil action type - declined", adminCert, models.ActionType(""), false, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of message type", ""),
			Entry("Editor - Action message with nil action type - declined", editorCert, models.ActionType(""), false, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of message type", ""),
			Entry("Viewer - Action message with nil action type - declined", viewerCert, models.ActionType(""), false, models.Action, map[int]string{0: "test-enterprise"}, false, "user is not authorized to perform actions of message type", ""),
		)
	})
})
