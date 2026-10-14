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

package sentry

import (
	"errors"

	"github.com/getsentry/sentry-go"

	//nolint:revive // dot import for Ginkgo DSL
	. "github.com/onsi/ginkgo/v2"
	//nolint:revive // dot import for Gomega matchers
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

var _ = Describe("SetInstanceUUID", func() {
	const instanceUUID = "a4c3c2b3-1fe8-465f-a73f-0222b1da17b5"

	var (
		transport      *sentry.MockTransport
		log            *zap.SugaredLogger
		debounceBefore bool
	)

	BeforeEach(func() {
		transport = &sentry.MockTransport{}
		client, err := sentry.NewClient(sentry.ClientOptions{
			Dsn:       "https://test@sentry.io/123",
			Transport: transport,
		})
		Expect(err).NotTo(HaveOccurred())

		// The pushed scope confines the instance_uuid tag to this spec;
		// PopScope in teardown drops it, so later specs see an untagged scope.
		sentry.CurrentHub().PushScope()
		sentry.CurrentHub().BindClient(client)

		// reportError drops an event only when the last error was sent less
		// than two hours ago, and errorLastSent starts 24 hours in the past,
		// so a fresh process sends its first error either way. With debouncing
		// forced off, the capture is deterministic regardless of which specs
		// ran earlier in the suite.
		debounceBefore = shouldDebounceErrors
		shouldDebounceErrors = false

		log = zaptest.NewLogger(GinkgoT()).Sugar()
	})

	AfterEach(func() {
		sentry.CurrentHub().PopScope()
		shouldDebounceErrors = debounceBefore
	})

	It("tags ReportIssue errors and recovered panics with the instance UUID", func() {
		SetInstanceUUID(instanceUUID)

		By("sending one error through ReportIssue")
		ReportIssue(errors.New("SetInstanceUUID test error"), IssueTypeError, log)

		By("sending one panic through RecoverAndReport")
		func() {
			defer RecoverAndReport()
			panic("SetInstanceUUID test panic")
		}()

		events := transport.Events()
		Expect(events).To(HaveLen(2), "expected one ReportIssue event and one panic-recovery event")
		Expect(events[0].Tags["instance_uuid"]).To(Equal(instanceUUID))
		Expect(events[1].Tags["instance_uuid"]).To(Equal(instanceUUID))
	})

	It("leaves events untagged without SetInstanceUUID", func() {
		By("sending one error through ReportIssue")
		ReportIssue(errors.New("SetInstanceUUID no-tag test error"), IssueTypeError, log)

		By("sending one panic through RecoverAndReport")
		func() {
			defer RecoverAndReport()
			panic("SetInstanceUUID no-tag test panic")
		}()

		events := transport.Events()
		Expect(events).To(HaveLen(2), "expected one ReportIssue event and one panic-recovery event")
		Expect(events[0].Tags).NotTo(HaveKey("instance_uuid"))
		Expect(events[1].Tags).NotTo(HaveKey("instance_uuid"))
	})

	It("leaves the scope unchanged for an empty UUID", func() {
		SetInstanceUUID("")

		By("sending one error through ReportIssue on a fresh scope")
		ReportIssue(errors.New("SetInstanceUUID empty UUID test error"), IssueTypeError, log)

		events := transport.Events()
		Expect(events).To(HaveLen(1), "expected exactly one ReportIssue event")
		Expect(events[0].Tags).NotTo(HaveKey("instance_uuid"))
	})
})
