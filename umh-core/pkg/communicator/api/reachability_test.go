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

package api_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

var _ = Describe("Checking API reachability", func() {
	const apiUrl = "https://management.umh.app/api"
	var log *zap.SugaredLogger

	BeforeEach(func() {
		log = logger.For("reachability_test")
	})

	When("API is reachable", func() {
		It("can be reached", func() {
			Eventually(func() bool {
				return api.CheckIfAPIIsReachable(false, apiUrl, log)
			}, "5m", "1s").Should(BeTrue())
		})
	})
})
