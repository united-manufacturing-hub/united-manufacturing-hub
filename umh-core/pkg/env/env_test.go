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

package env

import (
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetAsBoolStrict", func() {
	const key = "UMH_TEST_GET_AS_BOOL_STRICT"

	var envPrior string
	var envPresent bool

	BeforeEach(func() {
		envPrior, envPresent = os.LookupEnv(key)
		_ = os.Unsetenv(key)
	})

	AfterEach(func() {
		if envPresent {
			_ = os.Setenv(key, envPrior)
		} else {
			_ = os.Unsetenv(key)
		}
	})

	// GetAsBoolStrict returns an error naming the variable for every value
	// outside the recognised true/false spellings, while unset is not an
	// error.
	It("reports unrecognised values", func() {
		// Unset is not an error: callers use unset as their normal default.
		parsed, err := GetAsBoolStrict(key, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(parsed).To(BeFalse())

		for _, value := range []string{"true", "on", "ON", "false", "off"} {
			_ = os.Setenv(key, value)

			parsed, err := GetAsBoolStrict(key, false)
			Expect(err).NotTo(HaveOccurred(), "value %q", value)
			Expect(parsed).To(Equal(value == "true" || value == "on" || value == "ON"), "value %q", value)
		}

		for _, value := range []string{"maybe", "ture"} {
			_ = os.Setenv(key, value)

			parsed, err := GetAsBoolStrict(key, false)
			Expect(err).To(HaveOccurred(), "an unrecognised value must be reported, not swallowed (value %q)", value)
			Expect(err.Error()).To(ContainSubstring(key), "the error must name the variable (value %q)", value)
			Expect(err.Error()).To(ContainSubstring(value), "the error must quote the offending value (value %q)", value)
			Expect(strings.Count(err.Error(), value)).To(Equal(1), "the value may appear in the error only once")
			Expect(parsed).To(BeFalse(), "value %q", value)
		}
	})

	// GetAsBoolStrict returns defaultValue alongside the error, so a caller
	// that only logs the error still ends up where GetAsBool would have put
	// it.
	It("honours the default on an unrecognised value", func() {
		_ = os.Setenv(key, "maybe")

		parsed, err := GetAsBoolStrict(key, true)
		Expect(err).To(HaveOccurred())
		Expect(parsed).To(BeTrue(), "the parsed value alongside the error must be defaultValue, so an error-ignoring caller matches GetAsBool")
	})
})
