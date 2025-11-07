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


package location_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/location"
)

var _ = Describe("Location Computation", func() {
	Describe("MergeLocations", func() {
		Context("when parent has enterprise and site", func() {
			It("should merge parent and child correctly", func() {
				parent := []location.LocationLevel{
					{Type: "enterprise", Value: "ACME"},
					{Type: "site", Value: "Factory-1"},
				}
				child := []location.LocationLevel{
					{Type: "line", Value: "Line-A"},
					{Type: "cell", Value: "Cell-5"},
				}

				result := location.MergeLocations(parent, child)

				Expect(result).To(HaveLen(4))
				Expect(result[0]).To(Equal(location.LocationLevel{Type: "enterprise", Value: "ACME"}))
				Expect(result[1]).To(Equal(location.LocationLevel{Type: "site", Value: "Factory-1"}))
				Expect(result[2]).To(Equal(location.LocationLevel{Type: "line", Value: "Line-A"}))
				Expect(result[3]).To(Equal(location.LocationLevel{Type: "cell", Value: "Cell-5"}))
			})
		})

		Context("when only parent is provided", func() {
			It("should return parent locations", func() {
				parent := []location.LocationLevel{
					{Type: "enterprise", Value: "ACME"},
					{Type: "site", Value: "Factory-1"},
				}
				child := []location.LocationLevel{}

				result := location.MergeLocations(parent, child)

				Expect(result).To(HaveLen(2))
				Expect(result[0]).To(Equal(location.LocationLevel{Type: "enterprise", Value: "ACME"}))
				Expect(result[1]).To(Equal(location.LocationLevel{Type: "site", Value: "Factory-1"}))
			})
		})

		Context("when only child is provided", func() {
			It("should return child locations", func() {
				parent := []location.LocationLevel{}
				child := []location.LocationLevel{
					{Type: "line", Value: "Line-A"},
					{Type: "cell", Value: "Cell-5"},
				}

				result := location.MergeLocations(parent, child)

				Expect(result).To(HaveLen(2))
				Expect(result[0]).To(Equal(location.LocationLevel{Type: "line", Value: "Line-A"}))
				Expect(result[1]).To(Equal(location.LocationLevel{Type: "cell", Value: "Cell-5"}))
			})
		})

		Context("when both are empty", func() {
			It("should return empty slice", func() {
				parent := []location.LocationLevel{}
				child := []location.LocationLevel{}

				result := location.MergeLocations(parent, child)

				Expect(result).To(BeEmpty())
			})
		})

		Context("when child has duplicate type as parent", func() {
			It("should keep both occurrences in order", func() {
				parent := []location.LocationLevel{
					{Type: "enterprise", Value: "ACME"},
					{Type: "site", Value: "Factory-1"},
				}
				child := []location.LocationLevel{
					{Type: "site", Value: "Factory-2"},
					{Type: "line", Value: "Line-A"},
				}

				result := location.MergeLocations(parent, child)

				Expect(result).To(HaveLen(4))
				Expect(result[0]).To(Equal(location.LocationLevel{Type: "enterprise", Value: "ACME"}))
				Expect(result[1]).To(Equal(location.LocationLevel{Type: "site", Value: "Factory-1"}))
				Expect(result[2]).To(Equal(location.LocationLevel{Type: "site", Value: "Factory-2"}))
				Expect(result[3]).To(Equal(location.LocationLevel{Type: "line", Value: "Line-A"}))
			})
		})
	})

	Describe("FillISA95Gaps", func() {
		Context("when missing area in the middle", func() {
			It("should fill missing ISA-95 levels in correct order", func() {
				levels := []location.LocationLevel{
					{Type: "enterprise", Value: "ACME"},
					{Type: "site", Value: "Factory-1"},
					{Type: "line", Value: "Line-A"},
					{Type: "cell", Value: "Cell-5"},
				}

				result := location.FillISA95Gaps(levels)

				Expect(result).To(HaveLen(5))
				Expect(result[0]).To(Equal(location.LocationLevel{Type: "enterprise", Value: "ACME"}))
				Expect(result[1]).To(Equal(location.LocationLevel{Type: "site", Value: "Factory-1"}))
				Expect(result[2]).To(Equal(location.LocationLevel{Type: "area", Value: ""}))
				Expect(result[3]).To(Equal(location.LocationLevel{Type: "line", Value: "Line-A"}))
				Expect(result[4]).To(Equal(location.LocationLevel{Type: "cell", Value: "Cell-5"}))
			})
		})

		Context("when only child levels provided", func() {
			It("should fill enterprise, site, and area as empty", func() {
				levels := []location.LocationLevel{
					{Type: "line", Value: "Line-A"},
					{Type: "cell", Value: "Cell-5"},
				}

				result := location.FillISA95Gaps(levels)

				Expect(result).To(HaveLen(5))
				Expect(result[0]).To(Equal(location.LocationLevel{Type: "enterprise", Value: ""}))
				Expect(result[1]).To(Equal(location.LocationLevel{Type: "site", Value: ""}))
				Expect(result[2]).To(Equal(location.LocationLevel{Type: "area", Value: ""}))
				Expect(result[3]).To(Equal(location.LocationLevel{Type: "line", Value: "Line-A"}))
				Expect(result[4]).To(Equal(location.LocationLevel{Type: "cell", Value: "Cell-5"}))
			})
		})

		Context("when only parent levels provided", func() {
			It("should fill area, line, and cell as empty", func() {
				levels := []location.LocationLevel{
					{Type: "enterprise", Value: "ACME"},
					{Type: "site", Value: "Factory-1"},
				}

				result := location.FillISA95Gaps(levels)

				Expect(result).To(HaveLen(5))
				Expect(result[0]).To(Equal(location.LocationLevel{Type: "enterprise", Value: "ACME"}))
				Expect(result[1]).To(Equal(location.LocationLevel{Type: "site", Value: "Factory-1"}))
				Expect(result[2]).To(Equal(location.LocationLevel{Type: "area", Value: ""}))
				Expect(result[3]).To(Equal(location.LocationLevel{Type: "line", Value: ""}))
				Expect(result[4]).To(Equal(location.LocationLevel{Type: "cell", Value: ""}))
			})
		})

		Context("when all levels are already present", func() {
			It("should return all 5 levels unchanged", func() {
				levels := []location.LocationLevel{
					{Type: "enterprise", Value: "ACME"},
					{Type: "site", Value: "Factory-1"},
					{Type: "area", Value: "Area-North"},
					{Type: "line", Value: "Line-A"},
					{Type: "cell", Value: "Cell-5"},
				}

				result := location.FillISA95Gaps(levels)

				Expect(result).To(HaveLen(5))
				Expect(result).To(Equal(levels))
			})
		})

		Context("when empty slice provided", func() {
			It("should fill all 5 levels as empty", func() {
				levels := []location.LocationLevel{}

				result := location.FillISA95Gaps(levels)

				Expect(result).To(HaveLen(5))
				Expect(result[0]).To(Equal(location.LocationLevel{Type: "enterprise", Value: ""}))
				Expect(result[1]).To(Equal(location.LocationLevel{Type: "site", Value: ""}))
				Expect(result[2]).To(Equal(location.LocationLevel{Type: "area", Value: ""}))
				Expect(result[3]).To(Equal(location.LocationLevel{Type: "line", Value: ""}))
				Expect(result[4]).To(Equal(location.LocationLevel{Type: "cell", Value: ""}))
			})
		})

		Context("when levels are out of order", func() {
			It("should sort to ISA-95 order and fill gaps", func() {
				levels := []location.LocationLevel{
					{Type: "cell", Value: "Cell-5"},
					{Type: "enterprise", Value: "ACME"},
					{Type: "line", Value: "Line-A"},
				}

				result := location.FillISA95Gaps(levels)

				Expect(result).To(HaveLen(5))
				Expect(result[0]).To(Equal(location.LocationLevel{Type: "enterprise", Value: "ACME"}))
				Expect(result[1]).To(Equal(location.LocationLevel{Type: "site", Value: ""}))
				Expect(result[2]).To(Equal(location.LocationLevel{Type: "area", Value: ""}))
				Expect(result[3]).To(Equal(location.LocationLevel{Type: "line", Value: "Line-A"}))
				Expect(result[4]).To(Equal(location.LocationLevel{Type: "cell", Value: "Cell-5"}))
			})
		})
	})

	Describe("ComputeLocationPath", func() {
		Context("when all levels have values", func() {
			It("should join all non-empty values with dots", func() {
				levels := []location.LocationLevel{
					{Type: "enterprise", Value: "ACME"},
					{Type: "site", Value: "Factory-1"},
					{Type: "area", Value: "Area-North"},
					{Type: "line", Value: "Line-A"},
					{Type: "cell", Value: "Cell-5"},
				}

				result := location.ComputeLocationPath(levels)

				Expect(result).To(Equal("ACME.Factory-1.Area-North.Line-A.Cell-5"))
			})
		})

		Context("when some levels are empty in the middle", func() {
			It("should skip empty values (no double dots)", func() {
				levels := []location.LocationLevel{
					{Type: "enterprise", Value: "ACME"},
					{Type: "site", Value: "Factory-1"},
					{Type: "area", Value: ""},
					{Type: "line", Value: "Line-A"},
					{Type: "cell", Value: "Cell-5"},
				}

				result := location.ComputeLocationPath(levels)

				Expect(result).To(Equal("ACME.Factory-1.Line-A.Cell-5"))
			})
		})

		Context("when only parent levels have values", func() {
			It("should return path with parent values only", func() {
				levels := []location.LocationLevel{
					{Type: "enterprise", Value: "ACME"},
					{Type: "site", Value: "Factory-1"},
					{Type: "area", Value: ""},
					{Type: "line", Value: ""},
					{Type: "cell", Value: ""},
				}

				result := location.ComputeLocationPath(levels)

				Expect(result).To(Equal("ACME.Factory-1"))
			})
		})

		Context("when only child levels have values", func() {
			It("should return path with child values only", func() {
				levels := []location.LocationLevel{
					{Type: "enterprise", Value: ""},
					{Type: "site", Value: ""},
					{Type: "area", Value: ""},
					{Type: "line", Value: "Line-A"},
					{Type: "cell", Value: "Cell-5"},
				}

				result := location.ComputeLocationPath(levels)

				Expect(result).To(Equal("Line-A.Cell-5"))
			})
		})

		Context("when all levels are empty", func() {
			It("should return empty string", func() {
				levels := []location.LocationLevel{
					{Type: "enterprise", Value: ""},
					{Type: "site", Value: ""},
					{Type: "area", Value: ""},
					{Type: "line", Value: ""},
					{Type: "cell", Value: ""},
				}

				result := location.ComputeLocationPath(levels)

				Expect(result).To(BeEmpty())
			})
		})

		Context("when empty slice provided", func() {
			It("should return empty string", func() {
				levels := []location.LocationLevel{}

				result := location.ComputeLocationPath(levels)

				Expect(result).To(BeEmpty())
			})
		})
	})
})
