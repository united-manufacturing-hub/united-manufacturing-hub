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

package examples_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
)

var _ = Describe("v1 YAML runner caller-ctx cancellation", func() {
	It("tears down gracefully on a live tick loop when the caller ctx is cancelled mid-run", func() {
		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario: examples.Scenario{
				Name:        "v1-cancel-mid-run",
				Description: "test-local YAML scenario for the caller-ctx cancellation path",
				YAMLConfig:  "children: []",
			},
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		// Settle gate: cancel only once the application worker is observably
		// running, so the cancellation lands mid-run on a live tick loop. A
		// blind sleep can fire while the worker is still mid-startup, and a
		// mid-spawn worker intermittently cannot finish draining within one
		// graceful-shutdown phase budget. The YAML declares no children, so
		// the application worker is the only worker in the dump.
		Eventually(func(g Gomega) {
			dump, err := examples.DumpScenario(context.Background(), store, 0)
			g.Expect(err).NotTo(HaveOccurred())

			observedStates := map[string]interface{}{}
			for _, w := range dump.Workers {
				observedStates[w.WorkerType] = w.Observed["state"]
			}

			g.Expect(observedStates).To(HaveKeyWithValue(application.WorkerTypeName, "Running"))
		}, "30s").Should(Succeed(),
			"the application worker must be running before the mid-run cancellation")

		cancel()
		Eventually(result.Done, "55s").Should(BeClosed(),
			"cancelling the caller ctx must trigger a complete teardown")

		// Positive marker first: the cancel must reach an orderly Shutdown.
		Expect(logContainsEvent(logBuf.String(), "supervisor_shutting_down")).To(BeTrue(),
			"the caller-ctx cancel must trigger an orderly Shutdown, not a silent tick-loop death")

		// The graceful drain must run against a LIVE tick loop. If the tick
		// loop shared the caller's ctx, the cancel would kill it before
		// Shutdown, and every drain phase would wait out its timeout and
		// emit this warning.
		Expect(logContainsEvent(logBuf.String(), "graceful_shutdown_timeout")).To(BeFalse(),
			"the supervisor must drain via a live tick loop, not time out against a dead one")
	})
})
