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

package integration_test

// Full-instance integration test for the NMAP_BACKEND=fsmv2 stale-observation
// bug across a bridge (protocolConverter) connection edit.
//
// A connection-only bridge (a protocolConverter with a connection template but
// NO read/write DFC) reaches the ACCEPTED state starting_failed_dfc_missing when
// its connection is up. The edit-protocol-converter action, for the DFCType
// "empty" path, treats {active, idle, starting_failed_dfc_missing} as "rolled
// out successfully".
//
// Scenario driven here end-to-end against a REAL container:
//  1. Boot umh-core with a bridge whose connection nmap-probes a GOOD target
//     (127.0.0.1:8080, the always-open in-container metrics server).
//  2. Wait for the bridge to settle into an accepted state.
//  3. Send an edit-protocol-converter ACTION (via the REAL ManagementConsole
//     backend+router) that re-points the connection at a KNOWN-BAD target
//     (127.0.0.1:65000, a closed port).
//  4. Observe the terminal action-reply the agent pushes back.
//
// CORRECT behavior (fsmv1): editing the target restarts the S6 nmap service, the
// probe of the bad target reports the port closed, the connection goes down, the
// bridge leaves the accepted state, awaitRollout times out and rolls back →
// terminal reply = action-failure.
//
// BUGGY behavior (fsmv2): the adapter serves the stale "open" scan of the OLD
// target (keyed by an unchanged child ref, still Fresh by age), so the bridge
// briefly reads healthy and stays in the accepted state → awaitRollout reports
// action-success. This is WRONG.
//
// Both specs ASSERT the terminal reply is action-failure:
//   - the fsmv1 spec PASSES (green) — correct behavior.
//   - the fsmv2 spec is RED on purpose: it reproduces the bug (the agent pushes
//     action-success). It must flip to green once the staleness bug is fixed.
//     This mirrors the deliberately-red unit test in
//     pkg/fsmv2/nmap/nmap_staleness_integration_test.go.
//
// These specs are NOT under Label("integration") (that suite runs --fail-fast,
// which the intentionally-red fsmv2 spec would abort). Run them with:
//
//	make connection-staleness-test
//
// (which first runs `make pull-managementconsole` to fetch the backend+router
// source from the private repo using your `gh` credentials, then runs:
//
//	FORCE_DOCKER=true VERSION=dev MC_DIR=... go run github.com/onsi/ginkgo/v2/ginkgo \
//	  -r --tags=test --skip-package=managementconsole \
//	  --label-filter='connection-staleness' --timeout=30m ./integration/...)
//
// --skip-package=managementconsole keeps ginkgo's -r discovery out of the pulled
// ManagementConsole source tree (integration/managementconsole/), whose Go
// packages otherwise fail to compile under the test runner (e.g. the
// demo_simulator_v3 //go:embed requirements.json, present only at binary-build
// time).

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

const (
	stalenessBridgeName = "bridge-conn"

	// GOOD target: the in-container metrics server always listens on 8080.
	stalenessGoodIP   = "127.0.0.1"
	stalenessGoodPort = uint32(8080)

	// BAD target: a closed port on loopback.
	stalenessBadIP   = "127.0.0.1"
	stalenessBadPort = uint32(65000)

	// stalenessTagProcessorJS is the read DFC's tag_processor body. Static (no
	// template variables) so the deployed template and the edit action's readDFC
	// render to the same benthos config, keeping compareProtocolConverterDFCConfig
	// a match across the edit. Uses the canonical _historian data contract.
	stalenessTagProcessorJS = `msg.meta.location_path = "test-enterprise";
msg.meta.data_contract = "_historian";
msg.meta.tag_name = "my_data";
msg.payload = msg.payload;
return msg;`
)

