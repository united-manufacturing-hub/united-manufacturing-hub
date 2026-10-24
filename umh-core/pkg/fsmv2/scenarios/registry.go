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

package scenarios

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/scenariorunner"
)

// Registry contains all available scenarios, merged into ListScenarios
// alongside LiveRegistry. A name may be in only one of the two; the spec
// "keeps the registries' names disjoint" in scenario_test.go says why.
var Registry = map[string]scenariorunner.Scenario{
	"noop":         NoopScenario,
	"helloworld":   HelloworldScenario,
	"failing":      FailingScenario,
	"slow":         SlowScenario,
	"timeout":      TimeoutScenario,
	"panic":        PanicScenario,
	"dynamic":      DynamicScenario,
	"nmap":         NmapScenario,
	"transport":    TransportScenario,
	"concurrent":   ConcurrentScenario,
	"simple":       SimpleScenario,
	"cascade":      CascadeScenario,
	"configerror":  ConfigErrorScenario,
	"communicator": CommunicatorScenario,
	"persistence":  PersistenceScenario,

	"certfetcher-healthy":        CertFetcherHealthyScenario,
	"certfetcher-degraded":       CertFetcherDegradedScenario,
	"certfetcher-no-subscribers": CertFetcherNoSubscribersScenario,
	"historian":                  HistorianScenario,

	"cpu-pressure": CPUPressureScenario,
	"cpu-blind":    CPUBlindScenario,
	"cpu-stall":    CPUStallScenario,
	"cpu-filling":  CPUFillingScenario,
	"cpu-latch":    CPULatchScenario,
	"cpu-v1":       CPUV1Scenario,
}

// LiveRegistry holds scenarios that read the real machine the runner runs
// on, so their result depends on that machine's CPU load and cgroup files. The
// CLI runs them. The registry spec does not, because CI does not control those.
var LiveRegistry = map[string]scenariorunner.Scenario{
	"cpu-host": CPUHostScenario,
}

// ListScenarios returns all registered scenario names and descriptions,
// merging every registry into one listing.
func ListScenarios() map[string]string {
	result := make(map[string]string, len(Registry)+len(LiveRegistry))

	for name, scenario := range Registry {
		result[name] = scenario.Description
	}

	for name, scenario := range LiveRegistry {
		result[name] = scenario.Description
	}

	return result
}
