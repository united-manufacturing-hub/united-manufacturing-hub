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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
)

var _ = Describe("ValidateComponentName", func() {
	Context("Valid component names", func() {
		It("should accept lowercase letters only", func() {
			err := actions.ValidateComponentName("validname")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept numbers only", func() {
			err := actions.ValidateComponentName("123456")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept mix of lowercase letters and numbers", func() {
			err := actions.ValidateComponentName("component123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept hyphens in the middle", func() {
			err := actions.ValidateComponentName("valid-name")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept underscores in the middle", func() {
			err := actions.ValidateComponentName("valid_name")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept complex valid names", func() {
			err := actions.ValidateComponentName("my-component_123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names starting with letters", func() {
			err := actions.ValidateComponentName("a123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names starting with numbers", func() {
			err := actions.ValidateComponentName("1component")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names ending with letters", func() {
			err := actions.ValidateComponentName("123a")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names ending with numbers", func() {
			err := actions.ValidateComponentName("component1")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Invalid component names", func() {
		It("should reject empty names", func() {
			err := actions.ValidateComponentName("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Name cannot be empty"))
		})

		It("should reject names starting with hyphen", func() {
			err := actions.ValidateComponentName("-component")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Name has to start and end with a letter or number"))
		})

		It("should reject names starting with underscore", func() {
			err := actions.ValidateComponentName("_component")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Name has to start and end with a letter or number"))
		})

		It("should reject names starting with hyphen", func() {
			err := actions.ValidateComponentName("-component")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Name has to start and end with a letter or number"))
		})

		It("should reject names ending with hyphen", func() {
			err := actions.ValidateComponentName("component-")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Name has to start and end with a letter or number"))
		})

		It("should reject names ending with underscore", func() {
			err := actions.ValidateComponentName("component_")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Name has to start and end with a letter or number"))
		})

		It("should reject uppercase letters", func() {
			err := actions.ValidateComponentName("Component")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Only lowercase letters, numbers, dashes and underscores are allowed"))
		})

		It("should reject special characters", func() {
			err := actions.ValidateComponentName("component@123")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Only lowercase letters, numbers, dashes and underscores are allowed"))
		})

		It("should reject spaces", func() {
			err := actions.ValidateComponentName("component name")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Only lowercase letters, numbers, dashes and underscores are allowed"))
		})

		It("should reject dots", func() {
			err := actions.ValidateComponentName("component.name")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Only lowercase letters, numbers, dashes and underscores are allowed"))
		})

		It("should reject slashes", func() {
			err := actions.ValidateComponentName("component/name")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Only lowercase letters, numbers, dashes and underscores are allowed"))
		})

		It("should reject exclamation marks", func() {
			err := actions.ValidateComponentName("component!")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Only lowercase letters, numbers, dashes and underscores are allowed"))
		})

		It("should reject question marks", func() {
			err := actions.ValidateComponentName("component?")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Only lowercase letters, numbers, dashes and underscores are allowed"))
		})

		It("should reject hash symbols", func() {
			err := actions.ValidateComponentName("component#123")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Only lowercase letters, numbers, dashes and underscores are allowed"))
		})
	})

	Context("Edge cases", func() {
		It("should accept single character names", func() {
			err := actions.ValidateComponentName("a")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept single digit names", func() {
			err := actions.ValidateComponentName("1")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject single hyphen", func() {
			err := actions.ValidateComponentName("-")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Name has to start and end with a letter or number"))
		})

		It("should reject single underscore", func() {
			err := actions.ValidateComponentName("_")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Name has to start and end with a letter or number"))
		})

		It("should accept very long valid names", func() {
			longName := "a" + string(make([]byte, 100)) // Create a long string
			for i := range longName[1:] {
				longName = longName[:i+1] + "a" + longName[i+2:]
			}
			err := actions.ValidateComponentName(longName)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
