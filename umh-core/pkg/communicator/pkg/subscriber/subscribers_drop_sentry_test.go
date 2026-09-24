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

package subscriber_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/subscriber"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
	"go.uber.org/zap"
)

type recordedSentryWarn struct {
	feature       deps.Feature
	hierarchyPath string
	message       string
	fields        []deps.Field
}

// recordingFSMLogger implements deps.FSMLogger and records every SentryWarn call.
type recordingFSMLogger struct {
	mu    sync.Mutex
	warns []recordedSentryWarn
}

func (l *recordingFSMLogger) Debug(_ string, _ ...deps.Field) {}

func (l *recordingFSMLogger) Info(_ string, _ ...deps.Field) {}

func (l *recordingFSMLogger) SentryError(_ deps.Feature, _ string, _ error, _ string, _ ...deps.Field) {
}

func (l *recordingFSMLogger) With(_ ...deps.Field) deps.FSMLogger { return l }

func (l *recordingFSMLogger) SentryWarn(feature deps.Feature, hierarchyPath string, msg string, fields ...deps.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.warns = append(l.warns, recordedSentryWarn{
		feature:       feature,
		hierarchyPath: hierarchyPath,
		message:       msg,
		fields:        fields,
	})
}

func (l *recordingFSMLogger) warnCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return len(l.warns)
}

func (l *recordingFSMLogger) snapshot() []recordedSentryWarn {
	l.mu.Lock()
	defer l.mu.Unlock()

	return append([]recordedSentryWarn{}, l.warns...)
}

// hasFSMv2OutboundDropWarn reports whether any recorded call matches the
// behavioral contract for the FSMv2 drop site.
func (l *recordingFSMLogger) hasFSMv2OutboundDropWarn() bool {
	_, ok := l.fsmv2OutboundDropWarn()

	return ok
}

func (l *recordingFSMLogger) fsmv2OutboundDropWarn() (recordedSentryWarn, bool) {
	return l.outboundDropWarn("fsmv2_outbound_channel_full")
}

// hasGatekeeperOutboundDropWarn reports whether any recorded call matches the
// behavioral contract for the gatekeeper drop site.
func (l *recordingFSMLogger) hasGatekeeperOutboundDropWarn() bool {
	_, ok := l.gatekeeperOutboundDropWarn()

	return ok
}

func (l *recordingFSMLogger) gatekeeperOutboundDropWarn() (recordedSentryWarn, bool) {
	return l.outboundDropWarn("gatekeeper_outbound_channel_full")
}

// outboundDropWarn returns the first recorded call that carries exactly the
// feature, hierarchy path, the given message, and channel_len/channel_cap
// fields of 1 an outbound drop site must send.
func (l *recordingFSMLogger) outboundDropWarn(message string) (recordedSentryWarn, bool) {
	for _, w := range l.snapshot() {
		if w.feature != deps.FeatureFSMv1Communicator {
			continue
		}

		if w.hierarchyPath != "fsmv1.Communicator" {
			continue
		}

		if w.message != message {
			continue
		}

		fieldValues := make(map[string]any, len(w.fields))
		for _, f := range w.fields {
			fieldValues[f.Key] = f.Value
		}

		channelLen, lenIsInt := fieldValues["channel_len"].(int)
		channelCap, capIsInt := fieldValues["channel_cap"].(int)
		if len(fieldValues) == 2 && lenIsInt && capIsInt && channelLen == 1 && channelCap == 1 {
			return w, true
		}
	}

	return recordedSentryWarn{}, false
}

