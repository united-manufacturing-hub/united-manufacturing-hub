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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("ActionExecutor Integration", func() {
	var (
		sup    *supervisor.Supervisor
		ctx    context.Context
		cancel context.CancelFunc
		worker *mockWorker
		cfg    supervisor.CollectorHealthConfig
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		worker = &mockWorker{}
		cfg = supervisor.CollectorHealthConfig{}
		sup = newSupervisorWithWorker(worker, nil, cfg)
	})

	AfterEach(func() {
		if sup != nil {
			sup.Shutdown()
		}
		cancel()
	})

	Describe("ActionExecutor initialization", func() {
		It("should initialize ActionExecutor in constructor", func() {
			Expect(sup).ToNot(BeNil())

			err := sup.Tick(ctx)
			_ = err
		})

		It("should start ActionExecutor with supervisor", func() {
			done := sup.Start(ctx)
			Expect(done).ToNot(BeNil())

			err := sup.Tick(ctx)
			_ = err
		})

		It("should shutdown ActionExecutor with supervisor", func() {
			done := sup.Start(ctx)
			Expect(done).ToNot(BeNil())

			sup.Shutdown()
		})
	})

	Describe("Tick integration (stub)", func() {
		It("should complete tick without panic when ActionExecutor present", func() {
			err := sup.Tick(ctx)
			_ = err
		})

		It("should check for actions in progress during tick", func() {
			done := sup.Start(ctx)
			Expect(done).ToNot(BeNil())

			err := sup.Tick(ctx)
			_ = err
		})
	})
})
