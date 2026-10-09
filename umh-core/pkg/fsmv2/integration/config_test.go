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

package integration_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("Location integration tests", func() {
	Describe("Scenario 3: Location Computation", func() {
		It("should merge parent and child locations into complete ISA-95 path", func() {
			parentLocation := []config.LocationLevel{
				{Type: "enterprise", Value: "ACME"},
				{Type: "site", Value: "Factory-1"},
			}

			childLocation := []config.LocationLevel{
				{Type: "line", Value: "Line-A"},
				{Type: "cell", Value: "Cell-5"},
			}

			merged := config.MergeLocations(parentLocation, childLocation)
			Expect(merged).To(HaveLen(4))

			filled := config.NormalizeHierarchyLevels(merged)
			Expect(filled).To(HaveLen(5))

			Expect(filled[0].Type).To(Equal("enterprise"))
			Expect(filled[0].Value).To(Equal("ACME"))
			Expect(filled[1].Type).To(Equal("site"))
			Expect(filled[1].Value).To(Equal("Factory-1"))
			Expect(filled[2].Type).To(Equal("area"))
			Expect(filled[2].Value).To(Equal(""))
			Expect(filled[3].Type).To(Equal("line"))
			Expect(filled[3].Value).To(Equal("Line-A"))
			Expect(filled[4].Type).To(Equal("cell"))
			Expect(filled[4].Value).To(Equal("Cell-5"))

			path := config.ComputeLocationPath(filled)
			Expect(path).To(Equal("ACME.Factory-1.Line-A.Cell-5"))
			Expect(path).NotTo(ContainSubstring(".."))
		})
	})

	Describe("Scenario 7: Empty Locations", func() {
		It("should fill missing ISA-95 levels with empty strings and skip in path", func() {
			sparseLocation := []config.LocationLevel{
				{Type: "enterprise", Value: "ACME"},
				{Type: "line", Value: "Line-A"},
			}

			filled := config.NormalizeHierarchyLevels(sparseLocation)

			Expect(filled).To(HaveLen(5))
			Expect(filled[0].Type).To(Equal("enterprise"))
			Expect(filled[0].Value).To(Equal("ACME"))
			Expect(filled[1].Type).To(Equal("site"))
			Expect(filled[1].Value).To(Equal(""))
			Expect(filled[2].Type).To(Equal("area"))
			Expect(filled[2].Value).To(Equal(""))
			Expect(filled[3].Type).To(Equal("line"))
			Expect(filled[3].Value).To(Equal("Line-A"))
			Expect(filled[4].Type).To(Equal("cell"))
			Expect(filled[4].Value).To(Equal(""))

			path := config.ComputeLocationPath(filled)
			Expect(path).To(Equal("ACME.Line-A"))
			Expect(path).NotTo(ContainSubstring(".."))
		})
	})
})
