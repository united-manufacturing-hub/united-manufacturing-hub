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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// This runs the real transport worker tree against a mock Management Console and
// asks one question: which endpoint behaviours reproduce the log signature a
// customer instance produced on 2026-08-19, and which do not?
//
// The signature, taken from that instance's own log:
//
//	action_failed … error=context canceled during retry: context deadline
//	    exceeded … duration_ms=30000, timeout_ms=30000     (64 times)
//	push_reset_cleared … pending_dropped=459               (25 resets, 5,334 messages)
//	degrading: N pending messages (threshold=100)
//	pull_reset_cleared … pending_dropped=0                 (all 25)
//
// Reading a log signature backwards to a cause is only sound if one cause
// produces it. That is the thing to check, not assume, so each arm below injects
// a different fault and the assertions record which parts of the signature it
// produced.
//
// What this scenario covers and what it does not. It drives the fsmv2 transport
// tree, so it reproduces the push, pending and reset half of the signature. It
// does not run the legacy communicator, so the customer's
// "FSMv2 outbound channel full, dropping message for subscriber …" line is
// modelled here rather than produced: statusProducer writes once per second with
// a non-blocking send, which is what
// pkg/communicator/pkg/subscriber/subscribers.go does, and counts the refusals.

const (
	// The per-action budget in the supervisor is a hardcoded 30s
	// (supervisor/internal/execution/action_executor.go:defaultActionTimeout,
	// wired via supervisor.go with no Config field to override it), so an arm
	// that needs the budget to expire has to run longer than 30s in real time.
	// There is no clock abstraction in fsmv2 to compress it.
	//
	// EVERY arm runs for this long, including the ones expected to produce
	// nothing. A negative arm that ran for less would satisfy "budgetExpired
	// stays 0" simply by stopping before 30s had passed, which asserts nothing.
	armDuration = 65 * time.Second

	// When each arm switches from its failure phase to its recovery phase. Long
	// enough for several failing ticks to accumulate a pending list past the
	// degrade threshold of 100.
	recoveryAt = 18 * time.Second

	// Production's outbound channel capacity: cmd/main.go creates both channels
	// as make(chan *models.UMHMessage, 100).
	outboundCapacity = 100

	// How many messages each arm starts with on the channel: the channel's full
	// capacity, deliberately.
	//
	// This was 90, chosen so the healthy arm would not record a startup drop. That
	// was a mistake with a large consequence: at 90 every arm ended at 91/100 and
	// refused nothing, so a file named push_saturation never saturated. It also
	// changed which arms match, because state_running.go tests
	// ConsecutiveErrors >= 3 BEFORE pending >= 100 — at 90 a never-succeeding
	// console reaches 3 errors long before pending reaches 100, gets stuck in
	// Degraded, and can never log another Running -> Degraded reason.
	//
	// The healthy arm's startup drop is real and is now expected rather than
	// engineered away.
	preloadCount = outboundCapacity

	// The producer's rate, matching the subscriber's one status message per
	// second per subscriber.
	statusInterval = time.Second

	pushPath = "/v2/instance/push"
	pullPath = "/v2/instance/pull"

	// The budget-expiry error. This must be the DEADLINE text, not the
	// "context canceled during retry" wrapper around it: retryPending wraps
	// whatever ctx.Err() happens to be, so the wrapper also matches an ordinary
	// shutdown. An earlier version of this file matched the wrapper and counted a
	// teardown (duration_ms 1899, "context canceled") as a budget expiry.
	budgetExpiredMarker = "context deadline exceeded"
)

// phase applies a console behaviour at a point during the run, so one arm can
// model a failure burst followed by degraded-but-working service.
type phase struct {
	at    time.Duration
	apply func(*testutil.MockRelayServer)
}

// lockedBuffer lets the supervisor's goroutines share one log sink.
// bytes.Buffer is not safe for concurrent writers.
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

// statusProducer models the subscriber: one status message per second, offered to
// the outbound channel without blocking, dropped when there is no room.
type statusProducer struct {
	provider *examples.TransportTestChannelProvider
	stop     chan struct{}
	done     chan struct{}
	mu       sync.Mutex
	accepted int
	dropped  int
	maxLen   int
}

