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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	validator "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions/permission"
)

var _ = Describe("UserCertificate", func() {
	Describe("IsLocationAuthorized", func() {
		Context("when hierarchies are empty", func() {
			It("should deny access", func() {
				instanceLocation := models.InstanceLocation{
					Enterprise: "TestEnterprise",
					Site:       "TestSite",
					Area:       "TestArea",
					Line:       "TestLine",
					WorkCell:   "TestWorkCell",
				}

				Expect(validator.IsLocationAuthorized(&instanceLocation, []validator.LocationHierarchy{})).To(BeFalse())
			})
		})

		Context("when at least one hierarchy authorizes the location", func() {
			It("should allow access", func() {
				instanceLocation := models.InstanceLocation{
					Enterprise: "TestEnterprise",
					Site:       "TestSite",
					Area:       "TestArea",
					Line:       "TestLine",
					WorkCell:   "TestWorkCell",
				}

				// Create a hierarchy that matches exactly
				exactHierarchy := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewLocation(validator.LocationTypeArea, "TestArea"),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}

				// Create a hierarchy that doesn't match
				nonMatchingHierarchy := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "OtherEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "OtherSite"),
					Area:           validator.NewLocation(validator.LocationTypeArea, "OtherArea"),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "OtherLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "OtherWorkCell"),
				}

				// Test with only the matching hierarchy
				Expect(validator.IsLocationAuthorized(&instanceLocation, []validator.LocationHierarchy{exactHierarchy})).To(BeTrue())

				// Test with both hierarchies - should still allow because at least one matches
				Expect(validator.IsLocationAuthorized(&instanceLocation, []validator.LocationHierarchy{nonMatchingHierarchy, exactHierarchy})).To(BeTrue())

				// Test with only the non-matching hierarchy
				Expect(validator.IsLocationAuthorized(&instanceLocation, []validator.LocationHierarchy{nonMatchingHierarchy})).To(BeFalse())
			})
		})

		Context("when using wildcard hierarchies", func() {
			It("should authorize when at least one wildcard hierarchy matches", func() {
				instanceLocation := models.InstanceLocation{
					Enterprise: "TestEnterprise",
					Site:       "TestSite",
					Area:       "TestArea",
					Line:       "TestLine",
					WorkCell:   "TestWorkCell",
				}

				// Create a hierarchy with wildcards
				wildcardHierarchy := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewWildcardLocation(validator.LocationTypeProductionLine),
					WorkCell:       validator.NewWildcardLocation(validator.LocationTypeWorkCell),
				}

				// Create a non-matching hierarchy
				nonMatchingHierarchy := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "OtherEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "OtherSite"),
					Area:           validator.NewLocation(validator.LocationTypeArea, "OtherArea"),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "OtherLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "OtherWorkCell"),
				}

				// Test with only the wildcard hierarchy
				Expect(validator.IsLocationAuthorized(&instanceLocation, []validator.LocationHierarchy{wildcardHierarchy})).To(BeTrue())

				// Test with both hierarchies - should still allow because at least one matches
				Expect(validator.IsLocationAuthorized(&instanceLocation, []validator.LocationHierarchy{nonMatchingHierarchy, wildcardHierarchy})).To(BeTrue())

				// Test with all-wildcard hierarchy
				allWildcardHierarchy := validator.LocationHierarchy{
					Enterprise:     validator.NewWildcardLocation(validator.LocationTypeEnterprise),
					Site:           validator.NewWildcardLocation(validator.LocationTypeSite),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewWildcardLocation(validator.LocationTypeProductionLine),
					WorkCell:       validator.NewWildcardLocation(validator.LocationTypeWorkCell),
				}

				Expect(validator.IsLocationAuthorized(&instanceLocation, []validator.LocationHierarchy{allWildcardHierarchy})).To(BeTrue())
			})
		})

		Context("when using wildcard hierarchies with wildcards in the middle", func() {
			It("should authorize when wildcards are in the middle of the hierarchy", func() {
				instanceLocation := models.InstanceLocation{
					Enterprise: "TestEnterprise",
					Site:       "TestSite",
					Area:       "TestArea",
					Line:       "TestLine",
					WorkCell:   "TestWorkCell",
				}

				// Create a hierarchy with wildcards in the middle
				wildcardInMiddleHierarchy := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewWildcardLocation(validator.LocationTypeSite),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}

				Expect(validator.IsLocationAuthorized(&instanceLocation, []validator.LocationHierarchy{wildcardInMiddleHierarchy})).To(BeTrue())

				// Another hierarchy with different wildcards in the middle
				anotherWildcardInMiddleHierarchy := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewWildcardLocation(validator.LocationTypeWorkCell),
				}

				Expect(validator.IsLocationAuthorized(&instanceLocation, []validator.LocationHierarchy{anotherWildcardInMiddleHierarchy})).To(BeTrue())
			})
		})
	})

	Describe("IsLocationAuthorizedByHierarchy", func() {
		Context("when using exact matches", func() {
			It("should authorize only when all fields match", func() {
				instanceLocation := models.InstanceLocation{
					Enterprise: "TestEnterprise",
					Site:       "TestSite",
					Area:       "TestArea",
					Line:       "TestLine",
					WorkCell:   "TestWorkCell",
				}

				// Create a hierarchy that matches exactly
				exactHierarchy := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewLocation(validator.LocationTypeArea, "TestArea"),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}

				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, exactHierarchy)).To(BeTrue())

				// Test with a mismatch at each level
				enterpriseMismatch := exactHierarchy
				enterpriseMismatch.Enterprise = validator.NewLocation(validator.LocationTypeEnterprise, "OtherEnterprise")
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, enterpriseMismatch)).To(BeFalse())

				siteMismatch := exactHierarchy
				siteMismatch.Site = validator.NewLocation(validator.LocationTypeSite, "OtherSite")
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, siteMismatch)).To(BeFalse())

				areaMismatch := exactHierarchy
				areaMismatch.Area = validator.NewLocation(validator.LocationTypeArea, "OtherArea")
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, areaMismatch)).To(BeFalse())

				lineMismatch := exactHierarchy
				lineMismatch.ProductionLine = validator.NewLocation(validator.LocationTypeProductionLine, "OtherLine")
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, lineMismatch)).To(BeFalse())

				workCellMismatch := exactHierarchy
				workCellMismatch.WorkCell = validator.NewLocation(validator.LocationTypeWorkCell, "OtherWorkCell")
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, workCellMismatch)).To(BeFalse())
			})
		})

		Context("when using wildcards", func() {
			It("should authorize when wildcards are used", func() {
				instanceLocation := models.InstanceLocation{
					Enterprise: "TestEnterprise",
					Site:       "TestSite",
					Area:       "TestArea",
					Line:       "TestLine",
					WorkCell:   "TestWorkCell",
				}

				// Create a hierarchy with wildcards at different levels
				wildcardHierarchy := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewWildcardLocation(validator.LocationTypeProductionLine),
					WorkCell:       validator.NewWildcardLocation(validator.LocationTypeWorkCell),
				}

				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, wildcardHierarchy)).To(BeTrue())

				// Test with wildcards at different levels
				enterpriseWildcard := validator.LocationHierarchy{
					Enterprise:     validator.NewWildcardLocation(validator.LocationTypeEnterprise),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewLocation(validator.LocationTypeArea, "TestArea"),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, enterpriseWildcard)).To(BeTrue())

				// Test with all wildcards
				allWildcards := validator.LocationHierarchy{
					Enterprise:     validator.NewWildcardLocation(validator.LocationTypeEnterprise),
					Site:           validator.NewWildcardLocation(validator.LocationTypeSite),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewWildcardLocation(validator.LocationTypeProductionLine),
					WorkCell:       validator.NewWildcardLocation(validator.LocationTypeWorkCell),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, allWildcards)).To(BeTrue())
			})

			It("should test each level of the hierarchy independently", func() {
				instanceLocation := models.InstanceLocation{
					Enterprise: "TestEnterprise",
					Site:       "TestSite",
					Area:       "TestArea",
					Line:       "TestLine",
					WorkCell:   "TestWorkCell",
				}

				// Test wildcard at enterprise level only
				enterpriseWildcard := validator.LocationHierarchy{
					Enterprise:     validator.NewWildcardLocation(validator.LocationTypeEnterprise),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewLocation(validator.LocationTypeArea, "TestArea"),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, enterpriseWildcard)).To(BeTrue())

				// Test wildcard at site level only
				siteWildcard := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewWildcardLocation(validator.LocationTypeSite),
					Area:           validator.NewLocation(validator.LocationTypeArea, "TestArea"),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, siteWildcard)).To(BeTrue())

				// Test wildcard at area level only
				areaWildcard := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, areaWildcard)).To(BeTrue())

				// Test wildcard at production line level only
				lineWildcard := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewLocation(validator.LocationTypeArea, "TestArea"),
					ProductionLine: validator.NewWildcardLocation(validator.LocationTypeProductionLine),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, lineWildcard)).To(BeTrue())

				// Test wildcard at work cell level only
				workCellWildcard := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewLocation(validator.LocationTypeArea, "TestArea"),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewWildcardLocation(validator.LocationTypeWorkCell),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, workCellWildcard)).To(BeTrue())
			})

			It("should verify that wildcards don't override mismatches at other levels", func() {
				instanceLocation := models.InstanceLocation{
					Enterprise: "TestEnterprise",
					Site:       "TestSite",
					Area:       "TestArea",
					Line:       "TestLine",
					WorkCell:   "TestWorkCell",
				}

				// Wildcard at one level but mismatch at another level
				wildcardWithMismatch := validator.LocationHierarchy{
					Enterprise:     validator.NewWildcardLocation(validator.LocationTypeEnterprise),
					Site:           validator.NewLocation(validator.LocationTypeSite, "WrongSite"),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewWildcardLocation(validator.LocationTypeProductionLine),
					WorkCell:       validator.NewWildcardLocation(validator.LocationTypeWorkCell),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, wildcardWithMismatch)).To(BeFalse())

				// Multiple wildcards but one mismatch
				multipleWildcardsOneMismatch := validator.LocationHierarchy{
					Enterprise:     validator.NewWildcardLocation(validator.LocationTypeEnterprise),
					Site:           validator.NewWildcardLocation(validator.LocationTypeSite),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "WrongLine"),
					WorkCell:       validator.NewWildcardLocation(validator.LocationTypeWorkCell),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, multipleWildcardsOneMismatch)).To(BeFalse())
			})

			It("should handle empty location values correctly", func() {
				// Instance location with some empty values
				instanceLocation := models.InstanceLocation{
					Enterprise: "TestEnterprise",
					Site:       "TestSite",
					Area:       "", // Empty area
					Line:       "TestLine",
					WorkCell:   "", // Empty work cell
				}

				// Hierarchy with exact matches for non-empty values and wildcards for empty values
				hierarchyWithWildcards := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewWildcardLocation(validator.LocationTypeWorkCell),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, hierarchyWithWildcards)).To(BeTrue())

				// Hierarchy with exact matches for all values (including empty ones)
				hierarchyWithExactMatches := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewLocation(validator.LocationTypeArea, ""), // Match empty area
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, ""), // Match empty work cell
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, hierarchyWithExactMatches)).To(BeTrue())

				// Hierarchy with non-matching values for empty fields
				hierarchyWithNonMatches := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewLocation(validator.LocationTypeArea, "SomeArea"), // Non-matching area
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "SomeWorkCell"), // Non-matching work cell
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, hierarchyWithNonMatches)).To(BeFalse())
			})
		})

		Context("when using wildcards in the middle of the hierarchy", func() {
			It("should authorize with wildcards at any position in the hierarchy", func() {
				instanceLocation := models.InstanceLocation{
					Enterprise: "TestEnterprise",
					Site:       "TestSite",
					Area:       "TestArea",
					Line:       "TestLine",
					WorkCell:   "TestWorkCell",
				}

				// Test with wildcards at various positions in the hierarchy

				// Wildcard in the middle (Site)
				siteWildcardMiddle := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewWildcardLocation(validator.LocationTypeSite),
					Area:           validator.NewLocation(validator.LocationTypeArea, "TestArea"),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, siteWildcardMiddle)).To(BeTrue())

				// Wildcard in the middle (Area)
				areaWildcardMiddle := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, areaWildcardMiddle)).To(BeTrue())

				// Multiple wildcards in the middle
				multipleWildcardsMiddle := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewWildcardLocation(validator.LocationTypeSite),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, multipleWildcardsMiddle)).To(BeTrue())

				// Wildcards at start and end, specific values in middle
				wildcardStartEndSpecificMiddle := validator.LocationHierarchy{
					Enterprise:     validator.NewWildcardLocation(validator.LocationTypeEnterprise),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewLocation(validator.LocationTypeArea, "TestArea"),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewWildcardLocation(validator.LocationTypeWorkCell),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, wildcardStartEndSpecificMiddle)).To(BeTrue())

				// Alternating wildcards and specific values
				alternatingWildcards := validator.LocationHierarchy{
					Enterprise:     validator.NewWildcardLocation(validator.LocationTypeEnterprise),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewWildcardLocation(validator.LocationTypeWorkCell),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, alternatingWildcards)).To(BeTrue())
			})

			It("should deny access when non-wildcard values don't match", func() {
				instanceLocation := models.InstanceLocation{
					Enterprise: "TestEnterprise",
					Site:       "TestSite",
					Area:       "TestArea",
					Line:       "TestLine",
					WorkCell:   "TestWorkCell",
				}

				// Wildcards in the middle but with non-matching values at other levels
				wildcardMiddleNonMatchingOthers := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "OtherEnterprise"),
					Site:           validator.NewWildcardLocation(validator.LocationTypeSite),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "TestLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, wildcardMiddleNonMatchingOthers)).To(BeFalse())

				// Another example with wildcards in different positions
				anotherWildcardMiddleNonMatching := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "TestEnterprise"),
					Site:           validator.NewLocation(validator.LocationTypeSite, "TestSite"),
					Area:           validator.NewWildcardLocation(validator.LocationTypeArea),
					ProductionLine: validator.NewLocation(validator.LocationTypeProductionLine, "OtherLine"),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "TestWorkCell"),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&instanceLocation, anotherWildcardMiddleNonMatching)).To(BeFalse())
			})

			It("should handle complex hierarchies with mixed wildcards and specific values", func() {
				// Test with a more complex instance location
				complexInstanceLocation := models.InstanceLocation{
					Enterprise: "MegaCorp",
					Site:       "Headquarters",
					Area:       "Manufacturing",
					Line:       "Assembly",
					WorkCell:   "Station5",
				}

				// Complex hierarchy with wildcards at various levels
				complexHierarchy := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "MegaCorp"),
					Site:           validator.NewWildcardLocation(validator.LocationTypeSite),
					Area:           validator.NewLocation(validator.LocationTypeArea, "Manufacturing"),
					ProductionLine: validator.NewWildcardLocation(validator.LocationTypeProductionLine),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "Station5"),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&complexInstanceLocation, complexHierarchy)).To(BeTrue())

				// Change one non-wildcard value to make it not match
				complexHierarchyNonMatching := validator.LocationHierarchy{
					Enterprise:     validator.NewLocation(validator.LocationTypeEnterprise, "MegaCorp"),
					Site:           validator.NewWildcardLocation(validator.LocationTypeSite),
					Area:           validator.NewLocation(validator.LocationTypeArea, "Research"), // Different from "Manufacturing"
					ProductionLine: validator.NewWildcardLocation(validator.LocationTypeProductionLine),
					WorkCell:       validator.NewLocation(validator.LocationTypeWorkCell, "Station5"),
				}
				Expect(validator.IsLocationAuthorizedByHierarchy(&complexInstanceLocation, complexHierarchyNonMatching)).To(BeFalse())
			})
		})
	})

	Describe("IsRoleAllowedForActionAndMessageType", func() {
		Context("when action type is nil", func() {
			It("should allow any role for non-Action message types", func() {
				// Test with nil action type and non-Action message types
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, nil, models.Subscribe)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, nil, models.Status)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, nil, models.ActionReply)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, nil, models.EncryptedContent)).To(BeTrue())
			})

			It("should deny any role for Action message type", func() {
				// Test with nil action type and Action message type
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, nil, models.Action)).To(BeFalse())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, nil, models.Action)).To(BeFalse())
			})
		})

		Context("when action type is provided", func() {
			It("should allow Admin role for admin-only actions", func() {
				// Test with action types that only allow Admin role
				getAuditLog := models.GetAuditLog
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, &getAuditLog, models.Action)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, &getAuditLog, models.Action)).To(BeFalse())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleEditor, &getAuditLog, models.Action)).To(BeFalse())

				upgradeCompanion := models.UpgradeCompanion
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, &upgradeCompanion, models.Action)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, &upgradeCompanion, models.Action)).To(BeFalse())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleEditor, &upgradeCompanion, models.Action)).To(BeFalse())

				editInstance := models.EditInstance
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, &editInstance, models.Action)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, &editInstance, models.Action)).To(BeFalse())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleEditor, &editInstance, models.Action)).To(BeFalse())
			})

			It("should allow both Admin and Viewer roles for viewer-allowed actions", func() {
				// Test with action types that allow both Admin and Viewer roles
				getConnectionNotes := models.GetConnectionNotes
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, &getConnectionNotes, models.Action)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, &getConnectionNotes, models.Action)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleEditor, &getConnectionNotes, models.Action)).To(BeTrue())

				getDatasourceBasic := models.GetDatasourceBasic
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, &getDatasourceBasic, models.Action)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, &getDatasourceBasic, models.Action)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleEditor, &getDatasourceBasic, models.Action)).To(BeTrue())
			})

			It("should allow Admin and Editor roles for editor-allowed actions", func() {
				// Test with action types that allow both Admin and Editor roles
				testBenthosInput := models.TestBenthosInput
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, &testBenthosInput, models.Action)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, &testBenthosInput, models.Action)).To(BeFalse())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleEditor, &testBenthosInput, models.Action)).To(BeTrue())

				editDataFlowComponent := models.EditDataFlowComponent
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, &editDataFlowComponent, models.Action)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, &editDataFlowComponent, models.Action)).To(BeFalse())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleEditor, &editDataFlowComponent, models.Action)).To(BeTrue())
			})

			It("should deny for unknown action types", func() {
				// Create a custom action type that doesn't exist in the map
				unknownAction := models.ActionType("non-existent-action")
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, &unknownAction, models.Action)).To(BeFalse())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, &unknownAction, models.Action)).To(BeFalse())
			})
		})

		Context("when message type is not Action", func() {
			It("should still check role permissions for the action type", func() {
				// Even if message type is not Action, if action type is provided, it should check permissions
				deployConnection := models.DeployConnection
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, &deployConnection, models.Status)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, &deployConnection, models.Status)).To(BeFalse())

				getConnectionNotes := models.GetConnectionNotes
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleAdmin, &getConnectionNotes, models.Subscribe)).To(BeTrue())
				Expect(validator.IsRoleAllowedForActionAndMessageType(validator.RoleViewer, &getConnectionNotes, models.Subscribe)).To(BeTrue())
			})
		})
	})

	Describe("IsAllowedForAction", func() {
		Context("when checking Get Actions group", func() {
			It("should allow appropriate roles for get actions", func() {
				// Test get-connection-notes which allows Admin, Viewer, and Editor
				getConnectionNotes := models.GetConnectionNotes
				Expect(validator.IsAllowedForAction(validator.RoleAdmin, getConnectionNotes)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleViewer, getConnectionNotes)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleEditor, getConnectionNotes)).To(BeTrue())

				// Test get-audit-log which only allows Admin
				getAuditLog := models.GetAuditLog
				Expect(validator.IsAllowedForAction(validator.RoleAdmin, getAuditLog)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleViewer, getAuditLog)).To(BeFalse())
				Expect(validator.IsAllowedForAction(validator.RoleEditor, getAuditLog)).To(BeFalse())
			})
		})

		Context("when checking Edit Actions group", func() {
			It("should allow Admin and Editor roles for edit actions", func() {
				editConnection := models.EditConnection
				Expect(validator.IsAllowedForAction(validator.RoleAdmin, editConnection)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleEditor, editConnection)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleViewer, editConnection)).To(BeFalse())

				editInstance := models.EditInstance
				Expect(validator.IsAllowedForAction(validator.RoleAdmin, editInstance)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleEditor, editInstance)).To(BeFalse())
				Expect(validator.IsAllowedForAction(validator.RoleViewer, editInstance)).To(BeFalse())
			})
		})

		Context("when checking Delete Actions group", func() {
			It("should allow Admin and Editor roles for delete actions", func() {
				deleteConnection := models.DeleteConnection
				Expect(validator.IsAllowedForAction(validator.RoleAdmin, deleteConnection)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleEditor, deleteConnection)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleViewer, deleteConnection)).To(BeFalse())
			})
		})

		Context("when checking Deploy Actions group", func() {
			It("should allow Admin and Editor roles for deploy actions", func() {
				deployConnection := models.DeployConnection
				Expect(validator.IsAllowedForAction(validator.RoleAdmin, deployConnection)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleEditor, deployConnection)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleViewer, deployConnection)).To(BeFalse())
			})
		})

		Context("when checking Test Actions group", func() {
			It("should allow Admin and Editor roles for test actions", func() {
				testBenthosInput := models.TestBenthosInput
				Expect(validator.IsAllowedForAction(validator.RoleAdmin, testBenthosInput)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleEditor, testBenthosInput)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleViewer, testBenthosInput)).To(BeFalse())
			})
		})

		Context("when checking System Actions group", func() {
			It("should only allow Admin role for system actions", func() {
				upgradeCompanion := models.UpgradeCompanion
				Expect(validator.IsAllowedForAction(validator.RoleAdmin, upgradeCompanion)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleEditor, upgradeCompanion)).To(BeFalse())
				Expect(validator.IsAllowedForAction(validator.RoleViewer, upgradeCompanion)).To(BeFalse())
			})
		})

		Context("when checking ungrouped actions", func() {
			It("should handle unknown and dummy actions correctly", func() {
				// Test unknown action (should allow Admin, Viewer, Editor)
				unknownAction := models.ActionType("unknown")
				Expect(validator.IsAllowedForAction(validator.RoleAdmin, unknownAction)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleViewer, unknownAction)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleEditor, unknownAction)).To(BeTrue())

				// Test dummy action (should allow Admin, Viewer, Editor)
				dummyAction := models.ActionType("dummy")
				Expect(validator.IsAllowedForAction(validator.RoleAdmin, dummyAction)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleViewer, dummyAction)).To(BeTrue())
				Expect(validator.IsAllowedForAction(validator.RoleEditor, dummyAction)).To(BeTrue())
			})

			It("should deny access for non-existent actions", func() {
				nonExistentAction := models.ActionType("non-existent-action")
				Expect(validator.IsAllowedForAction(validator.RoleAdmin, nonExistentAction)).To(BeFalse())
				Expect(validator.IsAllowedForAction(validator.RoleViewer, nonExistentAction)).To(BeFalse())
				Expect(validator.IsAllowedForAction(validator.RoleEditor, nonExistentAction)).To(BeFalse())
			})
		})
	})
})
