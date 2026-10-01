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

package hello_world_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

type fakeMoodFilesystem struct {
	filesystem.Service
	contents string
}

func (f fakeMoodFilesystem) ReadFile(_ context.Context, _ string) ([]byte, error) {
	return []byte(f.contents), nil
}

var _ = Describe("HelloworldWorker", func() {
	var (
		worker fsmv2.Worker
		logger deps.FSMLogger
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
	})

	Describe("NewHelloworldWorker", func() {
		It("should create worker successfully", func() {
			identity := deps.Identity{ID: "test-worker", WorkerType: "helloworld"}
			w, err := hello_world.NewHelloworldWorker(identity, logger, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(w).NotTo(BeNil())
		})

		It("should fail with nil logger", func() {
			identity := deps.Identity{ID: "test-worker", WorkerType: "helloworld"}
			w, err := hello_world.NewHelloworldWorker(identity, nil, nil)

			Expect(err).To(HaveOccurred())
			Expect(w).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("logger must not be nil"))
		})
	})

	Describe("CollectObservedState", func() {
		BeforeEach(func() {
			identity := deps.Identity{ID: "test-worker", WorkerType: "helloworld"}
			var err error
			worker, err = hello_world.NewHelloworldWorker(identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should collect initial state with HelloSaid=false", func() {
			desired := &fsmv2.WrappedDesiredState[hello_world.HelloworldConfig]{}
			obs, err := worker.CollectObservedState(context.Background(), desired)

			Expect(err).NotTo(HaveOccurred())
			typedObs, ok := obs.(fsmv2.Observation[hello_world.HelloworldStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HelloSaid).To(BeFalse())
		})

		It("should return error when context is cancelled", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			desired := &fsmv2.WrappedDesiredState[hello_world.HelloworldConfig]{}
			obs, err := worker.CollectObservedState(ctx, desired)

			Expect(err).To(Equal(context.Canceled))
			Expect(obs).To(BeNil())
		})
	})

	Describe("GetInitialState", func() {
		BeforeEach(func() {
			identity := deps.Identity{ID: "test-worker", WorkerType: "helloworld"}
			var err error
			worker, err = hello_world.NewHelloworldWorker(identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return Stopped state", func() {
			initialState := worker.GetInitialState()

			Expect(initialState).NotTo(BeNil())
			Expect(initialState.String()).To(Equal("Stopped"))
		})
	})

	Describe("DeriveDesiredState", func() {
		BeforeEach(func() {
			identity := deps.Identity{ID: "test-worker", WorkerType: "helloworld"}
			var err error
			worker, err = hello_world.NewHelloworldWorker(identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return running state when spec is nil", func() {
			desired, err := worker.DeriveDesiredState(nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(desired).NotTo(BeNil())
		})
	})

	Describe("GetDependenciesAny", func() {
		It("returns *HelloworldDependencies", func() {
			identity := deps.Identity{ID: "test-worker", WorkerType: "helloworld"}
			concrete, err := hello_world.NewHelloworldWorker(identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
			var w fsmv2.Worker = concrete
			dp, ok := w.(fsmv2.DependencyProvider)
			Expect(ok).To(BeTrue(), "worker must implement DependencyProvider")
			got := dp.GetDependenciesAny()
			_, ok = got.(*hello_world.HelloworldDependencies)
			Expect(ok).To(BeTrue(), "expected *HelloworldDependencies, got %T", got)
		})
	})

	Describe("Actions", func() {
		BeforeEach(func() {
			identity := deps.Identity{ID: "test-worker", WorkerType: "helloworld"}
			var err error
			worker, err = hello_world.NewHelloworldWorker(identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("contains SayHelloActionName with a non-nil value", func() {
			ap, ok := worker.(fsmv2.ActionProvider)
			Expect(ok).To(BeTrue(), "worker must implement ActionProvider")
			actions := ap.Actions()
			Expect(actions).To(HaveKey(hello_world.SayHelloActionName))
			Expect(actions[hello_world.SayHelloActionName]).NotTo(BeNil())
		})
	})

	Describe("SayHello action", func() {
		var d *hello_world.HelloworldDependencies

		BeforeEach(func() {
			identity := deps.Identity{ID: "test-id", WorkerType: "helloworld"}
			baseDeps := deps.NewBaseDependencies(logger, nil, identity)
			d = hello_world.NewHelloworldDependencies(baseDeps)
		})

		It("should set HelloSaid to true", func() {
			Expect(d.HasSaidHello()).To(BeFalse())

			err := hello_world.SayHello(context.Background(), d)

			Expect(err).NotTo(HaveOccurred())
			Expect(d.HasSaidHello()).To(BeTrue())
		})

		It("should be idempotent when called multiple times", func() {
			ctx := context.Background()

			err := hello_world.SayHello(ctx, d)
			Expect(err).NotTo(HaveOccurred())
			Expect(d.HasSaidHello()).To(BeTrue())

			err = hello_world.SayHello(ctx, d)
			Expect(err).NotTo(HaveOccurred())
			Expect(d.HasSaidHello()).To(BeTrue())

			err = hello_world.SayHello(ctx, d)
			Expect(err).NotTo(HaveOccurred())
			Expect(d.HasSaidHello()).To(BeTrue())
		})
	})

	Describe("mood file dependency", func() {
		buildWorker := func(dependencies map[string]any) fsmv2.Worker {
			identity := deps.Identity{ID: "mood-worker", Name: "mood-worker", WorkerType: "helloworld"}
			w, err := factory.NewWorkerByType("helloworld", identity, logger, nil, dependencies)
			Expect(err).NotTo(HaveOccurred())

			return w
		}

		moodOf := func(w fsmv2.Worker, moodFilePath string) string {
			desired := &fsmv2.WrappedDesiredState[hello_world.HelloworldConfig]{
				Config: hello_world.HelloworldConfig{MoodFilePath: moodFilePath},
			}
			obs, err := w.CollectObservedState(context.Background(), desired)
			Expect(err).NotTo(HaveOccurred())

			typedObs, ok := obs.(fsmv2.Observation[hello_world.HelloworldStatus])
			Expect(ok).To(BeTrue())

			return typedObs.Status.Mood
		}

		It("reads the mood through the filesystem under the dependency key, and through the real filesystem without it", func() {
			missingPath := filepath.Join(GinkgoT().TempDir(), "mood.txt")

			dependencies := map[string]any{}
			config.SetDependency(dependencies, hello_world.FilesystemKey, filesystem.Service(fakeMoodFilesystem{contents: "grumpy"}))

			Expect(moodOf(buildWorker(dependencies), missingPath)).To(Equal("grumpy"))

			realPath := filepath.Join(GinkgoT().TempDir(), "real-mood.txt")
			Expect(os.WriteFile(realPath, []byte("cheerful"), 0o600)).To(Succeed())

			Expect(moodOf(buildWorker(nil), realPath)).To(Equal("cheerful"))
		})
	})
})