// buildStalenessConfig builds a config.yaml with:
//   - a communicator pointed at the real router (plain HTTP, insecure TLS),
//   - one connection-only bridge targeting the GOOD target, desired "active",
//   - redpanda active (a bridge only reaches starting_failed_dfc_missing once
//     redpanda is healthy — see pkg/fsm/protocolconverter/reconcile.go).
func buildStalenessConfig(apiURL string) string {
	redpandaCfg := config.RedpandaConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            "redpanda",
			DesiredFSMState: "active",
		},
		RedpandaServiceConfig: redpandaserviceconfig.RedpandaServiceConfig{},
	}
	redpandaCfg.RedpandaServiceConfig.Resources.MaxCores = 1
	redpandaCfg.RedpandaServiceConfig.Resources.MemoryPerCoreInBytes = 1024 * 1024 * 1024 * 2 // 2GB

	// The bridge body for the root protocol converter. On disk a ROOT protocol
	// converter (TemplateRef == Name) does NOT carry its body inline: the config
	// parser (convertYamlToSpec, pkg/config/yamlParsing.go) resolves TemplateRef
	// against the top-level templates.protocolConverter map and errors if the
	// entry is missing — which silently empties the whole config. So the body
	// lives here and the PC entry only references it.
	//
	// The bridge carries a READ dataflowcomponent so its accepted FSM state is
	// gated on connection health. A connection-only bridge sits at the accepted
	// state starting_failed_dfc_missing regardless of whether nmap sees the
	// target up or down, so an edit to a dead target still "succeeds" — the bug
	// cannot surface. With a read DFC the bridge only reaches active/idle while
	// the connection is up (pkg/fsm/protocolconverter reconcile checks
	// IsConnectionUp before the DFC), so re-pointing to a dead target drops it to
	// degraded_connection/starting_connection, and awaitRollout (the DFCTypeRead
	// path in edit-protocolconverter.go accepts only {active,idle}) fails. The
	// output is intentionally omitted — the read DFC's egress is forced to the
	// UNS publisher at render time (config_customized.go).
	bridgeTemplate := protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
		ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
			NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
				Target: "{{ .IP }}",
				Port:   "{{ .PORT }}",
			},
		},
		DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
				Input: map[string]any{
					"generate": map[string]any{
						"count":    0,
						"interval": "1s",
						"mapping":  `root = "hello world"`,
					},
				},
				Pipeline: map[string]any{
					"processors": []any{
						map[string]any{"tag_processor": map[string]any{"defaults": stalenessTagProcessorJS}},
					},
				},
				Buffer: map[string]any{"none": map[string]any{}},
			},
		},
	}

	full := config.FullConfig{
		Templates: config.TemplatesConfig{
			ProtocolConverter: map[string]any{
				stalenessBridgeName: bridgeTemplate,
			},
		},
		Agent: config.AgentConfig{
			MetricsPort: 8080,
			Location:    map[int]string{0: "test-enterprise"},
			CommunicatorConfig: config.CommunicatorConfig{
				APIURL:           apiURL,
				AuthToken:        "test-token",
				AllowInsecureTLS: true,
			},
		},
		ProtocolConverter: []config.ProtocolConverterConfig{
			{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            stalenessBridgeName,
					DesiredFSMState: "active",
				},
				ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
					// TemplateRef == Name marks this bridge as a template ROOT (the
					// body is in templates.protocolConverter above). A root is also
					// required for the edit: AtomicEditProtocolConverter rejects
					// editing a non-root child (pkg/config/protocolconverter.go), so
					// the deployed bridge must be a root for the edit to reach
					// awaitRollout.
					TemplateRef: stalenessBridgeName,
					Variables: variables.VariableBundle{
						User: map[string]any{
							"IP":   stalenessGoodIP,
							"PORT": strconv.FormatUint(uint64(stalenessGoodPort), 10),
						},
					},
					Location: map[string]string{
						"0": "test-enterprise",
					},
				},
			},
		},
		Internal: config.InternalConfig{
			Redpanda: redpandaCfg,
			TopicBrowser: config.TopicBrowserConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            "topic-browser",
					DesiredFSMState: "stopped",
				},
			},
		},
	}

	out, err := yaml.Marshal(full)
	Expect(err).NotTo(HaveOccurred())

	return string(out)
}

