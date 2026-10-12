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

package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// childrenSpecsParent renders the child specs its user config carries, so a
// test changes what the children are by updating the parent's config, through
// the same derive cache a real parent uses.
type childrenSpecsParent struct{}

// childrenSpecsConfig holds the child specs the parent's user config carries.
type childrenSpecsConfig struct {
	Children []config.ChildSpec `yaml:"children"`
}

func (w *childrenSpecsParent) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: "parent-worker", CollectedAt: time.Now()}, nil
}

func (w *childrenSpecsParent) DeriveDesiredState(spec any) (fsmv2.DesiredState, error) {
	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return &config.DesiredState{State: "running"}, nil
	}

	var cfg childrenSpecsConfig
	if err := yaml.Unmarshal([]byte(userSpec.Config), &cfg); err != nil {
		return nil, &config.ConfigError{Field: "children", Err: err}
	}

	return &config.DesiredState{State: "running", ChildrenSpecs: cfg.Children}, nil
}

func (w *childrenSpecsParent) GetInitialState() fsmv2.State[any, any] { return &mockState{} }

var _ = Describe("The reducer's not-found warning and config errors", func() {
	ctx := context.Background()

	registerChildTypes := func() {
		Expect(factory.RegisterFactoryByType("configerrorchild",
			func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
				return &configTextDeriveWorker{}
			})).NotTo(HaveOccurred())

		Expect(factory.RegisterSupervisorFactoryByType("configerrorchild", func(cfg any) any {
			return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg.(supervisor.Config))
		})).NotTo(HaveOccurred())

		Expect(factory.RegisterFactoryByType("plainerrorchild",
			func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
				return &plainErrorDeriveWorker{}
			})).NotTo(HaveOccurred())

		Expect(factory.RegisterSupervisorFactoryByType("plainerrorchild", func(cfg any) any {
			return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg.(supervisor.Config))
		})).NotTo(HaveOccurred())
	}

	newParent := func(recorder *recordingSentryLogger) *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState] {
		parentSuper := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: "parent",
			Store:      newMockTriangularStore(),
			Logger:     recorder,
		})

		Expect(parentSuper.AddWorker(deps.Identity{
			ID:         "parent-worker",
			Name:       "Parent Worker",
			WorkerType: "parent",
		}, &childrenSpecsParent{})).NotTo(HaveOccurred())

		return parentSuper
	}

	childrenYAML := func(children ...string) string {
		out := "children:\n"

		for _, child := range children {
			out += "  - " + child + "\n"
		}

		return out
	}

	It("keeps a child held back by a config error out of the Sentry warning", func() {
		registerChildTypes()

		recorder := newRecordingSentryLogger()
		parentSuper := newParent(recorder)

		parentSuper.TestUpdateUserSpec(config.UserSpec{Config: childrenYAML(
			`{name: held-child, workerType: configerrorchild, enabled: true, userSpec: {config: "{{ first mistake"}}`,
		)})

		for range 5 {
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
		}

		Expect(parentSuper.GetChildren()).To(BeEmpty())
		Expect(recorder.warnCount("config_error")).To(Equal(1))
		Expect(recorder.sentryWarnEvents()).NotTo(ContainElement("reducer_error"))
		Expect(recorder.sentryErrorEvents()).To(BeEmpty())
	})

	It("still warns when a child goes missing for another reason after a held child is created", func() {
		registerChildTypes()

		recorder := newRecordingSentryLogger()
		parentSuper := newParent(recorder)

		parentSuper.TestUpdateUserSpec(config.UserSpec{Config: childrenYAML(
			`{name: fixed-child, workerType: configerrorchild, enabled: true, userSpec: {config: "{{ first mistake"}}`,
		)})

		Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
		Expect(parentSuper.GetChildren()).To(BeEmpty())
		Expect(recorder.warnCount("config_error")).To(Equal(1))

		By("fixing the config and creating the child")

		parentSuper.TestUpdateUserSpec(config.UserSpec{Config: childrenYAML(
			`{name: fixed-child, workerType: configerrorchild, enabled: true, userSpec: {config: "state: running"}}`,
		)})

		Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
		Expect(parentSuper.GetChildren()).To(HaveKey("fixed-child"))

		By("a child failing for a non-config reason is genuinely missing and warns")

		parentSuper.TestUpdateUserSpec(config.UserSpec{Config: childrenYAML(
			`{name: fixed-child, workerType: configerrorchild, enabled: true, userSpec: {config: "state: running"}}`,
			`{name: plain-child, workerType: plainerrorchild, enabled: true, userSpec: {config: "state: running"}}`,
		)})

		Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())

		Expect(parentSuper.GetChildren()).To(HaveLen(1))
		Expect(parentSuper.GetChildren()).To(HaveKey("fixed-child"))
		Expect(recorder.sentryWarnEvents()).To(ContainElement("reducer_error"))
	})
})
