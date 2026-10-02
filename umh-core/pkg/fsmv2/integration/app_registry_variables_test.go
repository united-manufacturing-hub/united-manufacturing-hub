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
	"bytes"
	"context"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// lockedBuffer is a bytes.Buffer that the supervisor's goroutines can write
// while the spec reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

var _ = Describe("Application supervisor passes the registry's variable bundle to its children", func() {
	const configWorkerKey = "configworker"

	AfterEach(func() {
		register.ClearGlobalDeps(configWorkerKey)
	})

	It("gives a registry child and the config-worker kernel the variable bundle, Global included", func() {
		ctx := context.Background()

		w := dynamicchildren.NewWriter()
		register.SetGlobalDeps[*dynamicchildren.Registry](configWorkerKey, w.Registry())
		w.SetVariables(config.VariableBundle{
			User:   map[string]any{"IP": "10.0.0.1"},
			Global: map[string]any{"cluster_id": "c1"},
		})
		Expect(w.Upsert(dynamicchildren.Ref{WorkerType: "helloworld", Name: "hello-1"},
			map[string]any{"state": "running"})).To(Succeed())

		sup, store, _ := newAppSupervisorWithStore(deps.NewNopFSMLogger())
		sup.TestMarkAsStarted()

		storedSpecVariables := func(workerType, id string) map[string]any {
			docAny, err := store.LoadDesired(ctx, workerType, id) //nolint:staticcheck // the spec reads the raw document, so the typed loader does not fit
			if err != nil {
				return nil
			}

			doc, ok := docAny.(persistence.Document)
			if !ok {
				return nil
			}

			spec, ok := doc["originalUserSpec"].(map[string]any)
			if !ok {
				return nil
			}

			vars, _ := spec["variables"].(map[string]any)

			return vars
		}

		children := []struct{ workerType, id string }{
			{"helloworld", "hello-1-001"},
			{"configworker", "config-worker-001"},
		}

		Eventually(func(g Gomega) {
			_ = sup.TestTick(ctx)

			for _, c := range children {
				vars := storedSpecVariables(c.workerType, c.id)
				g.Expect(vars).To(HaveKeyWithValue("user", HaveKeyWithValue("IP", "10.0.0.1")), "%s", c.id)
				g.Expect(vars).To(HaveKeyWithValue("global", HaveKeyWithValue("cluster_id", "c1")), "%s", c.id)
			}
		}, "5s", "100ms").Should(Succeed(),
			"every child the application renders must receive the registry's variable bundle, user and global")

		w.SetVariables(config.VariableBundle{
			User:   map[string]any{"IP": "10.0.0.2"},
			Global: map[string]any{"cluster_id": "c2"},
		})

		Eventually(func(g Gomega) {
			_ = sup.TestTick(ctx)

			for _, c := range children {
				vars := storedSpecVariables(c.workerType, c.id)
				g.Expect(vars).To(HaveKeyWithValue("user", HaveKeyWithValue("IP", "10.0.0.2")), "%s", c.id)
				g.Expect(vars).To(HaveKeyWithValue("global", HaveKeyWithValue("cluster_id", "c2")), "%s", c.id)
			}
		}, "5s", "100ms").Should(Succeed(),
			"after SetVariables replaces the bundle, children that already run must receive the new values, user and global")
	})

	It("keeps the registry's value over an own child's, and warns once", func() {
		w := dynamicchildren.NewWriter()
		register.SetGlobalDeps[*dynamicchildren.Registry](configWorkerKey, w.Registry())
		w.SetVariables(config.VariableBundle{User: map[string]any{"IP": "10.0.0.1"}})

		logs := &lockedBuffer{}
		sup, store, appID := newAppSupervisorWithStore(deps.NewJSONFSMLogger(logs, deps.LevelWarn))
		sup.TestUpdateUserSpec(config.UserSpec{Config: `children:
  - name: own-hello
    workerType: helloworld
    userSpec:
      config: "state: running\n"
      variables:
        user:
          IP: ip-from-yaml
`})

		ctx, cancel := context.WithCancel(context.Background())
		done := sup.Start(ctx)

		defer func() { cancel(); <-done }()

		conflictLines := func() []string {
			var lines []string

			for _, line := range strings.Split(logs.String(), "\n") {
				if strings.Contains(line, `"msg":"registry_variable_overrides_child"`) {
					lines = append(lines, line)
				}
			}

			return lines
		}

		Eventually(func(g Gomega) {
			doc, err := store.LoadDesired(ctx, "helloworld", "own-hello-001") //nolint:staticcheck // the spec reads the raw document, so the typed loader does not fit
			g.Expect(err).NotTo(HaveOccurred())
			spec, _ := doc.(persistence.Document)["originalUserSpec"].(map[string]any)
			vars, _ := spec["variables"].(map[string]any)
			g.Expect(vars).To(HaveKeyWithValue("user", HaveKeyWithValue("IP", "10.0.0.1")))
		}, "5s", "100ms").Should(Succeed())
		Eventually(conflictLines, "5s", "100ms").Should(HaveLen(1))
		distinctCollectedAt := map[time.Time]struct{}{}
		recordCollectedAt := func() {
			obs, err := storage.LoadObservedTyped[fsmv2.Observation[snapshot.ApplicationStatus]](store, ctx, appID)
			if err != nil {
				return
			}

			distinctCollectedAt[obs.CollectedAt] = struct{}{}
		}

		Consistently(func(g Gomega) {
			g.Expect(conflictLines()).To(HaveLen(1))

			recordCollectedAt()
		}, "3s", "200ms").Should(Succeed())
		Expect(len(distinctCollectedAt)).To(BeNumerically(">=", 2),
			"the collector must run at least twice inside the window, or the check that the warning appears once means nothing")
		Expect(conflictLines()[0]).To(And(ContainSubstring(`"child_name":"own-hello"`), ContainSubstring(`"namespace":"User"`), ContainSubstring(`"key":"IP"`)))
		Expect(logs.String()).NotTo(ContainSubstring("ip-from-yaml"), "the warning names the key, never a value")
	})
})
