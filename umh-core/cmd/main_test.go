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
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

func TestMain(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Main Suite")
}

var _ = Describe("communicatorEnabled", func() {
	// communicatorEnabled is the decision main() consults to gate the FSMv2
	// communicator. It is a pure function of the backend configuration, so it
	// is asserted directly here rather than by running main(). The
	// keep-the-legacy knob (UseFSMv2Transport) no longer routes anywhere: the
	// legacy backend path was deleted, so the communicator is the only path it
	// gates.

	It("enables the communicator when credentials are present, even with the legacy transport switch off", func() {
		cfg := config.FullConfig{
			Agent: config.AgentConfig{
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:            "http://fsmv2.invalid:9999",
					AuthToken:         "test-token",
					UseFSMv2Transport: false,
				},
			},
		}

		Expect(communicatorEnabled(&cfg)).To(BeTrue())
	})

	It("does not enable the communicator when credentials are absent", func() {
		cfg := config.FullConfig{}

		Expect(communicatorEnabled(&cfg)).To(BeFalse())
	})

	It("does not enable the communicator when only API_URL is set", func() {
		cfg := config.FullConfig{
			Agent: config.AgentConfig{
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL: "http://fsmv2.invalid:9999",
				},
			},
		}

		// The && credential contract must reject partial state; an operator with
		// one field empty should not run without a token.
		Expect(communicatorEnabled(&cfg)).To(BeFalse())
	})

	It("does not enable the communicator when only AUTH_TOKEN is set", func() {
		cfg := config.FullConfig{
			Agent: config.AgentConfig{
				CommunicatorConfig: config.CommunicatorConfig{
					AuthToken: "test-token",
				},
			},
		}

		Expect(communicatorEnabled(&cfg)).To(BeFalse())
	})
})

var _ = Describe("counting historian bridges", func() {
	historianBridge := func() config.ProtocolConverterConfig {
		return config.ProtocolConverterConfig{
			ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
						Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
							Protocol: dataflowcomponentserviceconfig.HistorianDestinationProtocol,
						},
					},
				},
			},
		}
	}

	nonHistorianBridge := func() config.ProtocolConverterConfig {
		return config.ProtocolConverterConfig{
			ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
						Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
							Protocol: "kafka",
						},
					},
				},
			},
		}
	}

	It("counts only bridges writing to the historian", func() {
		cfg := config.FullConfig{
			ProtocolConverter: []config.ProtocolConverterConfig{
				historianBridge(),
				nonHistorianBridge(),
				historianBridge(),
			},
		}

		Expect(countHistorianBridges(cfg)).To(Equal(2))
	})

	It("returns zero when no bridges write to the historian", func() {
		cfg := config.FullConfig{
			ProtocolConverter: []config.ProtocolConverterConfig{
				nonHistorianBridge(),
			},
		}

		Expect(countHistorianBridges(cfg)).To(Equal(0))
	})
})

var _ = Describe("buildFSMv2Supervisor", func() {
	// With no Management Console credentials, the FSMv2 runtime used to be
	// skipped entirely: buildFSMv2Supervisor was only called when APIURL and
	// AuthToken were both set, so with no credentials no runtime existed and
	// every adapter-driven worker reported "starting" forever with no error,
	// no Sentry event and no exit. bringup_invariant_test.go pins main()'s
	// control flow so it can no longer skip the call; this spec pins the
	// constructor itself, which that structural guard cannot reach. Asserting
	// only a nil error would pass for a constructor that returns a nil
	// supervisor alongside it, leaving the runtime just as absent, so both
	// are checked.
	//
	// The constructor reads neither credential today, so what this spec pins is
	// their absence: it starts failing the moment either one is consulted here.
	// It does not exercise credential handling, which lives in
	// communicatorEnabled.
	//
	// The nil arguments below are unread on this path. Four of them panic if
	// that ever changes, which a spec reports as a failure. The nil
	// SystemSnapshotManager is the exception: every method on it returns early
	// on a nil receiver, so a future read is silently skipped rather than
	// reported. Pass a real one before asserting anything that depends on it.
	It("succeeds and returns a non-nil supervisor with empty credentials", func() {
		cfg := &config.FullConfig{
			Agent: config.AgentConfig{
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "",
					AuthToken: "",
				},
			},
		}

		commState := communication_state.NewCommunicationState(
			nil,
			make(chan *models.UMHMessage, 1),
			make(chan *models.UMHMessage, 1),
			"",
			nil,
			nil,
			logger.For(logger.ComponentCore),
			nil,
			nil,
		)

		// The channel bridge this builds starts two goroutines that exit only on
		// ctx.Done(), and cleanup cannot reap them, so the context has to be
		// cancellable. context.Background() would leak both for the life of the
		// test binary.
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		appSup, _, _, _, cleanup, err := buildFSMv2Supervisor(ctx, cfg, commState, logger.For(logger.ComponentCore), nil)
		DeferCleanup(cleanup)

		Expect(err).NotTo(HaveOccurred())
		Expect(appSup).NotTo(BeNil())
	})
})