func newStatusProducer(p *examples.TransportTestChannelProvider) *statusProducer {
	return &statusProducer{
		provider: p,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (s *statusProducer) run(interval time.Duration) {
	go func() {
		defer close(s.done)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for i := 0; ; i++ {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
			}

			msg := &types.UMHMessage{
				Content: fmt.Sprintf("status-snapshot-%d", i),
				Email:   "operator@example.com",
			}

			ok := s.provider.TryQueueOutbound(msg)

			s.mu.Lock()
			if ok {
				s.accepted++
			} else {
				s.dropped++
			}

			if l := s.provider.OutboundLen(); l > s.maxLen {
				s.maxLen = l
			}
			s.mu.Unlock()
		}
	}()
}

func (s *statusProducer) halt() (accepted, dropped, maxLen int) {
	close(s.stop)
	<-s.done

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.accepted, s.dropped, s.maxLen
}

// signature is what one arm produced, phrased as the parts of the customer's
// signature rather than as raw counts, so arms can be compared directly.
type signature struct {
	budgetExpired    int // action_failed carrying budgetExpiredMarker
	otherPushFailed  int // action_failed on push from any other cause
	resets           int // push_reset_cleared
	pendingDropped   int // sum of pending_dropped across resets
	degradedOnDepth  int // "degrading: N pending messages"
	degradedOnErrors int // "degrading: N consecutive errors"
	poisonDropped    int // dropping_poison_message
	pullDropped      int // sum of pending_dropped across pull_reset_cleared
	pushesHandled    int // requests the mock console actually accepted
	pushRequests     int // requests that reached the mock console at all
	statusAccepted   int
	statusDropped    int
	maxOutboundLen   int
	outboundCap      int
}

func (s signature) String() string {
	return fmt.Sprintf(
		"budgetExpired=%d otherPushFailed=%d poisonDropped=%d "+
			"degradedOnDepth=%d degradedOnErrors=%d resets=%d pendingDropped=%d "+
			"pullDropped=%d pushRequests=%d pushesHandled=%d "+
			"status(accepted=%d dropped=%d) outbound=%d/%d",
		s.budgetExpired, s.otherPushFailed, s.poisonDropped,
		s.degradedOnDepth, s.degradedOnErrors, s.resets, s.pendingDropped,
		s.pullDropped, s.pushRequests, s.pushesHandled,
		s.statusAccepted, s.statusDropped, s.maxOutboundLen, s.outboundCap)
}

// scanLog counts the signature markers in the JSON log the run emitted.
// It reads the "msg" field rather than substring-matching whole lines, so a
// message that merely mentions another marker cannot inflate a count.
func scanLog(raw string) signature {
	var sig signature

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil {
			continue
		}

		msg, _ := m["msg"].(string)

		switch msg {
		case "action_failed":
			errText, _ := m["error"].(string)
			name, _ := m["action_name"].(string)

			if name != "push" {
				continue
			}

			if strings.Contains(errText, budgetExpiredMarker) {
				sig.budgetExpired++
			} else {
				sig.otherPushFailed++
			}
		case "push_reset_cleared":
			sig.resets++

			if v, ok := m["pending_dropped"].(float64); ok {
				sig.pendingDropped += int(v)
			}
		case "pull_reset_cleared":
			if v, ok := m["pending_dropped"].(float64); ok {
				sig.pullDropped += int(v)
			}
		case "dropping_poison_message":
			sig.poisonDropped++
		case "state_transition":
			// The pull child emits the same depth and error strings as the push
			// child, so this must filter by worker. Without it, stalled-pull
			// reports a degrade whose push side is identical to healthy.
			worker, _ := m["worker"].(string)
			if !strings.Contains(worker, "push-") {
				continue
			}

			reason, _ := m["reason"].(string)

			switch {
			case strings.Contains(reason, "pending messages"):
				sig.degradedOnDepth++
			case strings.Contains(reason, "consecutive errors"):
				sig.degradedOnErrors++
			}
		}
	}

	return sig
}

