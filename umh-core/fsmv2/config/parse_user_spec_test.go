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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
)

type parseTestConfig struct {
	ChildrenCount int `yaml:"children_count"`
}

var _ = Describe("ParseUserSpec", func() {
	It("reports a value of the wrong type as a config error", func() {
		_, err := config.ParseUserSpec[parseTestConfig](config.UserSpec{
			Config: "children_count: not-a-number",
		})
		Expect(err).To(HaveOccurred())

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue(),
			"a value of the wrong type is the user's mistake, so it must be a *config.ConfigError, got %T", err)
		Expect(err.Error()).To(ContainSubstring("into int"),
			"the message must keep yaml's reason, which says the value is not an int")
	})
})