var _ = Describe("FSMv2 outbound channel drop Sentry warning", func() {
	It("sends fsmv2_outbound_channel_full through the injected FSMLogger when the channel is full, and delivers without warning while it has room", func() {
		zapLogger, err := zap.NewDevelopment()
		Expect(err).NotTo(HaveOccurred())
		logger := zapLogger.Sugar()

		snapshotManager := fsm.NewSnapshotManager()
		snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{
			SnapshotTime: time.Now(),
			Managers: map[string]fsm.ManagerSnapshot{
				"DropSentryManager": &fsm.BaseManagerSnapshot{Name: "DropSentryManager", SnapshotTime: time.Now()},
			},
		})

		fsmLogger := &recordingFSMLogger{}
		ch := make(chan *types.UMHMessage, 1)

		handler := subscriber.NewHandler(
			&mockWatchdog{},
			nil,
			uuid.New(),
			time.Minute,
			time.Minute,
			config.ReleaseChannelStable,
			false,
			snapshotManager,
			config.NewMockConfigManager(),
			logger,
			topicbrowser.NewTopicBrowserCommunicatorWithSimulator(logger),
			ch,
			nil,
			nil,
			fsmLogger,
		)

		const email = "drop-test@example.com"
		handler.AddOrRefreshSubscriber(email, true)

		var received int32
		stopConsumer := make(chan struct{})
		var consumerWG sync.WaitGroup
		consumerWG.Add(1)
		go func() {
			defer consumerWG.Done()
			for {
				select {
				case <-ch:
					atomic.AddInt32(&received, 1)
				case <-stopConsumer:
					return
				}
			}
		}()

		handler.StartNotifier()

		// Positive control: with room in the channel the notify path must
		// deliver a status message, so the drop assertions below cannot pass
		// or fail vacuously on a notify path that never sends.
		Eventually(func() int32 { return atomic.LoadInt32(&received) }, 5*time.Second, 100*time.Millisecond).
			Should(BeNumerically(">=", 1), "the notify path should deliver a status message while the channel has room")
		Consistently(func() int { return fsmLogger.warnCount() }, 2*time.Second, 200*time.Millisecond).
			Should(Equal(0), "a successful send must not SentryWarn")

		// Drop: stop draining, fill the capacity-1 channel so the next notify
		// tick finds it full. A notify tick may have delivered the message
		// itself; the channel is full either way.
		close(stopConsumer)
		consumerWG.Wait()

		select {
		case ch <- &types.UMHMessage{InstanceUUID: "drop-sentry-prefill"}:
		default:
		}

		Eventually(func() bool { return fsmLogger.hasFSMv2OutboundDropWarn() }, 5*time.Second, 100*time.Millisecond).
			Should(BeTrue(), "a full fsmOutboundChannel must SentryWarn fsmv2_outbound_channel_full with channel_len=1 and channel_cap=1")

		call, ok := fsmLogger.fsmv2OutboundDropWarn()
		Expect(ok).To(BeTrue())
		Expect(call.feature).To(Equal(deps.FeatureFSMv1Communicator))
		Expect(call.hierarchyPath).To(Equal("fsmv1.Communicator"))
		Expect(call.message).To(Equal("fsmv2_outbound_channel_full"))

		fieldValues := make(map[string]any, len(call.fields))
		for _, f := range call.fields {
			fieldValues[f.Key] = f.Value
		}

		Expect(fieldValues).To(HaveLen(2), "the drop warning must carry exactly channel_len and channel_cap")
		Expect(fieldValues["channel_len"]).To(BeAssignableToTypeOf(0))
		Expect(fieldValues["channel_len"]).To(Equal(1))
		Expect(fieldValues["channel_cap"]).To(BeAssignableToTypeOf(0))
		Expect(fieldValues["channel_cap"]).To(Equal(1))

		for _, w := range fsmLogger.snapshot() {
			for _, f := range w.fields {
				Expect(fmt.Sprintf("%v", f.Value)).NotTo(ContainSubstring(email),
					"no Sentry field value may contain the subscriber email")
			}
		}
	})
})