// stalenessReadDFCActionPayload builds the "readDFC" object for the edit action,
// matching the read DFC deployed in buildStalenessConfig's template. The input
// and processor bodies are raw benthos YAML strings (marshaled so indentation of
// the tag_processor block scalar is correct), which is the wire form the edit
// action parses (dataflow-component-parser.go). No output is set — the read DFC's
// egress is forced to the UNS publisher at render time. Keeping the body
// identical to the deployed template makes compareProtocolConverterDFCConfig a
// match across the edit, so success/failure hinges purely on the PC's FSM state
// (and thus connection health), which is what distinguishes the nmap backends.
func stalenessReadDFCActionPayload() map[string]any {
	genData, err := yaml.Marshal(map[string]any{
		"generate": map[string]any{
			"count":    0,
			"interval": "1s",
			"mapping":  `root = "hello world"`,
		},
	})
	Expect(err).NotTo(HaveOccurred())

	procData, err := yaml.Marshal(map[string]any{
		"tag_processor": map[string]any{"defaults": stalenessTagProcessorJS},
	})
	Expect(err).NotTo(HaveOccurred())

	return map[string]any{
		"state": "active",
		"inputs": map[string]any{
			"type": "generate",
			"data": string(genData),
		},
		"pipeline": map[string]any{
			"processors": map[string]any{
				"0": map[string]any{
					"type": "tag_processor",
					"data": string(procData),
				},
			},
		},
	}
}

// bridgeReachedAcceptedState reports whether the bridge has reached one of the
// accepted states for an "active" desired state. FSM state transitions are
// written to the container's INTERNAL log (/data/logs/umh-core/current), not to
// the docker stdout log, so this reads that file via `docker exec cat`. The
// system snapshot logger prints "<name>: <currentState> → <desiredState>".
func bridgeReachedAcceptedState() bool {
	out, err := runDockerCommand("exec", getContainerName(), "cat", "/data/logs/umh-core/current")
	if err != nil {
		return false
	}

	for _, accepted := range []string{
		stalenessBridgeName + ": starting_failed_dfc_missing",
		stalenessBridgeName + ": idle",
		stalenessBridgeName + ": active",
	} {
		if strings.Contains(out, accepted) {
			GinkgoWriter.Printf("NMAP backend reached accepted state: %s", accepted)
			return true
		}
	}

	return false
}

