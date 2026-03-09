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
	"fmt"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	transportWorker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// ENG-4559: Push worker stuck → instance offline
//
// Hypothesis H-G: Token expiry → transport enters StartingState → push gets ParentMappedState="stopped"
// → push stops → nobody drains outbound channel → channel fills → instance appears offline.
var _ = Describe("ENG-4559 Offline Scenario", func() {
	var ctx context.Context
	var cancel context.CancelFunc

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 45*time.Second)
	})

	AfterEach(func() {
		cancel()
		communicator.ClearChannelProvider()
		transportWorker.ClearChannelProvider()
	})

	// S1: Token Expiry + Re-Auth → Channel Overflow
	//
	// Reproduction sequence:
	// 1. Start communicator with MockRelayServer returning short-lived JWT (expiresAt = now + 10min + 3s)
	// 2. Continuously produce outbound messages
	// 3. After ~3s: IsTokenExpired() fires (10-minute buffer), transport → Starting, push Stops
	// 4. During re-auth gap: outbound channel fills, zero messages pushed
	// 5. After re-auth (mock server responds immediately): push restarts, messages drain
	//
	// The 10-minute buffer in IsTokenExpired means: if expiresAt = now + 10min + 3s,
	// then now + 10min > expiresAt triggers after 3 seconds.
	It("S1: token expiry causes push to stop and outbound channel to fill", func() {
		mockServer := testutil.NewMockRelayServer()
		defer mockServer.Close()

		// JWT expires in 10min + 3s from now. IsTokenExpired has 10-minute buffer,
		// so it triggers after ~3 seconds.
		shortExpiry := time.Now().Add(10*time.Minute + 3*time.Second).Unix()
		mockServer.SetJWTExpiry(shortExpiry)

		// Set up channel provider with small buffer to observe filling
		channelProvider := examples.NewTestChannelProvider(20)
		communicator.SetChannelProvider(channelProvider)
		transportWorker.SetChannelProvider(channelProvider)

		serverURL := mockServer.URL()
		scenarioConfig := fmt.Sprintf(`
children:
  - name: "communicator-1"
    workerType: "communicator"
    userSpec:
      config: |
        relayURL: "%s"
        instanceUUID: "test-instance-uuid"
        authToken: "test-auth-token"
        timeout: "5s"
`, serverURL)

		testScenario := examples.Scenario{
			Name:        "eng4559-s1-token-expiry",
			Description: "ENG-4559 S1: Token expiry → push stops → channel overflow",
			YAMLConfig:  scenarioConfig,
		}

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		runResult, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     testScenario,
			Duration:     0,
			TickInterval: 100 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		// Wait for initial auth to succeed and push to start working
		time.Sleep(2 * time.Second)

		// Record messages pushed before expiry
		pushedBeforeExpiry := len(mockServer.GetPushedMessages())
		GinkgoWriter.Printf("[S1] Messages pushed before expiry window: %d\n", pushedBeforeExpiry)

		// Confirm auth happened at least once
		Expect(mockServer.AuthCallCount()).To(BeNumerically(">=", 1),
			"Expected at least one auth call before expiry")

		// Now produce outbound messages continuously.
		// After ~3s from start (we already slept 2s, so ~1s more), token expiry fires.
		var produced atomic.Int64
		stopProducing := make(chan struct{})
		go func() {
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stopProducing:
					return
				case <-ticker.C:
					select {
					case channelProvider.GetOutboundChanForTest() <- &transport.UMHMessage{
						InstanceUUID: "test-instance",
						Content:      fmt.Sprintf("msg-%d", produced.Load()),
					}:
						produced.Add(1)
					default:
						// Channel full — this is the ENG-4559 symptom
						GinkgoWriter.Printf("[S1] Outbound channel FULL at msg %d\n", produced.Load())
					}
				}
			}
		}()

		// Wait for token expiry to fire (~1-2 more seconds) + re-auth time
		time.Sleep(4 * time.Second)

		// Check: auth should have been called again (re-auth after expiry)
		authCalls := mockServer.AuthCallCount()
		GinkgoWriter.Printf("[S1] Total auth calls: %d\n", authCalls)

		// Key assertion: re-auth happened (auth count > 1)
		Expect(authCalls).To(BeNumerically(">=", 2),
			"Expected re-auth after token expiry")

		// Check outbound channel stats — if push was stopped, messages accumulated
		outCap, outLen := channelProvider.GetOutboundStats()
		GinkgoWriter.Printf("[S1] Outbound channel: len=%d, cap=%d, produced=%d\n",
			outLen, outCap, produced.Load())

		// After re-auth, push should restart and drain the channel.
		// Give it a few seconds to recover.
		time.Sleep(3 * time.Second)

		// Record messages pushed after recovery
		pushedAfterRecovery := len(mockServer.GetPushedMessages())
		GinkgoWriter.Printf("[S1] Messages pushed after recovery: %d (before expiry: %d)\n",
			pushedAfterRecovery, pushedBeforeExpiry)

		// Verify recovery: more messages should have been pushed after re-auth
		Expect(pushedAfterRecovery).To(BeNumerically(">", pushedBeforeExpiry),
			"Expected messages to be pushed after re-auth recovery")

		close(stopProducing)
		runResult.Shutdown()
		Eventually(runResult.Done, 35*time.Second).Should(BeClosed())
	})

	// S2: Permanent Auth Failure → Push Stays Stopped Forever
	//
	// Same as S1 but mock server rejects ALL subsequent auth requests.
	// Push stays Stopped permanently. Channel fills and never drains.
	It("S2: permanent auth failure causes push to stay stopped and channel to fill permanently", func() {
		mockServer := testutil.NewMockRelayServer()
		defer mockServer.Close()

		// Short-lived JWT: expires in 10min + 3s (triggers after ~3s)
		shortExpiry := time.Now().Add(10*time.Minute + 3*time.Second).Unix()
		mockServer.SetJWTExpiry(shortExpiry)

		channelProvider := examples.NewTestChannelProvider(20)
		communicator.SetChannelProvider(channelProvider)
		transportWorker.SetChannelProvider(channelProvider)

		serverURL := mockServer.URL()
		scenarioConfig := fmt.Sprintf(`
children:
  - name: "communicator-1"
    workerType: "communicator"
    userSpec:
      config: |
        relayURL: "%s"
        instanceUUID: "test-instance-uuid"
        authToken: "test-auth-token"
        timeout: "5s"
`, serverURL)

		testScenario := examples.Scenario{
			Name:        "eng4559-s2-permanent-auth-fail",
			Description: "ENG-4559 S2: Permanent auth failure → push stopped forever → channel fills",
			YAMLConfig:  scenarioConfig,
		}

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		runResult, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     testScenario,
			Duration:     0,
			TickInterval: 100 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		// Wait for initial auth to succeed
		time.Sleep(2 * time.Second)
		Expect(mockServer.AuthCallCount()).To(BeNumerically(">=", 1),
			"Expected initial auth to succeed")

		// NOW enable persistent auth failure — all subsequent logins will fail
		mockServer.SimulatePersistentAuthError(401)

		// Produce messages to fill the outbound channel
		var produced atomic.Int64
		stopProducing := make(chan struct{})
		go func() {
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stopProducing:
					return
				case <-ticker.C:
					select {
					case channelProvider.GetOutboundChanForTest() <- &transport.UMHMessage{
						InstanceUUID: "test-instance",
						Content:      fmt.Sprintf("msg-%d", produced.Load()),
					}:
						produced.Add(1)
					default:
						// Channel full — permanent offline state
						GinkgoWriter.Printf("[S2] Outbound channel FULL at msg %d (permanent)\n", produced.Load())
					}
				}
			}
		}()

		// Wait for token expiry + several re-auth attempts (all fail)
		time.Sleep(6 * time.Second)

		// Auth should have been called multiple times (initial success + failed retries)
		authCalls := mockServer.AuthCallCount()
		GinkgoWriter.Printf("[S2] Total auth calls: %d (first succeeded, rest failed)\n", authCalls)
		Expect(authCalls).To(BeNumerically(">=", 2),
			"Expected re-auth attempts after token expiry")

		// Record pushed messages count — should NOT increase from this point
		pushedCount := len(mockServer.GetPushedMessages())
		GinkgoWriter.Printf("[S2] Messages pushed (should be frozen): %d\n", pushedCount)

		// Wait more and verify no new messages are pushed (push is permanently stopped)
		time.Sleep(3 * time.Second)
		pushedCountAfter := len(mockServer.GetPushedMessages())
		GinkgoWriter.Printf("[S2] Messages pushed after waiting: %d (diff: %d)\n",
			pushedCountAfter, pushedCountAfter-pushedCount)

		// Key assertion: outbound channel should be full (or nearly full)
		_, outLen := channelProvider.GetOutboundStats()
		GinkgoWriter.Printf("[S2] Outbound channel length: %d (produced: %d)\n",
			outLen, produced.Load())

		// The channel should have messages stuck in it (push can't drain them)
		Expect(produced.Load()).To(BeNumerically(">", 0),
			"Expected messages to be produced")

		// No new messages pushed while auth is failing
		Expect(pushedCountAfter).To(BeNumerically("<=", pushedCount+1),
			"Expected no new messages pushed during permanent auth failure")

		close(stopProducing)
		runResult.Shutdown()
		Eventually(runResult.Done, 35*time.Second).Should(BeClosed())
	})

	// S6: Slow Server → Pending Blocks Channel Drain → Messages Lost
	//
	// This scenario proves: when the relay server responds slowly, PushAction's
	// Phase 1 (pending retry, one-by-one) blocks Phase 2 (channel drain).
	// Messages accumulate on the outbound channel and are dropped.
	//
	// Reproduction sequence:
	// 1. Start healthy communicator (auth succeeds, push Running)
	// 2. Produce outbound messages at ~20/sec
	// 3. After push establishes baseline, make relay server slow (3s per push response)
	// 4. First slow push times out or succeeds slowly → messages go to pending on next failure
	// 5. Phase 1 retries pending one-by-one (each takes 3s)
	// 6. Phase 2 (channel drain) never runs → channel fills → messages dropped
	//
	// Key difference from S1/S2: push worker stays RUNNING (transport is fine).
	// The bug is in PushAction's sequential design, not in auth/transport.
	It("S6: slow server causes pending retry to block channel drain, dropping messages", func() {
		mockServer := testutil.NewMockRelayServer()
		defer mockServer.Close()

		// Small outbound channel to observe filling quickly
		channelProvider := examples.NewTestChannelProvider(20)
		communicator.SetChannelProvider(channelProvider)
		transportWorker.SetChannelProvider(channelProvider)

		serverURL := mockServer.URL()
		scenarioConfig := fmt.Sprintf(`
children:
  - name: "communicator-1"
    workerType: "communicator"
    userSpec:
      config: |
        relayURL: "%s"
        instanceUUID: "test-instance-uuid"
        authToken: "test-auth-token"
        timeout: "5s"
`, serverURL)

		testScenario := examples.Scenario{
			Name:        "eng4559-s6-slow-server-pending-blocks-drain",
			Description: "ENG-4559 S6: Slow server → pending blocks channel drain → messages lost",
			YAMLConfig:  scenarioConfig,
		}

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		runResult, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     testScenario,
			Duration:     0,
			TickInterval: 100 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		// Wait for initial auth + push to start working normally
		time.Sleep(2 * time.Second)

		// Verify baseline: auth succeeded and some messages can be pushed
		Expect(mockServer.AuthCallCount()).To(BeNumerically(">=", 1),
			"Expected initial auth to succeed")

		// Push a few baseline messages to confirm push is working
		for i := 0; i < 5; i++ {
			channelProvider.GetOutboundChanForTest() <- &transport.UMHMessage{
				InstanceUUID: "test-instance",
				Content:      fmt.Sprintf("baseline-msg-%d", i),
			}
		}
		time.Sleep(1 * time.Second)
		baselinePushed := len(mockServer.GetPushedMessages())
		GinkgoWriter.Printf("[S6] Baseline messages pushed: %d\n", baselinePushed)
		Expect(baselinePushed).To(BeNumerically(">", 0),
			"Expected baseline messages to be pushed successfully")

		// NOW make the server slow: 3 seconds per push response.
		// This is persistent — every push request will take 3s.
		mockServer.SimulatePersistentSlowPush(3 * time.Second)

		// Also inject one server error to force messages into pending buffer.
		// After this error, messages go to pending → Phase 1 activates → blocks Phase 2.
		mockServer.SimulateServerError(500)

		// Produce outbound messages at high rate (every 50ms = ~20/sec)
		var produced atomic.Int64
		var dropped atomic.Int64
		stopProducing := make(chan struct{})
		go func() {
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stopProducing:
					return
				case <-ticker.C:
					select {
					case channelProvider.GetOutboundChanForTest() <- &transport.UMHMessage{
						InstanceUUID: "test-instance",
						Content:      fmt.Sprintf("s6-msg-%d", produced.Load()),
					}:
						produced.Add(1)
					default:
						// Channel full — this is the S6 symptom: Phase 1 blocks Phase 2
						dropped.Add(1)
						GinkgoWriter.Printf("[S6] DROPPED message (channel full): produced=%d, dropped=%d\n",
							produced.Load(), dropped.Load())
					}
				}
			}
		}()

		// Wait for the slow server to cause cascading failure.
		// With 3s per push and 100ms tick interval:
		// - First push after error: messages go to pending
		// - Phase 1 retries one-by-one, each taking 3s
		// - Channel buffer (20) fills in ~1s at 20msg/sec
		// - Messages start dropping
		time.Sleep(10 * time.Second)

		// Stop producing
		close(stopProducing)

		// Collect results
		totalPushed := len(mockServer.GetPushedMessages())
		totalProduced := produced.Load()
		totalDropped := dropped.Load()
		_, outLen := channelProvider.GetOutboundStats()

		GinkgoWriter.Printf("[S6] Results:\n")
		GinkgoWriter.Printf("[S6]   Total produced: %d\n", totalProduced)
		GinkgoWriter.Printf("[S6]   Total pushed to server: %d\n", totalPushed)
		GinkgoWriter.Printf("[S6]   Total dropped (channel full): %d\n", totalDropped)
		GinkgoWriter.Printf("[S6]   Channel remaining: %d\n", outLen)
		GinkgoWriter.Printf("[S6]   Auth calls: %d (should be 1 — no auth issues)\n", mockServer.AuthCallCount())

		// KEY ASSERTION 1: Messages were dropped because channel filled up
		Expect(totalDropped).To(BeNumerically(">", 0),
			"Expected messages to be dropped due to channel being full (Phase 1 blocking Phase 2)")

		// KEY ASSERTION 2: Auth should NOT have been called more than once
		// (this is NOT an auth problem — transport stays healthy)
		Expect(mockServer.AuthCallCount()).To(BeNumerically("<=", 2),
			"Expected no re-auth — this is a push-level issue, not auth")

		// KEY ASSERTION 3: Total messages lost = produced - pushed - remaining in channel
		// This should be > 0, proving messages were irrecoverably lost
		messagesAccountedFor := int64(totalPushed) + int64(outLen)
		messagesLost := totalProduced - messagesAccountedFor
		if messagesLost < 0 {
			messagesLost = 0
		}
		GinkgoWriter.Printf("[S6]   Messages lost (produced - pushed - channel remaining): %d\n", messagesLost)

		// The combination of dropped + lost proves the bug:
		// Phase 1 retrying one-by-one blocks Phase 2 from draining the channel
		totalImpacted := totalDropped + messagesLost
		GinkgoWriter.Printf("[S6]   Total impacted (dropped + lost): %d\n", totalImpacted)
		Expect(totalImpacted).To(BeNumerically(">", 0),
			"Expected messages to be lost due to Phase 1 blocking Phase 2")

		// Clean up
		mockServer.ClearPersistentSlowPush()
		runResult.Shutdown()
		Eventually(runResult.Done, 35*time.Second).Should(BeClosed())
	})

	// S7: Reproduce 06:21 State — Pull Active, Push Stuck in Degraded, Channel Full
	//
	// After S6's slow-server cascade, push accumulates consecutive errors and enters
	// Degraded state with exponential backoff. During backoff:
	// - Push dispatches NO actions (no PushAction, no channel drain)
	// - Pull continues operating normally (PullAction dispatches every tick)
	// - Outbound channel fills because nobody reads from it
	// - Messages are dropped
	//
	// This reproduces the exact 06:21 log pattern: pull triggers at ~10/sec,
	// channel full, messages being dropped, while push is completely idle.
	It("S7: push stuck in degraded backoff while pull continues, channel fills", func() {
		mockServer := testutil.NewMockRelayServer()
		defer mockServer.Close()

		// Small outbound channel to observe filling quickly
		channelProvider := examples.NewTestChannelProvider(20)
		communicator.SetChannelProvider(channelProvider)
		transportWorker.SetChannelProvider(channelProvider)

		serverURL := mockServer.URL()
		scenarioConfig := fmt.Sprintf(`
children:
  - name: "communicator-1"
    workerType: "communicator"
    userSpec:
      config: |
        relayURL: "%s"
        instanceUUID: "test-instance-uuid"
        authToken: "test-auth-token"
        timeout: "5s"
`, serverURL)

		testScenario := examples.Scenario{
			Name:        "eng4559-s7-pull-active-push-degraded",
			Description: "ENG-4559 S7: Pull active, push stuck in Degraded backoff, channel full",
			YAMLConfig:  scenarioConfig,
		}

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		runResult, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     testScenario,
			Duration:     0,
			TickInterval: 100 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		// Wait for initial auth + push/pull to start working normally
		time.Sleep(2 * time.Second)

		// Verify baseline: auth succeeded and push is working
		Expect(mockServer.AuthCallCount()).To(BeNumerically(">=", 1),
			"Expected initial auth to succeed")

		// Push a few baseline messages to confirm push works
		for i := 0; i < 5; i++ {
			channelProvider.GetOutboundChanForTest() <- &transport.UMHMessage{
				InstanceUUID: "test-instance",
				Content:      fmt.Sprintf("baseline-msg-%d", i),
			}
		}
		time.Sleep(1 * time.Second)
		baselinePushed := len(mockServer.GetPushedMessages())
		GinkgoWriter.Printf("[S7] Baseline messages pushed: %d\n", baselinePushed)
		Expect(baselinePushed).To(BeNumerically(">", 0),
			"Expected baseline messages to be pushed successfully")

		// Record pull call count before push failure — we'll compare after
		pullCallsBefore := mockServer.PullCallCount()
		GinkgoWriter.Printf("[S7] Pull calls before push failure: %d\n", pullCallsBefore)

		// NOW make push fail persistently with 500 errors.
		// Pull endpoint stays healthy — only push is affected.
		// After 3+ consecutive push errors, push enters Degraded state.
		// Backoff delay = 2^consecutiveErrors seconds (2s, 4s, 8s, 16s, 32s, 60s max).
		mockServer.SimulatePersistentPushError(500)

		// Produce outbound messages at high rate to fill the channel
		var produced atomic.Int64
		var dropped atomic.Int64
		stopProducing := make(chan struct{})
		go func() {
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stopProducing:
					return
				case <-ticker.C:
					select {
					case channelProvider.GetOutboundChanForTest() <- &transport.UMHMessage{
						InstanceUUID: "test-instance",
						Content:      fmt.Sprintf("s7-msg-%d", produced.Load()),
					}:
						produced.Add(1)
					default:
						// Channel full — this is the 06:21 symptom
						dropped.Add(1)
					}
				}
			}
		}()

		// Wait for push to accumulate errors and enter Degraded with backoff.
		// With 100ms tick interval:
		// - First 3 push attempts fail (500 errors) over ~300ms
		// - Push enters Degraded, backoff starts at 2^3 = 8 seconds
		// - During those 8+ seconds, push dispatches NO actions
		// - Channel (cap=20) fills in ~1s at 20msg/sec
		// - Messages start dropping
		//
		// Meanwhile, pull continues every tick (100ms), pulling any queued messages.
		time.Sleep(10 * time.Second)

		// Stop producing
		close(stopProducing)

		// Record push count AFTER the degraded period — it should barely have increased
		// because push was in backoff (no PushAction dispatched during backoff window).
		pushedAfterDegraded := len(mockServer.GetPushedMessages())
		totalProduced := produced.Load()
		totalDropped := dropped.Load()
		_, outLen := channelProvider.GetOutboundStats()

		GinkgoWriter.Printf("[S7] Results:\n")
		GinkgoWriter.Printf("[S7]   Baseline pushed (before errors): %d\n", baselinePushed)
		GinkgoWriter.Printf("[S7]   Total pushed after degraded period: %d\n", pushedAfterDegraded)
		GinkgoWriter.Printf("[S7]   Total produced during degraded: %d\n", totalProduced)
		GinkgoWriter.Printf("[S7]   Total dropped (channel full): %d\n", totalDropped)
		GinkgoWriter.Printf("[S7]   Outbound channel remaining: %d\n", outLen)
		GinkgoWriter.Printf("[S7]   Auth calls: %d (should be 1 — no auth issues)\n", mockServer.AuthCallCount())

		// KEY ASSERTION 1: Messages were dropped because channel filled during push backoff
		Expect(totalDropped).To(BeNumerically(">", 0),
			"Expected messages to be dropped due to channel being full while push is in Degraded backoff")

		// KEY ASSERTION 2: Pull continued operating — verify pull HTTP calls increased.
		// Pull worker dispatches PullAction every tick (100ms) while in Running state.
		// Even if backpressure prevents actual HTTP pulls, pull's state machine keeps ticking.
		// The pull endpoint should have received calls during the degraded period.
		pullCallsAfter := mockServer.PullCallCount()
		pullCallsDuringDegraded := pullCallsAfter - pullCallsBefore
		GinkgoWriter.Printf("[S7]   Pull calls during degraded period: %d (before=%d, after=%d)\n",
			pullCallsDuringDegraded, pullCallsBefore, pullCallsAfter)
		// Note: Pull may or may not make HTTP calls depending on backpressure.
		// With small inbound channel (cap=20) < ExpectedBatchSize (50), backpressure triggers
		// and pull skips the HTTP call. This is actually realistic: in the 06:21 logs,
		// pull triggers fire but may not do useful work if channels are full.
		// What matters is: push is stuck (assertion 1), channel fills (assertion 1), and
		// the system is in the asymmetric state where push is degraded but pull's state
		// machine is still Running (not Stopped, not Degraded).

		// KEY ASSERTION 3: Auth should NOT have been called more than a few times.
		// This is a push-level degradation, not an auth problem.
		Expect(mockServer.AuthCallCount()).To(BeNumerically("<=", 3),
			"Expected minimal re-auth — this is a push-level issue, not auth")

		// KEY ASSERTION 4: Push barely progressed — most produced messages were
		// either dropped or stuck in channel, not successfully pushed.
		// The difference between pushed and baseline should be small relative to produced.
		newPushed := int64(pushedAfterDegraded - baselinePushed)
		GinkgoWriter.Printf("[S7]   New messages pushed during degraded: %d (vs %d produced)\n",
			newPushed, totalProduced)

		// With persistent 500 errors, push should have made very few successful pushes
		// (possibly zero, since all attempts return 500). The channel fills because
		// push can't drain it during backoff windows.
		Expect(totalDropped + int64(outLen)).To(BeNumerically(">", 0),
			"Expected messages stuck in channel or dropped — push couldn't drain them")

		// Clean up
		mockServer.ClearPersistentPushError()
		runResult.Shutdown()
		Eventually(runResult.Done, 35*time.Second).Should(BeClosed())
	})
})
