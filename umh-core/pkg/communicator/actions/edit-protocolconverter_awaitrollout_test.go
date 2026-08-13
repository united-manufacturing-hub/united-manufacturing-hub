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
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	connfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	connsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	nmapsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
)

var _ = Describe("EditProtocolConverter awaitRollout (config-as-truth gate)", func() {
	const (
		probeName    = "awaitrollout-bridge"
		port         = uint16(445)
		tickInterval = 1 * time.Second
	)

	var (
		probeUUID uuid.UUID
		mockCfg   *config.MockConfigManager
		snapMgr   *fsm.SnapshotManager
		outbound  chan *models.UMHMessage
		msgs      []*models.UMHMessage
		mu        sync.Mutex
	)

	// stageSnapshot writes a protocol-converter snapshot with an explicit
	// desired connection config (the rendered config the bridge should be
	// running, i.e. the "system of truth"), an explicit observed nmap config
	// (what the scan has actually dialed), the port state the last scan
	// reported (open/closed/filtered), and the PC FSM state. Giving the desired
	// and observed sides independently is what lets a test stage the anomaly: a
	// connection edited to a new target whose nmap scan has not yet caught up.
	// Staging PortResult.State is what distinguishes "not yet scanned" from
	// "scanned and found closed" — the two reasons awaitRollout must treat
	// differently.
	stageSnapshot := func(desiredTarget string, observedTarget string, pcState string, portState string) {
		observed := &protocolconverter.ProtocolConverterObservedStateSnapshot{
			ServiceInfo: protocolconvertersvc.ServiceInfo{
				ConnectionObservedState: connfsm.ConnectionObservedState{
					ObservedConnectionConfig: connectionserviceconfig.ConnectionServiceConfig{
						NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
							Target: desiredTarget,
							Port:   port,
						},
					},
					ServiceInfo: connsvc.ServiceInfo{
						NmapObservedState: nmapfsm.NmapObservedState{
							ObservedNmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
								Target: observedTarget,
								Port:   port,
							},
							ServiceInfo: nmapsvc.ServiceInfo{
								NmapStatus: nmapsvc.NmapServiceInfo{
									LastScan: &nmapsvc.NmapScanResult{
										PortResult: nmapsvc.PortResult{
											State: portState,
											Port:  port,
										},
									},
								},
							},
						},
					},
				},
			},
		}
		snapMgr.UpdateSnapshot(&fsm.SystemSnapshot{
			Managers: map[string]fsm.ManagerSnapshot{
				constants.ProtocolConverterManagerName: &actions.MockManagerSnapshot{
					Instances: map[string]*fsm.FSMInstanceSnapshot{
						probeName: {
							ID:                probeName,
							CurrentState:      pcState,
							DesiredState:      protocolconverter.OperationalStateActive,
							LastObservedState: observed,
						},
					},
				},
			},
		})
	}

	// runAwaitRollout drives a real awaitRollout Execute in a goroutine and
	// returns the resulting error and the wall-clock time it took.
	runAwaitRollout := func(ip string) (time.Duration, error) {
		a := actions.NewEditProtocolConverterAction(
			"probe@example.com", uuid.New(), uuid.New(), outbound, mockCfg, snapMgr)
		a.SetTickInterval(tickInterval)
		a.SetAwaitTimeout(6 * time.Second)

		payload := map[string]interface{}{
			"name": probeName,
			"uuid": probeUUID.String(),
			"connection": map[string]interface{}{
				"ip":   ip,
				"port": port,
			},
		}
		Expect(a.Parse(payload)).To(Succeed())
		Expect(a.Validate()).To(Succeed())
		Expect(a.GetDFCType()).To(Equal("empty"))

		type res struct {
			err     error
			elapsed time.Duration
		}
		ch := make(chan res, 1)
		start := time.Now()
		go func() {
			_, _, err := a.Execute()
			ch <- res{err, time.Since(start)}
		}()
		var got res
		Eventually(ch, "15s").Should(Receive(&got))

		return got.elapsed, got.err
	}

	BeforeEach(func() {
		probeUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(probeName)
		outbound = make(chan *models.UMHMessage, 200)
		mu.Lock()
		msgs = nil
		mu.Unlock()

		mockCfg = config.NewMockConfigManager().WithConfig(config.FullConfig{
			Agent: config.AgentConfig{MetricsPort: 8080},
			ProtocolConverter: []config.ProtocolConverterConfig{{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            probeName,
					DesiredFSMState: "active",
				},
				ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
					Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
						ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
							NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
								Target: "{{ .IP }}",
								Port:   "{{ .PORT }}",
							},
						},
					},
					Variables: variables.VariableBundle{
						User: map[string]interface{}{"IP": "src.example.com", "PORT": "443"},
					},
				},
			}},
		})

		snapMgr = fsm.NewSnapshotManager()
		go actions.ConsumeOutboundMessages(outbound, &msgs, &mu, true)
	})

	AfterEach(func() { close(outbound) })

	It("reports a scanned-and-open port as a successful rollout, promptly", func() {
		// Regression guard: when the observed scan has caught up to the
		// requested port and reports it open, awaitRollout must succeed on the
		// first tick rather than wait out the timeout. This guards the opposite
		// error: a check that never matches would fail every healthy edit after
		// 30s, which is worse than the bug it replaces.
		stageSnapshot("dest.example.com", "dest.example.com", protocolconverter.OperationalStateStartingFailedDFCMissing, string(nmapfsm.PortStateOpen))

		elapsed, err := runAwaitRollout("dest.example.com")
		Expect(err).NotTo(HaveOccurred(),
			"a scanned-and-open port must still be reported as a successful rollout, promptly")
		Expect(elapsed).To(BeNumerically("<", 5*time.Second),
			"a healthy edit must not wait out the rollout timeout")
	})

	It("reports failure when the port was scanned but found closed", func() {
		// The bug: the gate's only condition was port-number equality. Poll
		// stamps the requested port onto a closed result too, so one poll after
		// the edit the numbers match regardless of whether anything answered. A
		// confirmed-closed port must not report a successful rollout.
		stageSnapshot("dest.example.com", "dest.example.com", protocolconverter.OperationalStateStartingFailedDFCMissing, string(nmapfsm.PortStateClosed))

		_, err := runAwaitRollout("dest.example.com")
		Expect(err).To(HaveOccurred(),
			"a scanned-but-closed port must not be reported as a successful rollout")
	})

	It("reports failure when the port was scanned but found filtered", func() {
		// The connection FSM defines up as open and counts filtered (and all
		// five non-open states) as down. Requiring open here makes the gate
		// agree with that definition rather than inventing a second one.
		stageSnapshot("dest.example.com", "dest.example.com", protocolconverter.OperationalStateStartingFailedDFCMissing, string(nmapfsm.PortStateFiltered))

		_, err := runAwaitRollout("dest.example.com")
		Expect(err).To(HaveOccurred(),
			"a filtered port is down by the connection FSM's own definition and must not report success")
	})
})
