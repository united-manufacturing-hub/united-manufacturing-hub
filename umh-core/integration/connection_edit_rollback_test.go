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
	rollbackBridgeName = "bridge-conn"

	// 8080 is the agent's own metrics port (see buildRollbackConfig), which is
	// what makes this endpoint answer inside the container. Change one and the
	// pre-edit wait below fails naming the bridge, not the port.
	rollbackReachableIP   = "127.0.0.1"
	rollbackReachablePort = uint32(8080)

	// TEST-NET-2 is not routed, so the dial is dropped rather than refused and
	// blocks until the observation timeout. A closed port is refused instantly,
	// which lets a scan of the new target land before the rollout reads the
	// connection, and the spec then passes without exercising the rollout at all.
	// https://datatracker.ietf.org/doc/html/rfc5737#section-3
	rollbackUnreachableIP   = "198.51.100.1"
	rollbackUnreachablePort = uint32(445)

	rollbackTagProcessorJS = `msg.meta.location_path = "test-enterprise";
msg.meta.data_contract = "_historian";
msg.meta.tag_name = "my_data";
msg.payload = msg.payload;
return msg;`
)

func rollbackReadDFC() dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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
					map[string]any{"tag_processor": map[string]any{"defaults": rollbackTagProcessorJS}},
				},
			},
			Buffer: map[string]any{"none": map[string]any{}},
		},
	}
}

func rollbackReadDFCPayload() map[string]any {
	generateData, err := yaml.Marshal(map[string]any{
		"generate": map[string]any{
			"count":    0,
			"interval": "1s",
			"mapping":  `root = "hello world"`,
		},
	})
	Expect(err).NotTo(HaveOccurred())

	processorData, err := yaml.Marshal(map[string]any{
		"tag_processor": map[string]any{"defaults": rollbackTagProcessorJS},
	})
	Expect(err).NotTo(HaveOccurred())

	return map[string]any{
		"state": "active",
		"inputs": map[string]any{
			"type": "generate",
			"data": string(generateData),
		},
		"pipeline": map[string]any{
			"processors": map[string]any{
				"0": map[string]any{
					"type": "tag_processor",
					"data": string(processorData),
				},
			},
		},
	}
}

