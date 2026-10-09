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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// configTplWorkerType is a test-only worker type. No production worker carries
// a config.Template field yet, so this suite registers a minimal one to
// observe what a child renders from the values the application hands down.
const configTplWorkerType = "configtpl"

// configTplUserConfig is the typed configuration of the configtpl worker: one
// Template field, so the suite can read what the worker rendered.
type configTplUserConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	Read                config.Template `yaml:"read"`
}

// configTplStatus surfaces the rendered Read text so a test can read it back
// through the client.
type configTplStatus struct {
	Rendered string `json:"rendered"`
}

// configTplWorker embeds WorkerBase the way a production worker does, so
// DeriveDesiredState renders its Template field from the names WorkerBase
// builds.
type configTplWorker struct {
	fsmv2.WorkerBase[configTplUserConfig, configTplStatus, struct{}]
}

// CollectObservedState copies the rendered Read text into the status.
func (w *configTplWorker) CollectObservedState(_ context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	status := configTplStatus{}

	if wds, ok := desired.(*fsmv2.WrappedDesiredState[configTplUserConfig]); ok {
		status.Rendered = string(wds.Config.Read)
	}

	return fsmv2.NewObservation(status), nil
}

// configTplStoppedState is the initial state: it starts the worker.
type configTplStoppedState struct {
	helpers.StoppedBase
}

// Next starts the worker, or signals removal when a stop is required.
func (s *configTplStoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[configTplUserConfig, configTplStatus](snapAny)

	if snap.ShouldStop() {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil,
			"stop required: "+snap.StopReason(), nil)
	}

	return fsmv2.Transition(&configTplRunningState{}, fsmv2.SignalNone, nil, "starting", nil)
}

// String returns the state name for logging.
func (s *configTplStoppedState) String() string {
	return helpers.DeriveStateName(s)
}

// configTplRunningState is the steady state.
type configTplRunningState struct {
	helpers.RunningHealthyBase
}

// Next stays running, or returns to stopped when a stop is required.
func (s *configTplRunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[configTplUserConfig, configTplStatus](snapAny)

	if snap.ShouldStop() {
		return fsmv2.Transition(&configTplStoppedState{}, fsmv2.SignalNone, nil,
			"stop required: "+snap.StopReason(), nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "running", nil)
}

// String returns the state name for logging.
func (s *configTplRunningState) String() string {
	return helpers.DeriveStateName(s)
}

func init() {
	register.Worker[configTplUserConfig, configTplStatus, register.NoDeps](configTplWorkerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, _ map[string]any) (fsmv2.Worker, error) {
			w := &configTplWorker{}
			w.InitBase(id, logger, sr)

			return w, nil
		})

	fsmv2.RegisterInitialState(configTplWorkerType, &configTplStoppedState{})
}

var _ = Describe("Application supervisor hands the registry's config values to a child as the child's own section", func() {
	const configWorkerKey = "configworker"

	AfterEach(func() {
		register.ClearGlobalDeps(configWorkerKey)
	})

	It("delivers SetConfigVariables values to a registry child's own config section and renders its template with them", func() {
		ctx := context.Background()
		logger := deps.NewNopFSMLogger()

		cfgValues := map[string]any{
			"historian": map[string]any{
				"timescale": map[string]any{"host": "db"},
			},
		}

		w := dynamicchildren.NewWriter()
		register.SetGlobalDeps[*dynamicchildren.Registry](configWorkerKey, w.Registry())

		sup, store, _ := newAppSupervisorWithStore(logger)
		sup.TestMarkAsStarted()

		bridge := fsmv2client.NewFSMv2Client(w, store)
		Expect(bridge.SetConfigVariables(cfgValues)).To(Succeed(),
			"a valid values map must be accepted")

		ref := dynamicchildren.Ref{WorkerType: configTplWorkerType, Name: "tpl-1"}
		Expect(bridge.Upsert(ref, map[string]any{
			"state": "running",
			"read":  "{{ .config.historian.timescale.host }}",
		})).To(Succeed())

		Eventually(func(g Gomega) {
			_ = sup.TestTick(ctx)

			obs, err := fsmv2client.Get[configTplStatus](ctx, bridge, ref)
			g.Expect(err).NotTo(HaveOccurred(),
				"the spawned child must be observable through the client")
			g.Expect(obs.Status.Rendered).To(Equal("db"),
				"the child must render {{ .config.historian.timescale.host }} from the values the application hands down")
		}, "5s", "100ms").Should(Succeed(),
			"the values set through the client must reach the spawned child and render its template")

		docAny, err := store.LoadDesired(ctx, ref.WorkerType, config.ChildID(ref.Name)) //nolint:staticcheck // the spec reads the raw document, so the typed loader does not fit
		Expect(err).NotTo(HaveOccurred())

		doc, ok := docAny.(persistence.Document)
		Expect(ok).To(BeTrue())

		spec, ok := doc["originalUserSpec"].(map[string]any)
		Expect(ok).To(BeTrue(), "the child's stored desired document must carry its user spec")

		Expect(spec["config_values"]).To(Equal(cfgValues),
			"the values must reach the child's user spec under their own config_values key")

		vars, _ := spec["variables"].(map[string]any)
		Expect(vars).NotTo(HaveKey("historian"),
			"the values must not be merged into the child's variables")
	})
})
