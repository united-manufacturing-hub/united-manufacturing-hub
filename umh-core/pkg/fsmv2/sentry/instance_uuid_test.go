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

package sentry_test

import (
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"
	umhsentry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"

	//nolint:revive // dot import for Ginkgo DSL
	. "github.com/onsi/ginkgo/v2"
	//nolint:revive // dot import for Gomega matchers
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ = Describe("SetInstanceUUID", func() {
	const instanceUUID = "a4c3c2b3-1fe8-465f-a73f-0222b1da17b5"

	var (
		transport *sentrygo.MockTransport
		hook      *sentry.SentryHook
		logger    *zap.SugaredLogger
	)

	BeforeEach(func() {
		transport = &sentrygo.MockTransport{}
		client, err := sentrygo.NewClient(sentrygo.ClientOptions{
			Dsn:       "https://test@sentry.io/123",
			Transport: transport,
		})
		Expect(err).NotTo(HaveOccurred())

		// The pushed scope confines the instance_uuid tag to this spec, and
		// the bound client routes the hook's captures to the recording
		// transport. The hook clones the current hub at capture time
		// (captureToSentry), so it inherits both.
		sentrygo.CurrentHub().PushScope()
		sentrygo.CurrentHub().BindClient(client)

		hook = sentry.NewSentryHook(time.Hour)
		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(&discardWriter{}),
			zapcore.DebugLevel,
		)
		logger = zap.New(hook.Wrap(core)).Sugar()
	})

	AfterEach(func() {
		sentrygo.CurrentHub().PopScope()
		hook.Stop()
	})

	It("tags FSMv2 hook events with the instance UUID", func() {
		umhsentry.SetInstanceUUID(instanceUUID)

		// captureToSentry runs synchronously in the hook's Write, so the
		// transport holds the event once the log call returns. The message is
		// unique in this suite, so the fingerprint debouncer cannot drop it.
		logger.Errorw("SetInstanceUUID hook test error",
			"feature", "instance_uuid_test")

		events := transport.Events()
		Expect(events).To(HaveLen(1), "expected exactly one hook event")
		Expect(events[0].Tags["instance_uuid"]).To(Equal(instanceUUID))
	})
})
