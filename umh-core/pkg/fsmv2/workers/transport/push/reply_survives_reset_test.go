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
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// The push child drains the outbound channel and POSTs to the Management
// Console. A message it drained but failed to deliver goes into a pending list
// and is retried on the next tick. When the parent transport resets, that list
// is discarded whole: CheckAndClearOnReset sets pendingMessages to nil.
//
// Discarding is right for a status message. Status is a snapshot of the whole
// instance, produced once per second per subscriber, so a lost one is replaced
// before anyone notices.
//
// It is wrong for an action reply. A reply is the only answer to one specific
// user request, nothing regenerates it, and the action that produced it has
// already returned. Discarding it strands the user, who sees "No response from
// the UMH Instance" after the browser's own 30-second deadline and concludes
// the instance is down.
//
// Both kinds of message share one pending list and carry nothing that
// distinguishes them, so the discard cannot currently tell them apart.
//
// These specs use the real PushDependencies, not a double. A mock's
// CheckAndClearOnReset does not clear anything, so a mock would pass while
// production loses the reply.

// recordingTransport accumulates every message handed to Push across all calls.
// The mock in push_test.go assigns rather than appends, so it only ever shows
// the last call -- a test built on it cannot tell "delivered on the retry" from
// "never delivered".
type recordingTransport struct {
	mu       sync.Mutex
	pushed   []*types.UMHMessage
	pushErr  error
	pushCall int
	// onPush runs before the error is returned, so a spec can change the world
	// mid-push -- cancelling the action context, for instance.
	onPush func()
}

func (t *recordingTransport) Authenticate(_ context.Context, _ types.AuthRequest) (types.AuthResponse, error) {
	return types.AuthResponse{}, nil
}

func (t *recordingTransport) Pull(_ context.Context, _ string) ([]*types.UMHMessage, error) {
	return nil, nil
}

func (t *recordingTransport) Push(_ context.Context, _ string, messages []*types.UMHMessage) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.pushCall++

	if t.onPush != nil {
		t.onPush()
	}

	if t.pushErr != nil {
		return t.pushErr
	}

	t.pushed = append(t.pushed, messages...)

	return nil
}

func (t *recordingTransport) Close() {}

func (t *recordingTransport) Reset() {}

func (t *recordingTransport) delivered() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make([]string, 0, len(t.pushed))
	for _, m := range t.pushed {
		out = append(out, m.Content)
	}

	return out
}

func (t *recordingTransport) setErr(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.pushErr = err
}

// writableChannelProvider hands out the same channels it keeps a writable end
// of, so a spec can enqueue onto the outbound channel the push child reads.
type writableChannelProvider struct {
	inbound  chan *types.UMHMessage
	outbound chan *types.UMHMessage
}

func (p *writableChannelProvider) GetChannels(_ string) (chan<- *types.UMHMessage, <-chan *types.UMHMessage) {
	return p.inbound, p.outbound
}

func (p *writableChannelProvider) GetInboundStats(_ string) (int, int) {
	return cap(p.inbound), len(p.inbound)
}

var _ = Describe("An undelivered action reply across a transport reset", func() {
	const (
		replyContent  = "action-reply:get-protocolconverter"
		statusContent = "status-snapshot"
	)

	var (
		provider   *writableChannelProvider
		trans      *recordingTransport
		pushDeps   *push.PushDependencies
		parentDeps *transportpkg.TransportDependencies
		act        *action.PushAction
		transient  *types.TransportError
	)

	BeforeEach(func() {
		logger := deps.NewNopFSMLogger()

		provider = &writableChannelProvider{
			inbound:  make(chan *types.UMHMessage, 100),
			outbound: make(chan *types.UMHMessage, 100),
		}
		transportpkg.SetChannelProvider(provider)

		trans = &recordingTransport{}
		parentDeps = transportpkg.NewTransportDependencies(
			trans,
			deps.NewBaseDependencies(logger, nil, deps.Identity{ID: "parent-id", WorkerType: "transport"}),
		)

		var err error
		pushDeps, err = push.NewPushDependencies(
			parentDeps,
			deps.NewBaseDependencies(logger, nil, deps.Identity{ID: "push-child-id", WorkerType: "push"}),
		)
		Expect(err).NotTo(HaveOccurred())

		act = &action.PushAction{JWTToken: "test-token", InstanceUUID: "test-instance"}

		// A network timeout is the failure the incident logs show, and it is
		// transient, so the push child keeps the messages for a retry rather
		// than discarding them as poison.
		transient = &types.TransportError{Type: types.ErrorTypeNetwork, Message: "timeout"}
	})

	// SetChannelProvider is process-global. Without this the next spec in the
	// package inherits these channels and its result depends on spec order.
	AfterEach(func() {
		transportpkg.ClearChannelProvider()
	})

	// queueAndFailOnce puts one reply and one status message on the outbound
	// channel, then runs a tick whose push fails. Both messages end up in the
	// pending list. Asserting the pending count here is what keeps the specs
	// below from passing vacuously against an empty list.
	queueAndFailOnce := func() {
		provider.outbound <- &types.UMHMessage{Content: replyContent, Email: "operator@example.com"}
		provider.outbound <- &types.UMHMessage{Content: statusContent, Email: "operator@example.com"}

		trans.setErr(transient)

		Expect(act.Execute(context.Background(), pushDeps)).To(Succeed(),
			"a transient push failure is absorbed, not returned")
		Expect(pushDeps.PendingMessageCount()).To(Equal(2),
			"both messages must be held for retry before the reset")
		Expect(trans.delivered()).To(BeEmpty(),
			"nothing was delivered on the failing tick")
	}

	// This is the control. It runs every line of the spec below except the
	// reset, and it must pass both before and after the discard is fixed. If it
	// fails, the spec below is not evidence about the reset -- the retry path
	// itself broke.
	It("is delivered on the next tick when no reset intervenes", func() {
		queueAndFailOnce()

		trans.setErr(nil)
		Expect(act.Execute(context.Background(), pushDeps)).To(Succeed())

		Expect(trans.delivered()).To(ContainElement(replyContent),
			"the reply must reach the Console once the transport recovers")
		Expect(pushDeps.PendingMessageCount()).To(Equal(0),
			"the pending list must be empty once everything is delivered")
	})

	It("is delivered on the next tick when a reset intervenes", func() {
		queueAndFailOnce()

		// This is what the incident logs record 25 times, as
		// push_reset_cleared with pending_dropped up to 459.
		parentDeps.IncrementResetGeneration()

		trans.setErr(nil)
		Expect(act.Execute(context.Background(), pushDeps)).To(Succeed())

		Expect(trans.delivered()).To(ContainElement(replyContent),
			"a transport reset discarded an action reply that had never been delivered; "+
				"nothing regenerates it and the user is told the instance did not answer")
	})
})
