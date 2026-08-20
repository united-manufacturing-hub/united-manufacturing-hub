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

// This asks a different question from push_saturation_scenario_test.go in this
// package. That file varies what the Console DOES and asks which behaviour
// reproduces a log signature. This one holds the Console fixed and varies the
// LOAD, to see whether the failure appears as a dose-response: fine with a
// little, broken with more.
//
// Two knobs, because the production system has two:
//
//   - how many people have the Console open. Each one gets a status message
//     every second (pkg/communicator/pkg/subscriber/subscribers.go).
//   - how big each status message is. A subscriber the agent has already sent
//     the full topic list to gets a small delta; one it has not gets the whole
//     cache (topicbrowser/communicator.go:GetSubscriberData).
//
// The constraint is a BANDWIDTH, not a latency. That distinction is the whole
// experiment. Under a fixed delay, message size cannot affect anything, so an
// experiment that varies size against a fixed delay can only ever report that
// size does not matter. PathFault.BytesPerSecond holds each request for
// ContentLength/rate instead.
//
// The queue under test holds 100 messages, matching cmd/main.go. Every cell
// starts with it EMPTY, so the only thing filling it is the load.

const (
	// Long enough for the queue to fill at the rates below and for the reply
	// injected mid-run to have had a fair chance to get out.
	cellDuration = 35 * time.Second

	// When the tagged reply is put on the queue, well after the load has had
	// time to establish itself.
	replyInjectedAt = 20 * time.Second

	// How long the reply is allowed to take. The browser gives an action 30 s
	// (frontend/src/lib/utils/fetcher.ts:DEFAULT_ACTION_TIMEOUT), so a reply that
	// has not arrived by the end of the cell would have failed in the product.
	replyTag = "TAGGED-ACTION-REPLY"

	// The bandwidth every cell shares. Chosen so one small stream fits easily and
	// a large one cannot: see the assertions for the arithmetic.
	pushBytesPerSecond = 64 * 1024

	smallMessageBytes = 1 * 1024
	largeMessageBytes = 64 * 1024
)

// loadProducer offers one message per second per stream, without blocking, the
// way the production subscriber does.
type loadProducer struct {
	provider *examples.TransportTestChannelProvider
	stop     chan struct{}
	done     chan struct{}
	mu       sync.Mutex
	accepted int
	dropped  int
	maxLen   int
}

func newLoadProducer(p *examples.TransportTestChannelProvider) *loadProducer {
	return &loadProducer{provider: p, stop: make(chan struct{}), done: make(chan struct{})}
}

// run starts `streams` independent once-per-second producers, each offering a
// message of `size` bytes.
func (l *loadProducer) run(streams, size int) {
	var wg sync.WaitGroup

	for st := range streams {
		wg.Add(1)

		go func(stream int) {
			defer wg.Done()

			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			body := strings.Repeat("x", size)

			for i := 0; ; i++ {
				select {
				case <-l.stop:
					return
				case <-ticker.C:
				}

				msg := &types.UMHMessage{
					Content: fmt.Sprintf("s%d-n%d-%s", stream, i, body),
					Email:   fmt.Sprintf("watcher-%d@example.com", stream),
				}

				ok := l.provider.TryQueueOutbound(msg)

				l.mu.Lock()
				if ok {
					l.accepted++
				} else {
					l.dropped++
				}

				if n := l.provider.OutboundLen(); n > l.maxLen {
					l.maxLen = n
				}
				l.mu.Unlock()
			}
		}(st)
	}

	go func() {
		wg.Wait()
		close(l.done)
	}()
}

func (l *loadProducer) halt() (accepted, dropped, maxLen int) {
	close(l.stop)
	<-l.done

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.accepted, l.dropped, l.maxLen
}

// cellResult is what one (streams, size) combination produced.
type cellResult struct {
	streams        int
	messageBytes   int
	offeredBps     int
	replyDelivered bool
	statusAccepted int
	statusDropped  int
	maxQueueLen    int
	queueCap       int
	pushesHandled  int
	pushedBytes    int64
}

func (c cellResult) String() string {
	verdict := "reply DELIVERED"
	if !c.replyDelivered {
		verdict = "reply LOST"
	}

	return fmt.Sprintf(
		"streams=%d msgBytes=%d offered=%dB/s limit=%dB/s | queue=%d/%d dropped=%d accepted=%d "+
			"pushes=%d pushedBytes=%d | %s",
		c.streams, c.messageBytes, c.offeredBps, pushBytesPerSecond,
		c.maxQueueLen, c.queueCap, c.statusDropped, c.statusAccepted,
		c.pushesHandled, c.pushedBytes, verdict)
}

