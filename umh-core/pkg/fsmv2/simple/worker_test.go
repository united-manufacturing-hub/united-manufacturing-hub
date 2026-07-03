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

// Internal (white-box) tests: exercise the unexported worker + Register wiring.
package simple

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

type probeConfig struct {
	Target string `json:"target"`
}

type probeStatus struct {
	Reachable bool `json:"reachable"`
}

func newProbeWorker(spec Spec[probeConfig, probeStatus, struct{}]) (*simpleWorker[probeConfig, probeStatus, struct{}], error) {
	return newSimpleWorker(spec,
		deps.Identity{ID: "probe", WorkerType: spec.WorkerType},
		deps.NewNopFSMLogger(), nil)
}

var _ = Describe("simpleWorker", func() {
	Describe("CollectObservedState", func() {
		It("runs Poll and lands its status on the Observation", func() {
			var gotCfg probeConfig

			spec := Spec[probeConfig, probeStatus, struct{}]{
				WorkerType: "simpleworker_collect",
				Poll: func(_ context.Context, _ struct{}, cfg probeConfig) (probeStatus, error) {
					gotCfg = cfg

					return probeStatus{Reachable: true}, nil
				},
			}

			w, err := newProbeWorker(spec)
			Expect(err).NotTo(HaveOccurred())

			desired := &fsmv2.WrappedDesiredState[probeConfig]{Config: probeConfig{Target: "1.2.3.4"}}

			obs, err := w.CollectObservedState(context.Background(), desired)
			Expect(err).NotTo(HaveOccurred())
			Expect(gotCfg.Target).To(Equal("1.2.3.4"), "Poll receives the developer's config")

			o, ok := obs.(fsmv2.Observation[probeStatus])
			Expect(ok).To(BeTrue(), "observation is wrapped by the framework")
			Expect(o.Status.Reachable).To(BeTrue())
		})

		It("propagates a Poll error", func() {
			spec := Spec[probeConfig, probeStatus, struct{}]{
				WorkerType: "simpleworker_pollerr",
				Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
					return probeStatus{}, errors.New("dial timeout")
				},
			}

			w, err := newProbeWorker(spec)
			Expect(err).NotTo(HaveOccurred())

			desired := &fsmv2.WrappedDesiredState[probeConfig]{}

			_, err = w.CollectObservedState(context.Background(), desired)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dial timeout"))
		})

		It("honours context cancellation", func() {
			spec := Spec[probeConfig, probeStatus, struct{}]{
				WorkerType: "simpleworker_ctx",
				Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
					return probeStatus{}, nil
				},
			}

			w, err := newProbeWorker(spec)
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err = w.CollectObservedState(ctx, &fsmv2.WrappedDesiredState[probeConfig]{})
			Expect(err).To(MatchError(context.Canceled))
		})
	})

	Describe("dependencies", func() {
		It("reports a true-nil GetDependenciesAny so metrics injection is not skipped", func() {
			w, err := newProbeWorker(Spec[probeConfig, probeStatus, struct{}]{
				WorkerType: "simpleworker_deps",
				Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
					return probeStatus{}, nil
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(w.GetDependenciesAny()).To(BeNil())
		})
	})
})

var _ = Describe("Register", func() {
	AfterEach(func() {
		fsmv2.ResetInitialStateRegistry()
	})

	It("panics when WorkerType is empty", func() {
		Expect(func() {
			Register(Spec[probeConfig, probeStatus, struct{}]{
				Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
					return probeStatus{}, nil
				},
			})
		}).To(PanicWith(ContainSubstring("WorkerType")))
	})

	It("panics when Poll is nil", func() {
		Expect(func() {
			Register(Spec[probeConfig, probeStatus, struct{}]{WorkerType: "simpleworker_nopoll"})
		}).To(PanicWith(ContainSubstring("Poll")))
	})

	It("registers an initial state for the worker type", func() {
		Register(Spec[probeConfig, probeStatus, struct{}]{
			WorkerType: "simpleworker_register",
			Poll: func(_ context.Context, _ struct{}, _ probeConfig) (probeStatus, error) {
				return probeStatus{}, nil
			},
		})
		Expect(fsmv2.LookupInitialState("simpleworker_register")).NotTo(BeNil())
	})
})
