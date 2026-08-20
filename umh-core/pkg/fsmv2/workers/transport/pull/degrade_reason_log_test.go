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

package pull_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	httptransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// nginx502Body is a real nginx 502 page, single-line, no control
// characters, so sanitizeErrorDetail passes it through verbatim and the test
// can read 'nginx/1.27.5' off the captured log line.
const nginx502Body = "<html><head><title>502 Bad Gateway</title></head><body><center><h1>502 Bad Gateway</h1></center><hr><center>nginx/1.27.5</center></body></html>"

var _ = Describe("ENG-5023: the upstream 502 reaches the logged Running->Degraded record", func() {
	It("captures the upstream nginx body on the state_transition record, Execute returns nil, nothing at Warn+", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(nginx502Body))
		}))
		defer server.Close()

		buf := new(bytes.Buffer)
		logger := newUnsampledJSONLogger(buf)

		transportpkg.SetChannelProvider(newTestChannelProvider())
		defer transportpkg.ClearChannelProvider()

		realHTTP := httptransport.NewHTTPTransport(server.URL, 5*time.Second)
		parentIdentity := deps.Identity{ID: "log-detail-parent", WorkerType: "transport"}
		parentDeps := transportpkg.NewTransportDependencies(realHTTP, deps.NewBaseDependencies(logger, nil, parentIdentity))

		pullIdentity := deps.Identity{ID: "log-detail-pull", WorkerType: "pull"}
		pullDeps, err := pull.NewPullDependencies(parentDeps, deps.NewBaseDependencies(logger, nil, pullIdentity))
		Expect(err).NotTo(HaveOccurred())

		act := &action.PullAction{JWTToken: "log-detail-token"}
		for range 3 {
			err := act.Execute(context.Background(), pullDeps)
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(pullDeps.GetConsecutiveErrors()).To(BeNumerically(">=", 3))
		Expect(pullDeps.GetLastStatusCode()).To(Equal(http.StatusBadGateway))
		Expect(pullDeps.GetLastErrorDetail()).To(ContainSubstring("nginx/1.27.5"))

		store := storage.NewTriangularStore(memory.NewInMemoryStore(), logger)
		sup := supervisor.NewSupervisor[fsmv2.Observation[snapshot.PullStatus], *fsmv2.WrappedDesiredState[snapshot.PullDesiredState]](supervisor.Config{
			WorkerType: "pull",
			Store:      store,
			Logger:     logger,
		})

		sup.TestUpdateUserSpec(fsmconfig.UserSpec{Config: "authSession:\n  token: log-detail-token\n  expiry: 2030-01-01T00:00:00Z\n"})

		worker, err := pull.NewPullWorker(pullIdentity, logger, nil, pullDeps)
		Expect(err).NotTo(HaveOccurred())
		Expect(sup.AddWorker(pullIdentity, worker)).To(Succeed())

		ctx := context.Background()

		Eventually(func() bool {
			_ = sup.TestTick(ctx)

			return capturedDegradeTransitionReason(buf) != ""
		}, "5s", "50ms").Should(BeTrue(),
			"a state_transition record from Running to Degraded must be captured")

		degradeReason := capturedDegradeTransitionReason(buf)
		Expect(degradeReason).To(ContainSubstring("; last: "))
		Expect(degradeReason).To(ContainSubstring("HTTP 502 (server_error)"))
		Expect(degradeReason).To(ContainSubstring("nginx/1.27.5"))

		Expect(stateTransitionLevelsAtOrAboveWarn(buf)).To(BeEmpty())
	})
})

func newUnsampledJSONLogger(buf *bytes.Buffer) deps.FSMLogger {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		MessageKey:     "msg",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}

	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), zapcore.AddSync(buf), zapcore.DebugLevel)

	return deps.NewUnsampledFSMLogger(zap.New(core).Sugar())
}

func capturedLogRecords(buf *bytes.Buffer) []map[string]any {
	var records []map[string]any

	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}

		records = append(records, m)
	}

	return records
}

func capturedDegradeTransitionReason(buf *bytes.Buffer) string {
	for _, rec := range capturedLogRecords(buf) {
		if msg, _ := rec["msg"].(string); msg != "state_transition" {
			continue
		}

		from, _ := rec["from_state"].(string)
		to, _ := rec["to_state"].(string)

		if from != "Running" || to != "Degraded" {
			continue
		}

		if reason, ok := rec["reason"].(string); ok {
			return reason
		}
	}

	return ""
}

func stateTransitionLevelsAtOrAboveWarn(buf *bytes.Buffer) []string {
	var levels []string

	for _, rec := range capturedLogRecords(buf) {
		if msg, _ := rec["msg"].(string); msg != "state_transition" {
			continue
		}

		level, _ := rec["level"].(string)
		if level == "warn" || level == "error" || level == "dpanic" || level == "panic" || level == "fatal" {
			levels = append(levels, level)
		}
	}

	return levels
}