// runCell drives the real transport tree against a Console that accepts
// everything at a fixed bandwidth, under the given load, and reports whether one
// tagged reply injected mid-run reached the Console.
func runCell(streams, messageBytes int) cellResult {
	mockServer := testutil.NewMockRelayServer()
	defer mockServer.Close()

	// The Console answers every request. It is only bandwidth-limited. No error,
	// no stall, no fault of behaviour at all.
	mockServer.SetPathFault("/v2/instance/push", testutil.PathFault{BytesPerSecond: pushBytesPerSecond})

	provider := examples.NewTransportTestChannelProvider(outboundCapacity)
	producer := newLoadProducer(provider)

	ctx, cancel := context.WithTimeout(context.Background(), cellDuration+60*time.Second)
	defer cancel()

	result := examples.RunTransportScenario(ctx, examples.TransportRunConfig{
		Logger:          deps.NewNopFSMLogger(),
		MockServer:      mockServer,
		ChannelProvider: provider,
		HTTPTimeout:     20 * time.Second,
		Duration:        cellDuration,
	})
	Expect(result.Error).NotTo(HaveOccurred())

	producer.run(streams, messageBytes)

	// One reply, offered once, mid-run. If the queue is full at that moment the
	// production code would drop or block; here the offer simply fails and the
	// reply is recorded as lost, which is the same outcome for the user.
	replyOffered := make(chan bool, 1)
	time.AfterFunc(replyInjectedAt, func() {
		replyOffered <- provider.TryQueueOutbound(&types.UMHMessage{
			Content: replyTag,
			Email:   "watcher-0@example.com",
		})
	})

	Eventually(result.Done, cellDuration+50*time.Second).Should(BeClosed())

	accepted, dropped, maxLen := producer.halt()

	delivered := false

	for _, m := range mockServer.GetPushedMessages() {
		if m != nil && strings.Contains(m.Content, replyTag) {
			delivered = true

			break
		}
	}

	select {
	case ok := <-replyOffered:
		if !ok {
			delivered = false
		}
	default:
	}

	return cellResult{
		streams:        streams,
		messageBytes:   messageBytes,
		offeredBps:     streams * messageBytes,
		replyDelivered: delivered,
		statusAccepted: accepted,
		statusDropped:  dropped,
		maxQueueLen:    maxLen,
		queueCap:       provider.OutboundCap(),
		pushesHandled:  len(mockServer.GetPushedMessages()),
		pushedBytes:    mockServer.PushedBytes(),
	}
}

var _ = Describe("Does the failure scale with load rather than with a Console fault", Serial, func() {
	// The Console is identical in every cell: it accepts everything, at
	// 64 KB/s. Only the load changes. So any difference in outcome is caused by
	// the load and cannot be caused by the Console misbehaving.

	It("a light load gets through", func() {
		// 2 streams x 1 KB = 2 KB/s offered against 64 KB/s. 32x headroom.
		cell := runCell(2, smallMessageBytes)

		GinkgoWriter.Printf("CELL light: %s\n", cell)

		Expect(cell.pushesHandled).To(BeNumerically(">", 0),
			"the cell must actually reach the push endpoint, or it measures nothing")
		Expect(cell.statusDropped).To(Equal(0),
			"nothing should be refused with 32x bandwidth headroom")
		Expect(cell.replyDelivered).To(BeTrue(),
			"a reply must get out when the queue is not under pressure")
	})

	It("more watchers on the same small message still get through", func() {
		// 5 streams x 1 KB = 5 KB/s. Still far under the limit. This isolates
		// subscriber count from message size: if THIS breaks, count alone is
		// enough and size is not the variable.
		cell := runCell(5, smallMessageBytes)

		GinkgoWriter.Printf("CELL many-small: %s\n", cell)

		Expect(cell.pushesHandled).To(BeNumerically(">", 0))
		Expect(cell.replyDelivered).To(BeTrue(),
			"five watchers on small messages are still well inside the bandwidth")
	})

	It("one watcher on a large message still gets through, at exactly the limit", func() {
		// 1 stream x 64 KB = 64 KB/s against a 64 KB/s limit: 1.0x, no headroom
		// on paper. It copes anyway, because Phase 2 batches the queue into one
		// request and amortises the per-request cost. Measured, and it contradicts
		// the prediction this spec was first written with -- the failure needs
		// real oversubscription, not merely a saturated link.
		cell := runCell(1, largeMessageBytes)

		GinkgoWriter.Printf("CELL one-large-1x: %s\n", cell)

		Expect(cell.pushesHandled).To(BeNumerically(">", 0),
			"the Console must still be accepting, or this is a stall and not a bandwidth test")
		Expect(cell.replyDelivered).To(BeTrue(),
			"at 1.0x the batching absorbs it")
		Expect(cell.statusDropped).To(Equal(0),
			"and nothing is refused")
	})

	It("two watchers on large messages: where the knee is", func() {
		// 2 streams x 64 KB = 128 KB/s against 64 KB/s: 2.0x. This cell exists to
		// locate the threshold between the 1.0x cell above, which copes, and the
		// 5.0x cell below, which does not. Its assertions record whichever side it
		// lands on rather than predicting one.
		cell := runCell(2, largeMessageBytes)

		GinkgoWriter.Printf("CELL two-large-2x: %s\n", cell)

		Expect(cell.pushesHandled).To(BeNumerically(">", 0),
			"the Console must still be accepting")

		// Measured: the reply is lost here with the queue only about a third full
		// and NOTHING refused. So there are two distinct failure regimes, and this
		// is the milder one -- the reply is accepted onto the queue and then
		// starves behind a backlog it cannot get past. It is silent: no drop, no
		// saturation, no log line. The regime at 5x below is the one that produces
		// the drop lines the incident logs are full of.
		//
		// This assertion was first written the other way round, requiring a full
		// queue for a lost reply. That was wrong and the run said so.
		Expect(cell.replyDelivered).To(BeFalse(),
			"2x oversubscription must already lose the reply")
		Expect(cell.statusDropped).To(Equal(0),
			"and it must do so BEFORE anything is refused -- the user-visible failure "+
				"arrives before the log signature does")
		Expect(cell.maxQueueLen).To(BeNumerically("<", cell.queueCap),
			"the queue must NOT be full, or this is the same regime as the 5x cell")
	})

	It("five watchers on large messages saturate the queue and lose the reply", func() {
		// 5 streams x 64 KB = 320 KB/s against 64 KB/s: 5.0x oversubscribed.
		cell := runCell(5, largeMessageBytes)

		GinkgoWriter.Printf("CELL five-large-5x: %s\n", cell)

		Expect(cell.maxQueueLen).To(Equal(cell.queueCap))
		Expect(cell.statusDropped).To(BeNumerically(">", 0))
		Expect(cell.replyDelivered).To(BeFalse())
	})
})
