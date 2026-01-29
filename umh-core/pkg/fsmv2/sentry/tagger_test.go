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

package sentry_test

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"

	//nolint:revive // dot import for Ginkgo DSL
	. "github.com/onsi/ginkgo/v2"
	//nolint:revive // dot import for Gomega matchers
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseHierarchyPath", func() {

	Describe("FSMv2 format (parentheses notation)", func() {
		It("should parse app(application)/worker(communicator) correctly", func() {
			info := sentry.ParseHierarchyPath("app(application)/worker(communicator)")

			Expect(info.FSMVersion).To(Equal("v2"))
			Expect(info.WorkerType).To(Equal("communicator"))
		})
	})

	Describe("FSMv1 format (dot notation)", func() {
		It("should parse Enterprise.Site.Area.Line.WorkCell correctly", func() {
			info := sentry.ParseHierarchyPath("Enterprise.Site.Area.Line.WorkCell")

			Expect(info.FSMVersion).To(Equal("v1"))
			Expect(info.WorkerType).To(Equal("WorkCell"))
		})
	})

	Describe("Empty input", func() {
		It("should return unknown for empty string", func() {
			info := sentry.ParseHierarchyPath("")

			Expect(info.FSMVersion).To(Equal("unknown"))
			Expect(info.WorkerType).To(Equal("unknown"))
		})
	})
})