// runArm drives the transport tree for armDuration, applying each phase at its
// offset, and returns what the run produced.
//
// Every arm is preloaded to the channel's capacity. This is what makes the
// one-message-per-request path reachable at all: retryPending only runs when the
// pending list is non-empty, and the pending list is only populated by a push
// that already failed. Without a failure phase the push child stays in Phase 2,
// which batches the whole channel into a single request -- so a console that is
// merely slow never exercises the retry loop.
func runArm(httpTimeout time.Duration, phases []phase) signature {
	sink := &lockedBuffer{}
	logger := deps.NewJSONFSMLogger(sink, deps.LevelDebug)

	mockServer := testutil.NewMockRelayServer()
	defer mockServer.Close()

	provider := examples.NewTransportTestChannelProvider(outboundCapacity)
	producer := newStatusProducer(provider)

	// Fill the channel so the first push is a full batch. Capacity, not more:
	// QueueOutbound blocks on a full channel.
	preload := make([]*types.UMHMessage, 0, preloadCount)
	for i := range preloadCount {
		preload = append(preload, &types.UMHMessage{
			Content: fmt.Sprintf("preloaded-status-%d", i),
			Email:   "operator@example.com",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), armDuration+60*time.Second)
	defer cancel()

	result := examples.RunTransportScenario(ctx, examples.TransportRunConfig{
		Logger:                  logger,
		MockServer:              mockServer,
		ChannelProvider:         provider,
		HTTPTimeout:             httpTimeout,
		Duration:                armDuration,
		InitialOutboundMessages: preload,
	})
	Expect(result.Error).NotTo(HaveOccurred())

	producer.run(statusInterval)

	for _, ph := range phases {
		ph := ph

		time.AfterFunc(ph.at, func() { ph.apply(mockServer) })
	}

	Eventually(result.Done, armDuration+50*time.Second).Should(BeClosed())

	accepted, dropped, maxLen := producer.halt()

	sig := scanLog(sink.String())
	sig.pushesHandled = len(mockServer.GetPushedMessages())
	sig.pushRequests = mockServer.PushCallCount()
	sig.statusAccepted = accepted
	sig.statusDropped = dropped
	sig.maxOutboundLen = maxLen
	sig.outboundCap = provider.OutboundCap()

	return sig
}

var _ = Describe("How the push child behaves under five console behaviours", Serial, func() {
	// RETRACTED CONCLUSION, kept as a warning rather than deleted.
	//
	// This file was written to identify a cause: it asserted that only a
	// slowly-succeeding console produces a push-child queue-depth degrade, and
	// therefore that the customer's log identified that console behaviour. That
	// conclusion does not hold, for two independent reasons.
	//
	// First, the customer-log comparison behind it was invalid. All the
	// depth-degrade lines belong to the push child and all the error-degrade lines
	// belong to the PULL child, which logged separately. The push child logged no
	// error-degrades at all, and state_degraded.go explains why: it leaves Degraded
	// only at ConsecutiveErrors == 0, and the retry tracker has no decay, so once
	// the push child errors it is stuck and can never log another
	// Running -> Degraded reason. Its silence is structural, not informative.
	//
	// Second, the discrimination this file measured turned out to be a function of
	// preloadCount rather than of the console. See the comment on that constant.
	//
	// So the arms below record what each console behaviour does. They no longer
	// claim any of it identifies a cause. For the load result that does hold, see
	// push_dose_response_test.go in this package.

	fail502 := func(m *testutil.MockRelayServer) {
		m.SetPathFault(pushPath, testutil.PathFault{StatusCode: 502})
	}

	It("a console that always answers delivers everything", func() {
		sig := runArm(20*time.Second, nil)

		GinkgoWriter.Printf("ARM healthy: %s\n", sig)

		Expect(sig.pushesHandled).To(BeNumerically(">=", preloadCount),
			"the whole preload must get through, or the arm is not exercising the push path")
		Expect(sig.budgetExpired).To(Equal(0),
			"nothing should run out of budget against a console that answers at once")
	})

	It("a failure burst followed by a slow console runs out of budget", func() {
		sig := runArm(20*time.Second, []phase{
			{at: 0, apply: fail502},
			{at: recoveryAt, apply: func(m *testutil.MockRelayServer) {
				m.SetPathFault(pushPath, testutil.PathFault{Delay: 4 * time.Second})
			}},
		})

		GinkgoWriter.Printf("ARM burst-then-slow: %s\n", sig)

		Expect(sig.pushesHandled).To(BeNumerically(">", 0),
			"the recovery phase must actually deliver, or this is not a slow-success arm")
		Expect(sig.budgetExpired).To(BeNumerically(">", 0),
			"one message per request against a 4s console must exhaust the 30s budget")
	})

	It("a console that never answers accepts nothing", func() {
		sig := runArm(20*time.Second, []phase{
			{at: 0, apply: fail502},
			{at: recoveryAt, apply: func(m *testutil.MockRelayServer) {
				m.SetPathFault(pushPath, testutil.PathFault{Hang: true})
			}},
		})

		GinkgoWriter.Printf("ARM burst-then-hang: %s\n", sig)

		Expect(sig.pushRequests).To(BeNumerically(">", 0),
			"the arm must reach the push endpoint")
		Expect(sig.pushesHandled).To(Equal(0),
			"a hanging console must accept nothing, or the fault did not apply")
	})

	It("a console returning 502 throughout accepts nothing", func() {
		sig := runArm(20*time.Second, []phase{{at: 0, apply: fail502}})

		GinkgoWriter.Printf("ARM 502-throughout: %s\n", sig)

		Expect(sig.pushRequests).To(BeNumerically(">", 0),
			"the arm must reach the push endpoint")
		Expect(sig.pushesHandled).To(Equal(0),
			"a 502 must accept nothing, or the fault did not apply")
	})

	It("a stalled pull leaves the push side working", func() {
		sig := runArm(20*time.Second, []phase{
			{at: 0, apply: func(m *testutil.MockRelayServer) {
				m.SetPathFault(pullPath, testutil.PathFault{Hang: true})
			}},
		})

		GinkgoWriter.Printf("ARM stalled-pull: %s\n", sig)

		Expect(sig.pushesHandled).To(BeNumerically(">", 0),
			"push must keep working while pull is stalled")
		Expect(sig.budgetExpired).To(Equal(0),
			"a stalled pull must not spend the push child's budget")
	})
})
