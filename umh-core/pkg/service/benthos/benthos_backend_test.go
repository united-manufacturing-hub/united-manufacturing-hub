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

package benthos

// These specs pin the backend NewDefaultBenthosService stores based on
// envUseFsmv2BenthosMonitor: unset or false keeps the byte-identical fsmv1
// BenthosMonitorManager; a true value (as accepted by env.GetAsBool, which
// honours on/off, yes/no, y/n, 1/0 and true/false case-insensitively)
// stores the fsmv2 adapter WorkerManager. The two backends are
// distinguished by their concrete runtime type, which is why the specs
// assert that type directly rather than via a helper.

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	benthos_monitor_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/adapter"
	fsmv2benthosmonitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

var _ = Describe("USE_FSMV2_BENTHOS_MONITOR flag wiring", func() {
	var envPrior string
	var envPresent bool

	BeforeEach(func() {
		// Save and clear any prior value so a var preset in the shell or by an
		// earlier spec does not leak into these specs; AfterEach restores it.
		envPrior, envPresent = os.LookupEnv(envUseFsmv2BenthosMonitor)
		_ = os.Unsetenv(envUseFsmv2BenthosMonitor)
	})

	AfterEach(func() {
		if envPresent {
			_ = os.Setenv(envUseFsmv2BenthosMonitor, envPrior)
		} else {
			_ = os.Unsetenv(envUseFsmv2BenthosMonitor)
		}
	})

	Describe("backend selection", func() {
		It("stores the fsmv1 manager when USE_FSMV2_BENTHOS_MONITOR is unset (FF-off default)", func() {
			// env intentionally unset in BeforeEach
			svc := NewDefaultBenthosService("flag-off-benthos")

			_, ok := svc.benthosMonitorManager.(*benthos_monitor_fsm.BenthosMonitorManager)
			Expect(ok).To(BeTrue(),
				"unset USE_FSMV2_BENTHOS_MONITOR must keep the byte-identical fsmv1 monitor manager")
		})

		It("stores the fsmv2 adapter manager when USE_FSMV2_BENTHOS_MONITOR=true", func() {
			_ = os.Setenv(envUseFsmv2BenthosMonitor, "true")
			defer func() { _ = os.Unsetenv(envUseFsmv2BenthosMonitor) }()

			svc := NewDefaultBenthosService("flag-on-benthos")

			_, ok := svc.benthosMonitorManager.(*adapter.WorkerManager[config.BenthosMonitorConfig, simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus]])
			Expect(ok).To(BeTrue(),
				"USE_FSMV2_BENTHOS_MONITOR=true must select the fsmv2 adapter manager")
		})

		// Every spelling env.GetAsBool accepts must select the fsmv2 backend. "ON"
		// is the one that matters most: it is what the CPU measurement rig uses,
		// and every sibling flag reads through env.GetAsBool the same way
		// (USE_FSMV2_PROTOCOL_CONVERTER, cmd/main.go). Reading
		// this flag with strconv.ParseBool instead of env.GetAsBool rejected "ON",
		// so a benchmark run set the flag on, silently got fsmv1 on both arms, and
		// measured no difference. Unit tests using only "true" could not see it.
		for _, truthy := range []string{"true", "TRUE", "1", "on", "ON", "On", "yes", "y"} {
			value := truthy

			It("selects the fsmv2 backend for the truthy value "+value, func() {
				_ = os.Setenv(envUseFsmv2BenthosMonitor, value)
				defer func() { _ = os.Unsetenv(envUseFsmv2BenthosMonitor) }()

				svc := NewDefaultBenthosService("flag-truthy-" + value)

				_, ok := svc.benthosMonitorManager.(*adapter.WorkerManager[config.BenthosMonitorConfig, simple.Status[fsmv2benthosmonitor.BenthosMonitorStatus]])
				Expect(ok).To(BeTrue(),
					"USE_FSMV2_BENTHOS_MONITOR=%s must select the fsmv2 adapter manager (env.GetAsBool convention)", value)
			})
		}

		for _, falsy := range []string{"false", "FALSE", "0", "off", "OFF", "no", "n"} {
			value := falsy

			It("keeps the fsmv1 backend for the falsy value "+value, func() {
				_ = os.Setenv(envUseFsmv2BenthosMonitor, value)
				defer func() { _ = os.Unsetenv(envUseFsmv2BenthosMonitor) }()

				svc := NewDefaultBenthosService("flag-falsy-" + value)

				_, ok := svc.benthosMonitorManager.(*benthos_monitor_fsm.BenthosMonitorManager)
				Expect(ok).To(BeTrue(),
					"USE_FSMV2_BENTHOS_MONITOR=%s must keep the fsmv1 manager", value)
			})
		}

		It("keeps the fsmv1 manager for an explicit false value", func() {
			_ = os.Setenv(envUseFsmv2BenthosMonitor, "false")
			defer func() { _ = os.Unsetenv(envUseFsmv2BenthosMonitor) }()

			svc := NewDefaultBenthosService("flag-explicit-off-benthos")

			_, ok := svc.benthosMonitorManager.(*benthos_monitor_fsm.BenthosMonitorManager)
			Expect(ok).To(BeTrue(),
				"an explicit false must behave like unset (fsmv1 manager)")
		})

		It("treats an unrecognized value as off (fsmv1), not as on", func() {
			_ = os.Setenv(envUseFsmv2BenthosMonitor, "ture")
			defer func() { _ = os.Unsetenv(envUseFsmv2BenthosMonitor) }()

			svc := NewDefaultBenthosService("flag-typo-benthos")

			_, ok := svc.benthosMonitorManager.(*benthos_monitor_fsm.BenthosMonitorManager)
			Expect(ok).To(BeTrue(),
				"an unrecognized boolean value must not silently select a backend; it must stay on the fsmv1 default")
		})

		It("lets WithMonitorManager override the flag-selected backend", func() {
			_ = os.Setenv(envUseFsmv2BenthosMonitor, "true")
			defer func() { _ = os.Unsetenv(envUseFsmv2BenthosMonitor) }()

			injected := benthos_monitor_fsm.NewBenthosMonitorManager("injected-manager")
			svc := NewDefaultBenthosService("flag-on-but-injected", WithMonitorManager(injected))

			mgr, ok := svc.benthosMonitorManager.(*benthos_monitor_fsm.BenthosMonitorManager)
			Expect(ok).To(BeTrue(),
				"WithMonitorManager must override the flag-selected fsmv2 backend")
			Expect(mgr).To(BeIdenticalTo(injected),
				"the injected manager must be the one stored, not the flag-selected default")
		})
	})
})

var _ = Describe("benthos_monitor worker registered in production (D8)", func() {
	It("registers the benthos_monitor worker type through benthos.go's production import", func() {
		// The fsmv2 worker self-registers on import; benthos.go imports it (line 44)
		// and constructs the adapter manager under USE_FSMV2_BENTHOS_MONITOR. This
		// spec lives in package benthos, so the production import graph — not a
		// test file's own import — is what must have registered the worker. If a
		// refactor removes the production constructor call (or the import), FF-on
		// ships nothing while the CPU rig counts a fake win; this gate goes red.
		registered := false
		for _, wt := range factory.ListRegisteredTypes() {
			if wt == fsmv2benthosmonitor.WorkerType {
				registered = true
				break
			}
		}
		Expect(registered).To(BeTrue(),
			"worker type %q must be registered via the production import of pkg/service/benthos, not by a test file", fsmv2benthosmonitor.WorkerType)
	})
})
