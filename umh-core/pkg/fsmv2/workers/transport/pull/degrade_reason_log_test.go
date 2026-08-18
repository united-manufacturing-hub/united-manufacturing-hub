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
		// (1) A real upstream that answers every pull with a recognisable nginx 502 page.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(nginx502Body))
		}))
		defer server.Close()

		// Capturing logger WITHOUT the production samplers: NewUnsampledFSMLogger,
		// not NewFSMLogger (1s/5/100) nor the zap-level sampler (10s/3/100), both of
		// which can drop a record before an observer core sees it.
		buf := new(bytes.Buffer)
		logger := newUnsampledJSONLogger(buf)

		transportpkg.SetChannelProvider(newTestChannelProvider())
		defer transportpkg.ClearChannelProvider()

		// (2) Real HTTPTransport against the live server, real PullDependencies.
		realHTTP := httptransport.NewHTTPTransport(server.URL, 5*time.Second)
		parentIdentity := deps.Identity{ID: "log-detail-parent", WorkerType: "transport"}
		parentDeps := transportpkg.NewTransportDependencies(realHTTP, deps.NewBaseDependencies(logger, nil, parentIdentity))

		pullIdentity := deps.Identity{ID: "log-detail-pull", WorkerType: "pull"}
		pullDeps, err := pull.NewPullDependencies(parentDeps, deps.NewBaseDependencies(logger, nil, pullIdentity))
		Expect(err).NotTo(HaveOccurred())

		// (3) Drive the real action against the real 502 until the consecutive-error
		// threshold (errorDegradedThreshold = 3) is crossed.
		act := &action.PullAction{JWTToken: "log-detail-token"}
		for range 3 {
			err := act.Execute(context.Background(), pullDeps)
			// (b) ENG-4450 invariant: a 502 is ErrorTypeServerError, IsTransient()==true,
			// so Execute must return nil — this fix must not re-introduce the Sentry noise.
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(pullDeps.GetConsecutiveErrors()).To(BeNumerically(">=", 3))
		Expect(pullDeps.GetLastStatusCode()).To(Equal(http.StatusBadGateway))
		Expect(pullDeps.GetLastErrorDetail()).To(ContainSubstring("nginx/1.27.5"))

		// (4) Run the supervisor far enough that Running -> Degraded is reconciled
		// and the state_transition record is emitted. The fix's only observable
		// effect lives on that logged reason; asserting a Next() return value would
		// reproduce the exact failure #2626 slipped past.
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

		// Wait for the Running -> Degraded transition itself. The transition fires
		// whether or not the cause is appended, so waiting on it (rather than on the
		// '; last: ' marker) makes the RED failure land on the missing body below
		// instead of timing out.
		Eventually(func() bool {
			_ = sup.TestTick(ctx)

			return capturedDegradeTransitionReason(buf) != ""
		}, "5s", "50ms").Should(BeTrue(),
			"a state_transition record from Running to Degraded must be captured")

		// (a) The whole feature: the logged edge carries the upstream response body
		// AND the '; last: ' marker — read off captured log output, never a return value.
		degradeReason := capturedDegradeTransitionReason(buf)
		Expect(degradeReason).To(ContainSubstring("; last: "))
		Expect(degradeReason).To(ContainSubstring("HTTP 502 (server_error)"))
		Expect(degradeReason).To(ContainSubstring("nginx/1.27.5"))

		// (c) Nothing from the state-transition path is at Warn or above. Scoped BY
		// MESSAGE: RecordTypedError legitimately emits SentryWarn("persistent_pull_failure")
		// once the failure-rate tracker crosses 90% over ~600 samples.
		Expect(stateTransitionLevelsAtOrAboveWarn(buf)).To(BeEmpty())
	})
})

// newUnsampledJSONLogger builds a JSON FSMLogger that captures every record
// without the production message-based samplers (see spec Verification Strategy:
// use deps.NewUnsampledFSMLogger, not NewFSMLogger/NewJSONFSMLogger).
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

// capturedLogRecords parses every JSON line written to buf into a map.
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

// capturedDegradeTransitionReason returns the "reason" field of the captured
// 'state_transition' record that moves the worker from Running to Degraded, or
// "" if no such record has been captured yet.
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

// stateTransitionLevelsAtOrAboveWarn returns the level of every captured record
// with message 'state_transition' that sits at or above zapcore.WarnLevel.
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