var _ = Describe("Gatekeeper outbound channel drop Sentry warning", func() {
	It("sends gatekeeper_outbound_channel_full through the injected FSMLogger when the channel is full, and delivers without warning while it has room", func() {
		zapLogger, err := zap.NewDevelopment()
		Expect(err).NotTo(HaveOccurred())
		logger := zapLogger.Sugar()

		snapshotManager := fsm.NewSnapshotManager()
		snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{
			SnapshotTime: time.Now(),
			Managers: map[string]fsm.ManagerSnapshot{
				"GatekeeperDropSentryManager": &fsm.BaseManagerSnapshot{Name: "GatekeeperDropSentryManager", SnapshotTime: time.Now()},
			},
		})

		fsmLogger := &recordingFSMLogger{}
		ch := make(chan *types.MessageWithSender, 1)

		handler := subscriber.NewHandler(
			&mockWatchdog{},
			nil,
			uuid.New(),
			time.Minute,
			time.Minute,
			config.ReleaseChannelStable,
			false,
			snapshotManager,
			config.NewMockConfigManager(),
			logger,
			topicbrowser.NewTopicBrowserCommunicatorWithSimulator(logger),
			nil,
			ch,
			nil,
			fsmLogger,
		)

		const email = "gatekeeper-drop-test@example.com"
		handler.AddOrRefreshSubscriber(email, true)

		var received int32
		stopConsumer := make(chan struct{})
		var consumerWG sync.WaitGroup
		consumerWG.Add(1)
		go func() {
			defer consumerWG.Done()
			for {
				select {
				case <-ch:
					atomic.AddInt32(&received, 1)
				case <-stopConsumer:
					return
				}
			}
		}()

		handler.StartNotifier()

		// Positive control: with room in the channel the notify path must
		// deliver a status message, so the drop assertions below cannot pass
		// or fail vacuously on a notify path that never sends.
		Eventually(func() int32 { return atomic.LoadInt32(&received) }, 5*time.Second, 100*time.Millisecond).
			Should(BeNumerically(">=", 1), "the notify path should deliver a status message while the gatekeeper channel has room")
		Consistently(func() int { return fsmLogger.warnCount() }, 2*time.Second, 200*time.Millisecond).
			Should(Equal(0), "a successful send must not SentryWarn")

		// Drop: stop draining, fill the capacity-1 channel so the next notify
		// tick finds it full. A notify tick may have delivered the message
		// itself; the channel is full either way.
		close(stopConsumer)
		consumerWG.Wait()

		select {
		case ch <- &types.MessageWithSender{SenderEmail: "gatekeeper-drop-prefill"}:
		default:
		}

		Eventually(func() bool { return fsmLogger.hasGatekeeperOutboundDropWarn() }, 5*time.Second, 100*time.Millisecond).
			Should(BeTrue(), "a full gatekeeperOutboundChannel must SentryWarn gatekeeper_outbound_channel_full with channel_len=1 and channel_cap=1")

		call, ok := fsmLogger.gatekeeperOutboundDropWarn()
		Expect(ok).To(BeTrue())
		Expect(call.feature).To(Equal(deps.FeatureFSMv1Communicator))
		Expect(call.hierarchyPath).To(Equal("fsmv1.Communicator"))
		Expect(call.message).To(Equal("gatekeeper_outbound_channel_full"))

		fieldValues := make(map[string]any, len(call.fields))
		for _, f := range call.fields {
			fieldValues[f.Key] = f.Value
		}

		Expect(fieldValues).To(HaveLen(2), "the drop warning must carry exactly channel_len and channel_cap")
		Expect(fieldValues["channel_len"]).To(BeAssignableToTypeOf(0))
		Expect(fieldValues["channel_len"]).To(Equal(1))
		Expect(fieldValues["channel_cap"]).To(BeAssignableToTypeOf(0))
		Expect(fieldValues["channel_cap"]).To(Equal(1))

		for _, w := range fsmLogger.snapshot() {
			for _, f := range w.fields {
				Expect(fmt.Sprintf("%v", f.Value)).NotTo(ContainSubstring(email),
					"no Sentry field value may contain the subscriber email")
			}
		}
	})
})