func buildRollbackConfig(apiURL string) string {
	redpandaConfig := config.RedpandaConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            "redpanda",
			DesiredFSMState: "active",
		},
		RedpandaServiceConfig: redpandaserviceconfig.RedpandaServiceConfig{},
	}
	redpandaConfig.RedpandaServiceConfig.Resources.MaxCores = 1
	redpandaConfig.RedpandaServiceConfig.Resources.MemoryPerCoreInBytes = 2 * 1024 * 1024 * 1024

	bridgeTemplate := protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
		ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
			NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
				Target: "{{ .IP }}",
				Port:   "{{ .PORT }}",
			},
		},
		DataflowComponentReadServiceConfig: rollbackReadDFC(),
	}

	full := config.FullConfig{
		Templates: config.TemplatesConfig{
			ProtocolConverter: map[string]any{
				rollbackBridgeName: bridgeTemplate,
			},
		},
		Agent: config.AgentConfig{
			MetricsPort: 8080,
			Location:    map[int]string{0: "test-enterprise"},
			CommunicatorConfig: config.CommunicatorConfig{
				APIURL:           apiURL,
				AuthToken:        mcAuthToken,
				AllowInsecureTLS: true,
			},
		},
		ProtocolConverter: []config.ProtocolConverterConfig{
			{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            rollbackBridgeName,
					DesiredFSMState: "active",
				},
				ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
					TemplateRef: rollbackBridgeName,
					Variables: variables.VariableBundle{
						User: map[string]any{
							"IP":   rollbackReachableIP,
							"PORT": strconv.FormatUint(uint64(rollbackReachablePort), 10),
						},
					},
					Location: map[string]string{"0": "test-enterprise"},
				},
			},
		},
		Internal: config.InternalConfig{
			Redpanda: redpandaConfig,
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

func lastBridgeState() (string, error) {
	out, err := runDockerCommand("exec", getContainerName(), "cat", "/data/logs/umh-core/current")
	if err != nil {
		return "", fmt.Errorf("failed to read the agent log from the container: %w", err)
	}

	marker := rollbackBridgeName + ": "
	state := ""

	for _, line := range strings.Split(out, "\n") {
		index := strings.LastIndex(line, marker)
		if index < 0 {
			continue
		}

		// Only the snapshot logger's state lines carry the arrow. The marker also
		// appears in s6 dependency and service-status lines, which would otherwise
		// win whenever they happen to come last in the file.
		fields := strings.Fields(line[index+len(marker):])
		if len(fields) < 2 || fields[1] != "\u2192" {
			continue
		}

		state = fields[0]
	}

	return state, nil
}

func runConnectionEditRollbackSpec(nmapBackend string) {
	var backend *mcStack

	BeforeAll(func() {
		encodingChooseCorev1()

		var err error

		backend, err = newMCStack(context.Background())
		Expect(err).NotTo(HaveOccurred())

		GinkgoWriter.Printf("ManagementConsole stack up, router at %s (NMAP_BACKEND=%s)\n", backend.apiURL(), nmapBackend)

		extraCreateArgs = []string{
			"-e", "NMAP_BACKEND=" + nmapBackend,
			"-e", "USE_FSMV2_TRANSPORT=true",
			"-e", "API_URL=" + backend.apiURL(),
			"-e", "AUTH_TOKEN=" + mcAuthToken,
			"-e", "ALLOW_INSECURE_TLS=true",
			"--add-host=host.docker.internal:host-gateway",
		}

		err = BuildAndRunContainer(buildRollbackConfig(backend.apiURL()), DEFAULT_MEMORY, DEFAULT_CPUS)
		if err != nil {
			printContainerDebugInfo()
			Expect(err).NotTo(HaveOccurred(), "Container startup failed")
		}

		Expect(waitForMetrics()).To(Succeed(), "Metrics endpoint should be available")
	})

	AfterAll(func() {
		PrintLogsAndStopContainer()
		CleanupDockerBuildCache()

		if backend != nil {
			backend.stop()
		}

		extraCreateArgs = nil

		if !CurrentSpecReport().Failed() {
			cleanupTmpDirs(getContainerName())
		}
	})

	It("fails the edit and rolls back", func() {
		By("waiting for the container to log in to the ManagementConsole backend")
		Eventually(backend.loginSeen, 120*time.Second, 1*time.Second).Should(BeTrue(),
			"container must complete login against the ManagementConsole backend")

		By("waiting for the bridge to run with the reachable target")
		Eventually(lastBridgeState, 300*time.Second, 2*time.Second).Should(BeElementOf("idle", "active"),
			"bridge must run with the reachable target before the edit")

		By("re-pointing the connection at the unreachable target")

		actionUUID := uuid.New()
		bridgeUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(rollbackBridgeName)

		Expect(backend.enqueueEditProtocolConverter(
			actionUUID, bridgeUUID, rollbackBridgeName,
			rollbackUnreachableIP, rollbackUnreachablePort,
			rollbackReadDFCPayload(),
		)).To(Succeed())

		By("waiting for a terminal action-reply")

		var finalState models.ActionReplyState

		Eventually(func() bool {
			state, terminal := backend.lastReplyState(actionUUID)
			if terminal {
				finalState = state
			}

			return terminal
		}, 150*time.Second, 1*time.Second).Should(BeTrue(),
			"the edit must produce a terminal action-reply")

		replies := backend.replyDump(actionUUID)

		AddReportEntry(fmt.Sprintf("NMAP_BACKEND=%s reply history", nmapBackend), strings.Join(replies, "\n"))

		Expect(replies).To(ContainElement(ContainSubstring("Waiting for bridge")),
			"the edit must reach the rollout wait; a terminal reply without it failed earlier, "+
				"at parse/validate/persist, and says nothing about the connection")

		Expect(finalState).To(Equal(models.ActionFinishedWithFailure),
			"an edit to an unreachable target must fail")

		// A terminal failure alone does not say the previous configuration came
		// back: awaitRollout reports the same state when the rollback itself
		// fails, and when it aborts on a render error without dialling anything.
		// Only the message separates them.
		Expect(replies).To(ContainElement(ContainSubstring("Rolled back to previous configuration")),
			"the previous configuration must be restored, and the reply must say so")
		Expect(replies).NotTo(ContainElement(ContainSubstring("Rolling back to previous configuration failed")),
			"the rollback itself must succeed")

		By("editing back to the reachable target")

		goodActionUUID := uuid.New()

		Expect(backend.enqueueEditProtocolConverter(
			goodActionUUID, bridgeUUID, rollbackBridgeName,
			rollbackReachableIP, rollbackReachablePort,
			rollbackReadDFCPayload(),
		)).To(Succeed())

		// The positive control. Without it, a harness that fails every edit for
		// its own reasons satisfies everything above.
		var goodState models.ActionReplyState

		Eventually(func() bool {
			state, terminal := backend.lastReplyState(goodActionUUID)
			if terminal {
				goodState = state
			}

			return terminal
		}, 150*time.Second, 1*time.Second).Should(BeTrue(),
			"the second edit must produce a terminal action-reply")

		goodReplies := backend.replyDump(goodActionUUID)

		AddReportEntry(fmt.Sprintf("NMAP_BACKEND=%s reply history (edit back)", nmapBackend), strings.Join(goodReplies, "\n"))

		Expect(goodState).To(Equal(models.ActionFinishedSuccessfull),
			"an edit to a reachable target must succeed, or the check rejects everything and the "+
				"assertions above prove nothing")
	})
}

var _ = Describe("Connection edit to an unreachable target - NMAP_BACKEND=fsmv1",
	Ordered, Label("connection-edit-rollback"), func() {
		runConnectionEditRollbackSpec("fsmv1")
	})

var _ = Describe("Connection edit to an unreachable target - NMAP_BACKEND=fsmv2",
	Ordered, Label("connection-edit-rollback"), func() {
		runConnectionEditRollbackSpec("fsmv2")
	})
