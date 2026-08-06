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

package datamodel_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel"
)

// benthos-umh's UNS schema validator finds a contract's schemas by building
// schemaPrefix := ref.Full + "-" from the contract name it reads off the
// topic, then selects every registry subject starting with it. The two
// repos agree iff generateSubjectName produces, for every contract name
// umh-core creates, a subject that begins with contractName + "-" exactly.
var _ = Describe("generateSubjectName cross-repo subject-prefix invariant", func() {
	type invariantCase struct {
		contractName string
		version      string
		payloadShape string
	}

	DescribeTable("every contract name the product creates yields a subject beginning with contractName + \"-\"",
		func(tc invariantCase) {
			subject := datamodel.GenerateSubjectNameForTest(tc.contractName, tc.version, tc.payloadShape)

			// The whole string is asserted, not strings.HasPrefix: a prefix
			// check would still pass if an extra version segment were
			// inserted between the contract name and the payload shape,
			// which is exactly the regression this test exists to catch.
			Expect(subject).To(Equal(tc.contractName + "-" + tc.payloadShape))
		},
		Entry("newly created model at v1_0", invariantCase{
			contractName: "_pump_v1_0",
			version:      "v1_0",
			payloadShape: "timeseries-number",
		}),
		Entry("after a minor bump", invariantCase{
			contractName: "_pump_v1_1",
			version:      "v1_1",
			payloadShape: "timeseries-number",
		}),
		Entry("multi-digit major and minor", invariantCase{
			contractName: "_pump_v10_3",
			version:      "v10_3",
			payloadShape: "timeseries-number",
		}),
		Entry("digits in the model name itself", invariantCase{
			contractName: "_line_2_v1_1",
			version:      "v1_1",
			payloadShape: "timeseries-number",
		}),
	)

	// A hand-written contract with no "_v" at all takes generateSubjectName's
	// other branch and inserts a version segment, so the resulting subject
	// does not start with contractName + "-". This is a genuine mismatch,
	// but it is unreachable from any contract the product creates:
	// config.DataContractNameFor always produces "_" + modelName + "_" +
	// versionKey, and the version key grammar (^v\d+(_\d+)?$) guarantees a
	// "_v" substring, so every product-created contract name takes the
	// satisfying branch. It is also unreachable from benthos-umh's side:
	// its own contract-ref parser requires a _vN or _vN_M suffix, and a
	// contract without one is treated as unversioned and bypasses schema
	// validation before ever reaching the prefix fetch this invariant
	// protects.
	It("does not hold for a contract with no _v segment, which the product never creates", func() {
		subject := datamodel.GenerateSubjectNameForTest("_myfoo", "v1_0", "timeseries-number")

		Expect(subject).To(Equal("_myfoo_v1_0-timeseries-number"))
		Expect(subject).ToNot(HavePrefix("_myfoo-"))
	})
})
