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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
)

var _ = Describe("Application supervisor shutdown", func() {
	const (
		configWorkerKey = "configworker"
		maxFreshAge     = 10 * time.Second
	)

	AfterEach(func() {
		register.ClearDeps(configWorkerKey)
	})

	It("leaves a worker removed in the shutdown drain reading as Deleted", func() {
		ctx := context.Background()
		logger := deps.NewNopFSMLogger()

		w := dynamicchildren.NewWriter()
		register.SetDeps[*dynamicchildren.Registry](configWorkerKey, w.Registry())

		ref := dynamicchildren.Ref{WorkerType: "helloworld", Name: "hello-1"}
		Expect(w.Upsert(ref, map[string]any{"state": "running"})).To(Succeed())

		sup, store, _ := newAppSupervisorWithStore(logger)

		// The shutdown drain waits for the tick loop to remove each worker, so
		// this test runs the real tick loop rather than ticking by hand.
		runCtx, cancel := context.WithCancel(ctx)
		DeferCleanup(cancel)

		done := sup.Start(runCtx)

		client := fsmv2client.NewFSMv2Client(w, store)

		Eventually(func() fsmv2client.Freshness {
			_, freshness, _ := fsmv2client.GetFresh[hello_world.HelloworldStatus](ctx, client, ref, maxFreshAge)

			return freshness
		}, "10s", "100ms").Should(Equal(fsmv2client.Fresh),
			"the helloworld worker must first run and report a fresh observation")

		sup.Shutdown()
		Eventually(done, "15s").Should(BeClosed(), "the supervisor must finish shutting down")

		obs, err := fsmv2client.Get[hello_world.HelloworldStatus](ctx, client, ref)
		Expect(errors.Is(err, fsmv2client.ErrWorkerDeleted)).To(BeTrue(),
			"after shutdown, Get on a worker the drain removed must return ErrWorkerDeleted, got %v", err)
		Expect(obs.State).To(BeEmpty())
	})
})
