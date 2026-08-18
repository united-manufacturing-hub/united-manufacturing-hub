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
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	connfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	connsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	dfcsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
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

	// observedConnection builds the connection half of a protocol-converter
	// snapshot: an explicit desired connection config (the rendered config the
	// bridge should be running, i.e. the "system of truth"), an explicit observed
	// nmap config (what the scan has actually dialed), the port state the last
	// scan reported (open/closed/filtered), and the port the scan ran against.
	// Giving the desired and observed sides independently is what lets a test
	// stage the anomaly: a connection edited to a new target whose nmap scan has
	// not yet caught up. Staging PortResult.State is what distinguishes "not yet
	// scanned" from "scanned and found closed" — the two reasons awaitRollout
	// must treat differently. Staging the scanned port separately from the
	// payload port is what lets a test stage a templated connection, whose
	// resolved port is not the one the payload carries.
	observedConnection := func(desiredTarget string, observedTarget string, portState string, scannedPort uint16) connfsm.ConnectionObservedState {
		return connfsm.ConnectionObservedState{
			ObservedConnectionConfig: connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: desiredTarget,
					Port:   scannedPort,
				},
			},
			ServiceInfo: connsvc.ServiceInfo{
				NmapObservedState: nmapfsm.NmapObservedState{
					ObservedNmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
						Target: observedTarget,
						Port:   scannedPort,
					},
					ServiceInfo: nmapsvc.ServiceInfo{
						NmapStatus: nmapsvc.NmapServiceInfo{
							LastScan: &nmapsvc.NmapScanResult{
								PortResult: nmapsvc.PortResult{
									State: portState,
									Port:  scannedPort,
								},
							},
						},
					},
				},
			},
		}
	}

	// publish installs observed as the bridge's LastObservedState with the given
	// PC FSM state.
	publish := func(pcState string, observed *protocolconverter.ProtocolConverterObservedStateSnapshot) {
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

	// stageSnapshotOnPort stages a bridge with no dataflow component: only the
	// connection half is observed, which is all the DFCTypeEmpty branch reads.
	stageSnapshotOnPort := func(desiredTarget string, observedTarget string, pcState string, portState string, scannedPort uint16) {
		publish(pcState, &protocolconverter.ProtocolConverterObservedStateSnapshot{
			ServiceInfo: protocolconvertersvc.ServiceInfo{
				ConnectionObservedState: observedConnection(desiredTarget, observedTarget, portState, scannedPort),
			},
		})
	}

	// stageSnapshot stages a scan of the port the payload asks for, the case
	// every non-templated connection is in.
	stageSnapshot := func(desiredTarget string, observedTarget string, pcState string, portState string) {
		stageSnapshotOnPort(desiredTarget, observedTarget, pcState, portState, port)
	}

	// runAwaitRolloutOnPort drives a real awaitRollout Execute in a goroutine
	// with the given payload connection, and returns the resulting error and the
	// wall-clock time it took.
	runAwaitRolloutOnPort := func(ip string, payloadPort uint32) (time.Duration, error) {
		a := actions.NewEditProtocolConverterAction(
			"probe@example.com", uuid.New(), uuid.New(), outbound, mockCfg, snapMgr)
		a.SetTickInterval(tickInterval)
		a.SetAwaitTimeout(6 * time.Second)

		payload := map[string]interface{}{
			"name": probeName,
			"uuid": probeUUID.String(),
			"connection": map[string]interface{}{
				"ip":   ip,
				"port": payloadPort,
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

	// runAwaitRollout edits a connection to ip on the payload port, the case
	// every non-templated connection is in.
	runAwaitRollout := func(ip string) (time.Duration, error) {
		return runAwaitRolloutOnPort(ip, uint32(port))
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

	It("reports failure when the only open port on record belongs to the previous host", func() {
		// ENG-5586. The gate compared the port number and nothing else. So
		// repointing a bridge from one host to another on the same port was
		// accepted by the OLD host's scan: the number matches, that port is
		// open, and nothing has dialled the new host. If the new host refuses
		// the connection the edit still reported success.
		stageSnapshot("dest.example.com", "src.example.com", protocolconverter.OperationalStateStartingFailedDFCMissing, string(nmapfsm.PortStateOpen))

		_, err := runAwaitRollout("dest.example.com")
		Expect(err).To(HaveOccurred(),
			"an open port on the previous host must not be reported as a successful rollout of the new one")
		// A bare HaveOccurred() would also be satisfied by a timeout for any other
		// reason, so it cannot tell this spec passing from this spec passing by
		// accident. The rollback message names what the last tick was waiting for.
		Expect(err.Error()).To(ContainSubstring("waiting for nmap to scan dest.example.com"),
			"the rollout must have timed out on the host comparison, not on something else")
	})

	Describe("a bridge whose connection is templated", func() {
		// A bridge that writes to the historian scans the shared
		// historian.timescale endpoint, so its connection template carries no
		// {{ .IP }}/{{ .PORT }} and its spec keeps no IP/PORT user variables.
		// get-protocolconverter therefore hands the Management Console the raw
		// template string as the connection IP and a port of 0 (a template
		// string does not parse as a number), and that is what returns in the
		// edit payload. The gate must compare the scan against the rendered
		// endpoint; comparing it against the payload can never match.
		const (
			timescaleHost = "timescale.example.com"
			// Deliberately not the 5432 default, so a port that only matches by
			// coincidence cannot pass this spec.
			timescalePort = uint16(5433)
		)

		BeforeEach(func() {
			mockCfg = config.NewMockConfigManager().WithConfig(config.FullConfig{
				Agent: config.AgentConfig{MetricsPort: 8080},
				Historian: &config.HistorianConfig{
					Timescale: config.TimescaleConfig{
						Host:     timescaleHost,
						Port:     timescalePort,
						Password: "probe-password",
					},
				},
				ProtocolConverter: []config.ProtocolConverterConfig{{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            probeName,
						DesiredFSMState: "active",
					},
					ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
						Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
							ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
								NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
									Target: "{{ .historian.timescale.host }}",
									Port:   "{{ .historian.timescale.port }}",
								},
							},
							DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
								Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
									Protocol: dataflowcomponentserviceconfig.HistorianDestinationProtocol,
									Code:     "data_contract_name: pump",
								},
								Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.*"},
							},
						},
						Variables: variables.VariableBundle{User: map[string]interface{}{}},
					},
				}},
			})
		})

		It("reports a scanned-and-open resolved endpoint as a successful rollout, promptly", func() {
			stageSnapshotOnPort(timescaleHost, timescaleHost, protocolconverter.OperationalStateStartingFailedDFCMissing, string(nmapfsm.PortStateOpen), timescalePort)

			elapsed, err := runAwaitRolloutOnPort("{{ .historian.timescale.host }}", 0)
			Expect(err).NotTo(HaveOccurred(),
				"the gate must compare the scan against the rendered endpoint, not the unresolved payload")
			Expect(elapsed).To(BeNumerically("<", 5*time.Second),
				"a healthy edit of a templated connection must not wait out the rollout timeout")
		})
	})

	// --- the read-DFC path -----------------------------------------------------
	//
	// Every spec above drives an edit with no dataflow component, which
	// awaitRollout classifies as DFCTypeEmpty and gates on the nmap scan. An edit
	// that carries a read DFC takes an entirely different branch: the nmap check
	// lives inside `if a.dfcType == DFCTypeEmpty`, so on the read path it never
	// runs, and acceptance is decided by the protocolconverter's CurrentState
	// alone. These two specs pin that difference in-process.
	//
	// caughtUpReadDFCBenthos is the Benthos config a converged bridge observes for
	// the read DFC in runAwaitRolloutRead's payload: the payload's own generate
	// input and tag_processor, plus the downsampler that renderConfig appends to
	// every timeseries pipeline. The values were read off renderDesiredDFCConfig
	// for that exact payload, because compareSingleDFCConfig accepts only a
	// comparator-equal match and the appended downsampler is invisible in the
	// payload. Output is deliberately absent: the read comparison nils both sides'
	// Output before comparing, since the read DFC's egress is forced to the UNS
	// publisher at render time.
	caughtUpReadDFCBenthos := benthosserviceconfig.BenthosServiceConfig{
		Input: map[string]interface{}{
			"generate": map[string]interface{}{
				"count":    0,
				"interval": "1s",
				"mapping":  `root = "hello world"`,
			},
		},
		Pipeline: map[string]interface{}{
			"processors": []interface{}{
				map[string]interface{}{
					"tag_processor": map[string]interface{}{
						"defaults": "msg.meta.location_path = \"probe\";\nmsg.meta.data_contract = \"_raw\";\nreturn msg;\n",
					},
				},
				map[string]interface{}{"downsampler": map[string]interface{}{}},
			},
		},
		Buffer: map[string]interface{}{"none": map[string]interface{}{}},
	}

	// stageReadDFCSnapshot stages a bridge that carries a read DFC. Beyond the
	// connection half it stages the two things the read branch needs before it can
	// reach its accepted-state check:
	//
	//   - ObservedProtocolConverterSpecConfig with a connection template.
	//     renderDesiredDFCConfig renders the whole spec, and ConvertTemplateToRuntime
	//     rejects an absent connection template outright ("connection template is nil
	//     or empty"), so without it every tick fails to render rather than comparing.
	//   - the observed read-DFC Benthos config. compareSingleDFCConfig treats a nil
	//     observed Input as "Benthos is still starting" and returns false before it
	//     ever renders, so a snapshot without it can never be accepted.
	//
	// The spec's variables hold the PRE-EDIT connection values on purpose: the
	// observed spec lags the persisted edit by one or more control-loop cycles, and
	// renderDesiredDFCConfig is expected to overlay the edit's own IP/PORT on top.
	stageReadDFCSnapshot := func(desiredTarget string, observedTarget string, pcState string, portState string) {
		publish(pcState, &protocolconverter.ProtocolConverterObservedStateSnapshot{
			ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
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
			ServiceInfo: protocolconvertersvc.ServiceInfo{
				ConnectionObservedState:       observedConnection(desiredTarget, observedTarget, portState, port),
				DataflowComponentReadFSMState: protocolconverter.OperationalStateActive,
				DataflowComponentReadObservedState: dfcfsm.DataflowComponentObservedState{
					ServiceInfo: dfcsvc.ServiceInfo{
						BenthosObservedState: benthosfsm.BenthosObservedState{
							ObservedBenthosServiceConfig: caughtUpReadDFCBenthos,
						},
					},
				},
			},
		})
	}

	// runAwaitRolloutRead is runAwaitRollout plus a read DFC in the payload, which
	// is what flips deriveDFCType to "read".
	runAwaitRolloutRead := func(ip string) (time.Duration, error, []string) {
		a := actions.NewEditProtocolConverterAction(
			"probe@example.com", uuid.New(), uuid.New(), outbound, mockCfg, snapMgr)
		a.SetTickInterval(tickInterval)
		a.SetAwaitTimeout(6 * time.Second)

		payload := map[string]interface{}{
			"name": probeName,
			"uuid": probeUUID.String(),
			"readDFC": map[string]interface{}{
				"state": "active",
				"inputs": map[string]interface{}{
					"type": "generate",
					"data": "generate:\n  count: 0\n  interval: 1s\n  mapping: root = \"hello world\"\n",
				},
				// pipeline.processors is required by buildReadDFCServiceConfig; a
				// read DFC without it is rejected before dfcType is ever derived.
				"pipeline": map[string]interface{}{
					"processors": map[string]interface{}{
						"0": map[string]interface{}{
							"type": "tag_processor",
							"data": "tag_processor:\n  defaults: |\n    msg.meta.location_path = \"probe\";\n    msg.meta.data_contract = \"_raw\";\n    return msg;\n",
						},
					},
				},
			},
		}
		// An empty ip stands for "the payload carried no connection block at all",
		// which is what an edit that only changes a flow looks like.
		if ip != "" {
			payload["connection"] = map[string]interface{}{"ip": ip, "port": port}
		}

		Expect(a.Parse(payload)).To(Succeed())
		Expect(a.Validate()).To(Succeed())
		Expect(a.GetDFCType()).To(Equal("read"),
			"this spec is about the read-DFC branch; if the payload no longer yields a read DFC it is testing the wrong path")

		start := time.Now()
		_, _, err := a.Execute()
		elapsed := time.Since(start)

		mu.Lock()
		defer mu.Unlock()

		// The replies are the only record of WHETHER the gate accepted and on
		// which tick, so they are decoded to text rather than counted.
		out := make([]string, 0, len(msgs))

		for _, m := range msgs {
			dec, decErr := encoding.DecodeMessageFromUMHInstanceToUser(m.Content)
			if decErr != nil {
				continue
			}

			out = append(out, fmt.Sprintf("%v", dec.Payload))
		}

		return elapsed, err, out
	}

	It("POSITIVE CONTROL: a read-DFC edit whose scan has caught up is accepted", func() {
		// Establishes that this harness can reach the read path's acceptance at
		// all. Without it the next spec's "was not accepted" could be satisfied by
		// the DFC config comparison blocking the gate for an unrelated reason, and
		// would pass while proving nothing. The two specs differ in exactly one
		// value: the observed nmap target.
		stageReadDFCSnapshot("dest.example.com", "dest.example.com", protocolconverter.OperationalStateActive, string(nmapfsm.PortStateOpen))

		elapsed, err, replies := runAwaitRolloutRead("dest.example.com")

		Expect(err).NotTo(HaveOccurred(), "a read-DFC edit whose scan agrees with the request must be accepted")
		Expect(replies).To(ContainElement(ContainSubstring("read DFC configuration verified")),
			"the read path's acceptance must be reachable in this harness, or the next spec proves nothing")
		Expect(elapsed).To(BeNumerically("<", 4*time.Second),
			"against a 6s budget: a gate that withholds acceptance from a healthy read-DFC edit is worse "+
				"than the bug it replaces, so this must not creep towards the timeout")
	})

	It("does not accept a read-DFC edit while the scan still shows the previous target", func() {
		// The reported bug (ENG-5580), in-process and deterministic. The bridge is
		// edited to dest.example.com; the protocolconverter still carries its
		// pre-edit CurrentState of active, and the scan still shows the OLD target
		// as open. Correct behaviour is to withhold acceptance until the requested
		// target has actually been scanned.
		//
		// Today this FAILS: the read branch reads only CurrentState, so it accepts
		// on the first tick against the pre-edit snapshot. That failure is the
		// point — it is the deterministic form of a race the container harness
		// reproduces only intermittently, and it depends on no log capture.
		stageReadDFCSnapshot("dest.example.com", "src.example.com", protocolconverter.OperationalStateActive, string(nmapfsm.PortStateOpen))

		elapsed, err, replies := runAwaitRolloutRead("dest.example.com")

		Expect(replies).NotTo(ContainElement(ContainSubstring("read DFC configuration verified")),
			"the edit was accepted while the only scan on record still described the PREVIOUS target: "+
				"the read branch gates on the protocolconverter's CurrentState, which still held its pre-edit value")
		Expect(err).To(HaveOccurred(),
			"an edit must not report success before the requested target has been scanned")
		Expect(elapsed).To(BeNumerically(">", time.Second),
			"acceptance on the first tick means the pre-edit snapshot was taken as evidence about the new config")
	})

	It("names the stale scan, not the dataflow component, when neither has caught up", func() {
		// PLACEMENT GUARD. The spec above cannot tell where the connection check
		// sits: staged below the DFC config comparison it would still pass, because
		// that comparison matches in that spec's case and the check then runs
		// anyway. This spec stages the case that separates the two positions — the
		// observed read DFC has not started (nil Input, which the comparison reads
		// as "Benthos is still starting") AND the scan still shows the previous
		// target.
		//
		// With the connection check above the comparison, the operator is told the
		// scan has not caught up. Below it, the comparison's own wait returns first
		// and the scan is never mentioned, so the reply blames the dataflow
		// component for a connection that was never dialed.
		publish(protocolconverter.OperationalStateActive, &protocolconverter.ProtocolConverterObservedStateSnapshot{
			ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
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
			ServiceInfo: protocolconvertersvc.ServiceInfo{
				ConnectionObservedState:       observedConnection("dest.example.com", "src.example.com", string(nmapfsm.PortStateOpen), port),
				DataflowComponentReadFSMState: protocolconverter.OperationalStateActive,
			},
		})

		_, err, replies := runAwaitRolloutRead("dest.example.com")

		Expect(err).To(HaveOccurred(), "neither the scan nor the dataflow component has caught up")
		Expect(replies).To(ContainElement(ContainSubstring("waiting for nmap to scan dest.example.com")),
			"the connection check must run before the dataflow component comparison, or a bridge whose "+
				"target was never dialed is reported as a dataflow component problem")
	})

	It("does not wait for a scan when the edit carries no connection", func() {
		// The exemption that keeps a broken bridge editable. An edit that changes
		// only a flow has no new endpoint to verify, so it must not be held up by
		// the connection: otherwise a bridge whose target is unreachable can never
		// be edited at all, including the edit that stops its flows.
		//
		// The scan here still shows the PREVIOUS target — the same staging the
		// ENG-5580 spec above rejects. The only difference is that this payload
		// carries no connection.
		stageReadDFCSnapshot("dest.example.com", "src.example.com", protocolconverter.OperationalStateActive, string(nmapfsm.PortStateOpen))

		elapsed, err, replies := runAwaitRolloutRead("")

		Expect(err).NotTo(HaveOccurred(), "an edit that does not touch the connection must not be blocked by it")
		Expect(replies).To(ContainElement(ContainSubstring("read DFC configuration verified")))
		Expect(elapsed).To(BeNumerically("<", 4*time.Second),
			"against a 6s budget: this edit has nothing to wait for")
	})
})
