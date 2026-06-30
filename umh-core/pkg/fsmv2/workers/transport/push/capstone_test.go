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

package push_test

import (
	"bytes"
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// fNginxCapstone is a real nginx 502 HTML page with CRLF between tags; the
// double-quoted literal makes \r\n real control bytes, exercising the
// control-strip path of sanitizeErrorDetail.
const fNginxCapstone = "<html>\r\n<head><title>502 Bad Gateway</title></head>\r\n<body>\r\n<center><h1>502 Bad Gateway</h1></center>\r\n<hr><center>nginx</center>\r\n</body>\r\n</html>\r\n"

var _ = Describe("ENG-5275 incident reproduction (P8 capstone)", func() {
	It("surfaces a 502 response body on persistent_push_failure and the degraded reason", func() {
		// The shape of the ENG-5275 incident: a reverse proxy returning a 502
		// nginx page. ExtractErrorDetails is what the push action calls on the
		// transport error (verified by R5); this capstone drives the real
		// dependency path past escalation and checks the body survives.
		fNginxErr := &types.TransportError{
			Type:       types.ErrorTypeServerError,
			StatusCode: 502,
			Message:    "HTTP 502 (server_error): " + fNginxCapstone,
		}
		errType, retryAfter, statusCode, detail := types.ExtractErrorDetails(fNginxErr)

		buf := new(bytes.Buffer)
		jsonLogger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)
		transport.SetChannelProvider(newTestChannelProvider())
		parentDeps := createParentDeps(jsonLogger)
		identity := deps.Identity{ID: "push-capstone", WorkerType: "push"}
		pushDeps, err := push.NewPushDependencies(parentDeps, deps.NewBaseDependencies(jsonLogger, nil, identity))
		Expect(err).NotTo(HaveOccurred())

		// Drive past the escalation threshold (MinSamples=100, one-shot).
		for range transport.ChildFailureRateConfig.MinSamples {
			pushDeps.RecordTypedError(errType, retryAfter, statusCode, detail)
		}

		// (a) The persistent_push_failure Sentry event carries the status code
		// and the sanitized nginx body (control chars stripped, "502 Bad
		// Gateway" and "nginx" preserved).
		m := parseLastJSONLine(buf)
		Expect(m).NotTo(BeNil())
		Expect(m["msg"]).To(Equal("persistent_push_failure"))
		Expect(m["status_code"]).To(BeEquivalentTo(502))
		Expect(m["error_detail"]).To(ContainSubstring("502 Bad Gateway"))
		Expect(m["error_detail"]).To(ContainSubstring("nginx"))

		// (b) The degraded-state reason appends the same detail.
		worker, err := push.NewPushWorker(identity, jsonLogger, nil, pushDeps)
		Expect(err).NotTo(HaveOccurred())
		desired := &fsmv2.WrappedDesiredState[snapshot.PushDesiredState]{
			Config: snapshot.PushDesiredState{
				AuthSession: types.AuthSession{
					Token:  "valid-token",
					Expiry: time.Now().Add(time.Hour),
				},
			},
		}
		observed, err := worker.CollectObservedState(context.Background(), desired)
		Expect(err).NotTo(HaveOccurred())
		typedObs, ok := observed.(fsmv2.Observation[snapshot.PushStatus])
		Expect(ok).To(BeTrue())

		snap := fsmv2.Snapshot{
			Observed: typedObs,
			Desired:  desired,
			Identity: identity,
		}
		reason := (&state.DegradedState{}).Next(snap).Reason
		Expect(reason).To(ContainSubstring("; last: HTTP 502 (server_error):"))
		Expect(reason).To(ContainSubstring("nginx"))
	})
})