// runConnectionEditStalenessSpec registers a full Ordered spec (BeforeAll boots
// the container + real ManagementConsole stack, It drives the edit, AfterAll tears down) for the
// given NMAP_BACKEND. It is invoked once per backend so each backend gets its
// own independent container so a red fsmv2 result does not skip fsmv1.
func runConnectionEditStalenessSpec(nmapBackend string) {
	var backend *mcStack

	BeforeAll(func() {
		// The container encodes messages with corev1 (cmd/main.go); the test
		// process must match so the container can decode our action and we can
		// decode its replies. The backend relays Content opaquely.
		encodingChooseCorev1()

		var err error

		backend, err = newMCStack(context.Background())
		Expect(err).NotTo(HaveOccurred())

		GinkgoWriter.Printf("ManagementConsole stack up, router at %s (NMAP_BACKEND=%s)\n", backend.apiURL(), nmapBackend)

		// Extra `docker create` args:
		//   - select the nmap backend,
		//   - force the FSMv2 transport on (it hosts the fsmv2client the fsmv2 nmap
		//     manager reads from),
		//   - enable the communicator via API_URL/AUTH_TOKEN env. This is how
		//     production umh-core (and ManagementConsole's docker.ts) wires the
		//     backend connection: pkg/config/env.go reads API_URL/AUTH_TOKEN and
		//     they gate the communicator at cmd/main.go:283. AUTH_TOKEN is the RAW
		//     token; umh-core hashes it to LoginHash before sending, and mcStack
		//     seeds that same hash.
		//   - ALLOW_INSECURE_TLS for the plain-HTTP router,
		//   - and let the container dial the host-bound router.
		extraCreateArgs = []string{
			"-e", "NMAP_BACKEND=" + nmapBackend,
			"-e", "USE_FSMV2_TRANSPORT=true",
			"-e", "API_URL=" + backend.apiURL(),
			"-e", "AUTH_TOKEN=" + mcAuthToken,
			"-e", "ALLOW_INSECURE_TLS=true",
			"--add-host=host.docker.internal:host-gateway",
		}

		cfg := buildStalenessConfig(backend.apiURL())

		err = BuildAndRunContainer(cfg, DEFAULT_MEMORY, DEFAULT_CPUS)
		if err != nil {
			printContainerDebugInfo()
			Expect(err).NotTo(HaveOccurred(), "Container startup failed")
		}

		Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available")
	})

	AfterAll(func() {
		PrintLogsAndStopContainer()

		if backend != nil {
			backend.stop()
		}

		extraCreateArgs = nil
		skipConfigCopy = false

		if !CurrentSpecReport().Failed() {
			cleanupTmpDirs(getContainerName())
		}
	})

	It("rolls back the connection edit to a bad target (terminal reply = action-failure)", func() {
		By("waiting for the container to log in to the ManagementConsole backend")
		Eventually(func() bool {
			return backend.loginSeen()
		}, 120*time.Second, 1*time.Second).Should(BeTrue(),
			"container must complete login against the ManagementConsole backend")

		time.Sleep(10 * time.Second)

		By("waiting for the bridge to settle into an accepted state with the GOOD connection")
		Eventually(bridgeReachedAcceptedState, 300*time.Second, 2*time.Second).Should(BeTrue(),
			"bridge must reach starting_failed_dfc_missing/idle/active with the good target before editing")

		By("enqueuing an edit-protocol-converter action re-pointing the connection at the BAD target")

		time.Sleep(10 * time.Second)

		actionUUID := uuid.New()
		pcUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(stalenessBridgeName)

		// The edit MUST carry the read DFC too. edit-protocolconverter.go derives
		// DFCType from the payload: a connection-only edit → DFCTypeEmpty (the
		// non-connection-gated rollout that accepts starting_failed_dfc_missing).
		// Re-sending the read DFC keeps it DFCTypeRead, whose rollout accepts only
		// {active,idle} — reachable only while the connection is up.
		Expect(backend.enqueueEditProtocolConverter(
			actionUUID, pcUUID, stalenessBridgeName, stalenessBadIP, stalenessBadPort,
			stalenessReadDFCActionPayload(),
		)).To(Succeed())

		By("waiting for a terminal action-reply for the edit action")
		Eventually(func() bool {
			_, terminal := backend.terminalReplyState(actionUUID)

			return terminal
		}, 150*time.Second, 1*time.Second).Should(BeTrue(),
			"the edit action must produce a terminal action-reply (success or failure)")

		finalState, _ := backend.terminalReplyState(actionUUID)
		dump := backend.replyDump(actionUUID)

		// Surface the full reply history unconditionally (visible with `-v`) so a
		// pass/fail can be attributed to the real rollout outcome rather than an
		// early/spurious failure (e.g. "not found", parse/validation error).
		AddReportEntry(fmt.Sprintf("NMAP_BACKEND=%s edit action-reply history", nmapBackend), strings.Join(dump, "\n"))
		GinkgoWriter.Printf("NMAP_BACKEND=%s terminal action-reply for %s: %q\nfull history:\n  %s\n",
			nmapBackend, actionUUID, finalState, strings.Join(dump, "\n  "))

		// Guard against a false result: the edit must actually reach awaitRollout
		// (the stage that polls the connection/nmap health), not fail earlier at
		// parse/validate/persist. awaitRollout emits a distinctive
		// "Waiting for bridge <name> to be active..." reply; without it, a
		// terminal action-failure is an early/spurious failure that would make the
		// fsmv2 spec pass for the WRONG reason and mask the staleness bug.
		Expect(dump).To(ContainElement(ContainSubstring("Waiting for bridge")),
			"edit must reach awaitRollout (the connection-health poll); a terminal reply without a "+
				"'Waiting for bridge' reply is an early/spurious failure, not a real rollout result")

		// CORRECT behavior: a connection edit to a closed port must NOT report
		// success. fsmv1 satisfies this (green). fsmv2 currently reports
		// action-success (RED) because the adapter serves the stale "open" scan
		// of the OLD target across the edit — this spec reproduces the bug and
		// must flip to green once the staleness bug is fixed.
		Expect(finalState).To(Equal(models.ActionFinishedWithFailure),
			"connection edit to a bad target must fail/rollback; a non-failure terminal reply "+
				"(action-success) is the NMAP_BACKEND=fsmv2 staleness bug")
	})
}

var _ = Describe("Connection edit staleness - NMAP_BACKEND=fsmv1 (correct, expected GREEN)",
	Ordered, Label("connection-staleness"), func() {
		runConnectionEditStalenessSpec("fsmv1")
	})

var _ = Describe("Connection edit staleness - NMAP_BACKEND=fsmv2 (BUG, expected RED until fixed)",
	Ordered, Label("connection-staleness"), func() {
		runConnectionEditStalenessSpec("fsmv2")
	})
