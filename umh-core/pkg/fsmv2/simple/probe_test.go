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

package simple

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type probeRecordingDeps struct {
	probedConfigs *[]probeConfig
}

func recordProbedConfig(_ context.Context, d probeRecordingDeps, cfg probeConfig) {
	*d.probedConfigs = append(*d.probedConfigs, cfg)
}

var _ = Describe("probeAction", func() {
	It("runs the probe registered for the worker type with the worker's deps and the snapshot config", func() {
		var probedConfigs []probeConfig

		registerProbe("simpleprobe_runs", &typedProbe[probeConfig, probeRecordingDeps]{
			workerType: "simpleprobe_runs",
			probe:      recordProbedConfig,
		})

		action := &probeAction[probeConfig]{workerType: "simpleprobe_runs", cfg: probeConfig{Target: "plc:502"}}

		err := action.Execute(context.Background(), probeRecordingDeps{probedConfigs: &probedConfigs})

		Expect(err).NotTo(HaveOccurred())
		Expect(probedConfigs).To(Equal([]probeConfig{{Target: "plc:502"}}))
	})

	It("skips the probe and reports no failure when the context is already cancelled", func() {
		var probedConfigs []probeConfig

		registerProbe("simpleprobe_cancelled", &typedProbe[probeConfig, probeRecordingDeps]{
			workerType: "simpleprobe_cancelled",
			probe:      recordProbedConfig,
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		action := &probeAction[probeConfig]{workerType: "simpleprobe_cancelled"}

		err := action.Execute(ctx, probeRecordingDeps{probedConfigs: &probedConfigs})

		Expect(err).NotTo(HaveOccurred())
		Expect(probedConfigs).To(BeEmpty())
	})

	It("fails when no probe is registered for the worker type", func() {
		action := &probeAction[probeConfig]{workerType: "simpleprobe_unregistered"}

		err := action.Execute(context.Background(), probeRecordingDeps{})

		Expect(err).To(MatchError(ContainSubstring("simpleprobe_unregistered")))
	})

	It("fails when the worker's deps are not the type the probe expects", func() {
		registerProbe("simpleprobe_wrongdeps", &typedProbe[probeConfig, probeRecordingDeps]{
			workerType: "simpleprobe_wrongdeps",
			probe:      recordProbedConfig,
		})

		action := &probeAction[probeConfig]{workerType: "simpleprobe_wrongdeps"}

		err := action.Execute(context.Background(), struct{}{})

		Expect(err).To(MatchError(ContainSubstring("simpleprobe_wrongdeps")))
	})
})
