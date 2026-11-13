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

// Which monitor backend NewDefaultBenthosService stores for a given value of
// USE_FSMV2_BENTHOS_METRICS. Every spelling LookupBool accepts must
// select a backend, so the table below pins them next to pkg/env's own specs.

package benthos

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	benthos_monitor_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	fsmv2benthosmetrics "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/benthosmetrics"
)

var _ = Describe("Backend selection", func() {
	var envPrior string
	var envPresent bool

	BeforeEach(func() {
		// Save and clear any prior value so a var preset in the shell or by an
		// earlier spec does not leak into these specs; AfterEach restores it.
		envPrior, envPresent = os.LookupEnv(envUseFsmv2BenthosMetrics)
		_ = os.Unsetenv(envUseFsmv2BenthosMetrics)
	})

	AfterEach(func() {
		if envPresent {
			_ = os.Setenv(envUseFsmv2BenthosMetrics, envPrior)
		} else {
			_ = os.Unsetenv(envUseFsmv2BenthosMetrics)
		}
	})

	DescribeTable("the backend for a flag value",
		func(value string, wantFSMv2, wantReadErr bool) {
			if value != "" {
				_ = os.Setenv(envUseFsmv2BenthosMetrics, value)
			}

			// A recognised spelling must read without an error, so a reader
			// that silently drops one fails here, not only in pkg/env's specs.
			_, err := FSMv2BenthosMetricsEnabled()
			if wantReadErr {
				Expect(err).To(HaveOccurred(), "value %q is not a boolean", value)
			} else {
				Expect(err).NotTo(HaveOccurred(),
					"value %q is a spelling LookupBool accepts", value)
			}

			svc := NewDefaultBenthosService("flag-" + value)

			if wantFSMv2 {
				Expect(svc.benthosMonitorManager).To(BeAssignableToTypeOf(fsmv2benthosmetrics.NewManager("x")),
					"value %q must select the FSMv2 Benthos metrics manager", value)
				_, ok := svc.benthosMonitorManager.(*benthos_monitor_fsm.BenthosMonitorManager)
				Expect(ok).To(BeFalse(), "value %q must not select the FSMv1 manager", value)
			} else {
				_, ok := svc.benthosMonitorManager.(*benthos_monitor_fsm.BenthosMonitorManager)
				Expect(ok).To(BeTrue(), "value %q must select the FSMv1 BenthosMonitorManager", value)
			}
		},
		Entry("unset", "", false, false),
		Entry("true", "true", true, false),
		// The remaining spellings are the ones LookupBool accepts that
		// pkg/env's own specs do not already pin (env_test.go covers true, on,
		// ON, false and off): every one of them must select a backend too, so a
		// reader that silently drops a spelling fails here.
		Entry("1", "1", true, false),
		Entry("yes", "yes", true, false),
		Entry("y", "y", true, false),
		Entry("TRUE", "TRUE", true, false),
		Entry("0", "0", false, false),
		Entry("no", "no", false, false),
		Entry("n", "n", false, false),
		Entry("FALSE", "FALSE", false, false),
		// A non-boolean value selects FSMv1; cmd/main.go logs the warning.
		Entry("typo", "maybe", false, true),
	)

	// A caller-supplied WithMonitorManager manager wins over the
	// flag-selected default, whatever the flag holds.
	It("lets WithMonitorManager override the flag", func() {
		_ = os.Setenv(envUseFsmv2BenthosMetrics, "true")

		injected := benthos_monitor_fsm.NewBenthosMonitorManager("injected-manager")
		svc := NewDefaultBenthosService("flag-on-but-injected", WithMonitorManager(injected))

		mgr, ok := svc.benthosMonitorManager.(*benthos_monitor_fsm.BenthosMonitorManager)
		Expect(ok).To(BeTrue(), "WithMonitorManager must override the flag-selected FSMv2 backend")
		Expect(mgr).To(BeIdenticalTo(injected), "the injected manager must be the one stored")
	})

	// A nil WithMonitorManager manager is no override either: nil means
	// "use the flag's choice", whatever the flag holds.
	It("falls back to the flag-selected backend on WithMonitorManager(nil)", func() {
		_ = os.Setenv(envUseFsmv2BenthosMetrics, "true")
		svcOn := NewDefaultBenthosService("nil-manager-flag-on", WithMonitorManager(nil))
		Expect(svcOn.benthosMonitorManager).To(BeAssignableToTypeOf(fsmv2benthosmetrics.NewManager("x")),
			"nil must fall back to the flag-selected FSMv2 backend")

		_ = os.Unsetenv(envUseFsmv2BenthosMetrics)
		svcOff := NewDefaultBenthosService("nil-manager-flag-off", WithMonitorManager(nil))
		mgr, ok := svcOff.benthosMonitorManager.(*benthos_monitor_fsm.BenthosMonitorManager)
		Expect(ok).To(BeTrue(), "nil must fall back to the flag-selected FSMv1 backend")
		Expect(mgr).NotTo(BeNil(), "the FSMv1 fallback must be a real manager, not a nil pointer")
	})
})
