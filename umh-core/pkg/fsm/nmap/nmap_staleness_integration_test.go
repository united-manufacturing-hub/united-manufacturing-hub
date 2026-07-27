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

package nmap_test

// FSMv1 counterpart of the NMAP_BACKEND=fsmv2 stale-observation bug
// (pkg/fsmv2/nmap/nmap_staleness_integration_test.go). It mirrors the same
// scenario — a bridge connection edited from a healthy target to a known-bad
// one — and proves that the fsmv1 nmap backend does NOT exhibit the bug: after
// the edit it never reports the connection open from the old target's scan.
//
// Why fsmv1 is correct: the operational state is derived every reconcile from
// the observation the live S6-supervised nmap service produces for the CURRENT
// target. reconcileRunningStates only stays open while isNmapHealthy holds
// (a recent, non-errored scan from a running service — actions.go) and
// checkPortState still reads "open" (reconcile.go). Editing the target
// reconfigures/restarts that S6 service (UpdateNmapInS6Manager), so the next
// observation reflects the new target: while the restarted scanner has not yet
// completed a scan the instance drops to degraded, and once the bad target is
// scanned it reads closed. There is no per-name observation store that can
// serve a stale "open" of the old target, which is exactly what the fsmv2
// adapter does via its unchanged child ref.
//
// This drives the REAL read path: a real NmapManager (NewNmapManager) fed the
// edit through the SystemSnapshot config path (CurrentConfig.Internal.Nmap,
// the same channel ManagementConsole uses), reconciling a single instance whose
// only stubbed dependency is the nmap service — the mock stands in for the live
// S6 scanner and returns the scan the current target would yield.

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	nmapsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("NMAP_BACKEND=fsmv1 fresh observation across a connection edit", func() {
	const (
		name        = "bridge-conn"
		port uint16 = 502

		goodTarget = "192.0.2.10" // stands in for the healthy target
		badTarget  = "192.0.2.99" // the known-bad target the edit points at
	)

	var (
		ctx         context.Context
		now         time.Time
		mockService *nmapsvc.MockNmapService
		mgr         *nmapfsm.NmapManager
		services    *serviceregistry.Registry
	)

	// nmapConfig builds an enabled (open == running) config.NmapConfig for the
	// given target. The instance name is stable across targets, matching the
	// connection-only edit the fsmv2 test exercises.
	nmapConfig := func(target string) config.NmapConfig {
		return config.NmapConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            name,
				DesiredFSMState: nmapfsm.OperationalStateOpen,
			},
			NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
				Target: target,
				Port:   port,
			},
		}
	}

	// snapshotWith puts the config at the path the manager reads, exactly as the
	// agent hands the edited config.yaml to the FSM layer.
	snapshotWith := func(cfg config.NmapConfig) publicfsm.SystemSnapshot {
		return publicfsm.SystemSnapshot{
			SnapshotTime: now,
			CurrentConfig: config.FullConfig{
				Internal: config.InternalConfig{Nmap: []config.NmapConfig{cfg}},
			},
		}
	}

	// setScan makes the mock nmap service report the given port scan, the way
	// the live S6 scanner would after scanning the current target.
	setScan := func(portState string, running bool) {
		mockService.ExistingServices[name] = true
		mockService.ServiceStates[name] = &nmapsvc.ServiceInfo{
			NmapStatus: nmapsvc.NmapServiceInfo{
				IsRunning: running,
				LastScan: &nmapsvc.NmapScanResult{
					Timestamp:  now,
					PortResult: nmapsvc.PortResult{State: portState, Port: port},
				},
			},
		}
	}

	// setRestarting makes the mock report a scanner that has been reconfigured
	// and restarted for a new target but has not produced a completed scan yet:
	// no LastScan, not running. isNmapHealthy rejects this, so the instance
	// cannot remain open.
	setRestarting := func() {
		mockService.ExistingServices[name] = true
		mockService.ServiceStates[name] = &nmapsvc.ServiceInfo{
			NmapStatus: nmapsvc.NmapServiceInfo{
				IsRunning: false,
				LastScan:  nil,
			},
		}
	}

	// currentState reads the fsmv1 operational state the connection FSM would see.
	currentState := func() string {
		inst, found := mgr.GetInstance(name)
		Expect(found).To(BeTrue(), "instance %q must exist in the manager", name)

		return inst.GetCurrentFSMState()
	}

	// reconcile runs one manager tick against the given config with a deadline'd
	// context (the base manager requires one).
	reconcile := func(cfg config.NmapConfig) {
		rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		_, _ = mgr.Reconcile(rctx, snapshotWith(cfg), services)
	}

	// driveUntil reconciles the given config until the instance reaches target
	// or the attempt budget is exhausted.
	driveUntil := func(cfg config.NmapConfig, target string) {
		for range 40 {
			if currentState() == target {
				return
			}

			reconcile(cfg)
		}

		Expect(currentState()).To(Equal(target),
			"instance did not reach %q within the attempt budget", target)
	}

	BeforeEach(func() {
		ctx = context.Background()
		now = time.Now()

		mockService = nmapsvc.NewMockNmapService()
		mockService.GetConfigResult = nmapConfig(goodTarget).NmapServiceConfig
		services = serviceregistry.NewMockRegistry()

		// Real manager (real compare/setConfig) driving a single instance whose
		// nmap service is the mock. The manager applies edits from the snapshot
		// via its real setConfig path.
		mgr = nmapfsm.NewNmapManager("staleness-test")

		inst := nmapfsm.NewNmapInstance(nmapConfig(goodTarget))
		inst.SetService(mockService)
		mgr.AddInstanceForTest(name, inst)
	})

	It("does not report the connection open after the target is edited to a bad host (fsmv1 is correct)", func() {
		// 1. Deploy the healthy connection: the scanner reports the good target
		//    open, and the instance settles in open.
		setScan(string(nmapfsm.PortStateOpen), true)
		driveUntil(nmapConfig(goodTarget), nmapfsm.OperationalStateOpen)
		Expect(currentState()).To(Equal(nmapfsm.OperationalStateOpen),
			"the healthy connection must read open before the edit")

		// 2. Edit the connection to a known-bad target. Only the target changes;
		//    the instance name is unchanged. Editing the target reconfigures and
		//    restarts the S6 nmap service, so the old open scan of the GOOD
		//    target no longer applies and no completed scan of the bad target
		//    exists yet.
		setRestarting()

		// 3. A target change must not keep the connection open: fsmv1 derives
		//    the state from the current scanner observation, so with the scanner
		//    restarted it drops out of open. This PASSES (GREEN) — fsmv1 never
		//    serves the old target's stale open, unlike the fsmv2 adapter.
		driveUntil(nmapConfig(badTarget), nmapfsm.OperationalStateDegraded)
		Expect(currentState()).NotTo(Equal(nmapfsm.OperationalStateOpen),
			"a connection edited to a bad target must not read open from the old target's scan")

		// 4. Once the restarted scanner actually scans the bad target (closed),
		//    the instance reads closed — confirming the earlier drop was the
		//    real state, not a stale-open window.
		setScan(string(nmapfsm.PortStateClosed), true)
		driveUntil(nmapConfig(badTarget), nmapfsm.OperationalStateClosed)
		Expect(currentState()).To(Equal(nmapfsm.OperationalStateClosed),
			"after the bad target is scanned the connection reads closed")
	})
})
