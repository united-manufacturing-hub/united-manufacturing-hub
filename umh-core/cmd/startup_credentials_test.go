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

package main

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/managementconsole"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// moduleRequiringCredentials stands in for the registered Management Console
// module, which refuses to start without a relay URL and an auth token. It
// records in called whether startup called it.
func moduleRequiringCredentials(called *bool) managementconsole.StartFunc {
	return func(d managementconsole.Deps) (managementconsole.Result, error) {
		*called = true

		var missing []error
		if d.Config.RelayURL == "" {
			missing = append(missing, errors.New("Config.RelayURL is empty"))
		}

		if d.Config.AuthToken == "" {
			missing = append(missing, errors.New("Config.AuthToken is empty"))
		}

		if len(missing) > 0 {
			return managementconsole.Result{}, errors.Join(missing...)
		}

		return managementconsole.Result{Children: []fsmv2config.ChildSpec{{Name: "connection", WorkerType: "test-worker", Enabled: true}}}, nil
	}
}

var _ = Describe("Startup without relay credentials", func() {
	BeforeEach(func() {
		restoreStartupSeams()
	})

	AfterEach(func() {
		restoreStartupSeams()
	})

	It("checks that startup runs the supervisor without the module's children and never starts the module when API_URL and AUTH_TOKEN are empty", func() {
		got := installStubBuild()
		noopRouter()

		supStarted := make(chan struct{}, 1)
		installStartProbe(supStarted)

		var called bool
		startManagementConsoleConnection = moduleRequiringCredentials(&called)

		handle, err := runStartup(&config.FullConfig{})
		Expect(err).NotTo(HaveOccurred())
		defer handle.cleanup()

		Expect(called).To(BeFalse(), "the module must not start without relay credentials")
		Expect(*got).To(BeEmpty())
		Eventually(supStarted, time.Second).Should(Receive())
	})
})
