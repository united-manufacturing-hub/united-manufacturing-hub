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

package config_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

const (
	errEmptyName         = "name cannot be empty"
	errInvalidStartEnd   = "name has to start and end with a letter or number"
	errInvalidCharacters = "only letters, numbers, dashes and underscores are allowed"
)

var _ = Describe("ValidateComponentName", func() {
	Context("Valid component names", func() {
		It("should accept lowercase letters only", func() {
			err := config.ValidateComponentName("validname")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept uppercase letters only", func() {
			err := config.ValidateComponentName("VALIDNAME")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept mixed case letters", func() {
			err := config.ValidateComponentName("ValidName")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept numbers only", func() {
			err := config.ValidateComponentName("123456")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept mix of lowercase letters and numbers", func() {
			err := config.ValidateComponentName("component123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept mix of uppercase letters and numbers", func() {
			err := config.ValidateComponentName("Component123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept mix of mixed case letters and numbers", func() {
			err := config.ValidateComponentName("MyComponent123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept hyphens in the middle", func() {
			err := config.ValidateComponentName("valid-name")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept underscores in the middle", func() {
			err := config.ValidateComponentName("valid_name")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept complex valid names", func() {
			err := config.ValidateComponentName("my-component_123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept complex valid names with mixed case", func() {
			err := config.ValidateComponentName("My-Component_123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names starting with lowercase letters", func() {
			err := config.ValidateComponentName("a123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names starting with uppercase letters", func() {
			err := config.ValidateComponentName("A123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names starting with numbers", func() {
			err := config.ValidateComponentName("1component")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names ending with lowercase letters", func() {
			err := config.ValidateComponentName("123a")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names ending with uppercase letters", func() {
			err := config.ValidateComponentName("123A")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names ending with numbers", func() {
			err := config.ValidateComponentName("component1")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Invalid component names", func() {
		It("should reject empty names", func() {
			err := config.ValidateComponentName("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errEmptyName))
		})

		It("should reject names starting with hyphen", func() {
			err := config.ValidateComponentName("-component")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidStartEnd))
		})

		It("should reject names starting with underscore", func() {
			err := config.ValidateComponentName("_component")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidStartEnd))
		})

		It("should reject names ending with hyphen", func() {
			err := config.ValidateComponentName("component-")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidStartEnd))
		})

		It("should reject names ending with underscore", func() {
			err := config.ValidateComponentName("component_")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidStartEnd))
		})

		It("should reject special characters", func() {
			err := config.ValidateComponentName("component@123")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidCharacters))
		})

		It("should reject spaces", func() {
			err := config.ValidateComponentName("component name")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidCharacters))
		})

		It("should reject dots", func() {
			err := config.ValidateComponentName("component.name")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidCharacters))
		})

		It("should reject slashes", func() {
			err := config.ValidateComponentName("component/name")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidCharacters))
		})

		It("should reject exclamation marks", func() {
			err := config.ValidateComponentName("component!")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidCharacters))
		})

		It("should reject question marks", func() {
			err := config.ValidateComponentName("component?")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidCharacters))
		})

		It("should reject hash symbols", func() {
			err := config.ValidateComponentName("component#123")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidCharacters))
		})
	})

	Context("Edge cases", func() {
		It("should accept single lowercase character names", func() {
			err := config.ValidateComponentName("a")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept single uppercase character names", func() {
			err := config.ValidateComponentName("A")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept single digit names", func() {
			err := config.ValidateComponentName("1")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject single hyphen", func() {
			err := config.ValidateComponentName("-")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidStartEnd))
		})

		It("should reject single underscore", func() {
			err := config.ValidateComponentName("_")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(errInvalidStartEnd))
		})

		It("should accept long names", func() {
			longName := strings.Repeat("a", 255)
			err := config.ValidateComponentName(longName)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
