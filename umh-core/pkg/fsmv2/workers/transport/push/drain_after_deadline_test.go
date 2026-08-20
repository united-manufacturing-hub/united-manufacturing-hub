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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// recordingTransport and writableChannelProvider are defined in
// reply_survives_reset_test.go, in this package.
//
// The push child has two phases. Phase 1 retries the pending list, one message
// per HTTP request. Phase 2 drains the outbound channel and pushes what it
// found. Phase 1 returning early skips Phase 2, so on a failing tick the only
// thing that relieves the channel is drainChannelToPending, called from Phase 1.
//
// That call is guarded: `if ctx.Err() == nil`. The guard means "only drain while
// there is still time", and the comment beside it says the messages "survive in
// the channel until the next tick". Both are true. What neither accounts for is
// that a stuck tick reaches its own deadline every time, so the guard is false
// on exactly the ticks where the drain is needed, and the channel is never
// relieved.
//
// The consequence is not a lost message -- what is in the channel stays there.
// It is a channel that stays full, which is a different injury: producers can no
// longer enqueue. Status messages are dropped (subscribers.go logs one per
// second per subscriber) and action replies block, because
// sendActionReplyInternal writes without a select.
//
// So this spec asserts what a producer can observe: after a tick that hit its
// deadline, there must be somewhere to put the next message.
//
// KNOWN CONFLICT, do not resolve it by reverting the fix. The existing spec
// push/action/push_test.go "should skip channel drain when context is canceled
// during retry" asserts `HaveLen(1)` with the reason "channel should NOT be
// drained when ctx is canceled". It states today's behaviour as the requirement.
// A change that satisfies this file must delete that spec, not appease it.
var _ = Describe("The outbound channel after a tick that hit its deadline", func() {
	var (
		provider   *writableChannelProvider
		trans      *recordingTransport
		pushDeps   *push.PushDependencies
		act        *action.PushAction
		transient  *types.TransportError
		cancelPush context.CancelFunc
		pushCtx    context.Context
	)

	BeforeEach(func() {
		logger := deps.NewNopFSMLogger()

		// Capacity 3 rather than production's 100. The behaviour under test is
		// "the channel is at capacity and stays there", which does not depend
		// on the capacity, and a small channel makes the fill explicit.
		provider = &writableChannelProvider{
			inbound:  make(chan *types.UMHMessage, 3),
			outbound: make(chan *types.UMHMessage, 3),
		}
		transportpkg.SetChannelProvider(provider)

		trans = &recordingTransport{}
		parentDeps := transportpkg.NewTransportDependencies(
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
		transient = &types.TransportError{Type: types.ErrorTypeNetwork, Message: "timeout"}

		pushCtx, cancelPush = context.WithCancel(context.Background())

		// Phase 1 only runs when the pending list is non-empty, and only Phase 1
		// can relieve the channel on a failing tick.
		pushDeps.StorePendingMessages([]*types.UMHMessage{{Content: "pending-status"}})

		// Fill the channel. Asserting the fill is what stops these specs from
		// passing against a channel that had room all along.
		for range cap(provider.outbound) {
			provider.outbound <- &types.UMHMessage{Content: "queued-status"}
		}
		Expect(provider.outbound).To(HaveLen(cap(provider.outbound)),
			"the outbound channel must be full before the tick runs")

		trans.setErr(transient)
	})

	AfterEach(func() {
		cancelPush()
		transportpkg.ClearChannelProvider()
	})

	// The control. Identical to the spec below except that the context stays
	// live. It must pass before and after the guard is fixed; if it fails, the
	// spec below says nothing about the deadline.
	It("has room again when the tick still had time left", func() {
		Expect(act.Execute(pushCtx, pushDeps)).To(Succeed())

		Expect(len(provider.outbound)).To(BeNumerically("<", cap(provider.outbound)),
			"a failing tick with time left must relieve the channel")
	})

	It("has room again when the tick ran out of time", func() {
		// Reaching the deadline mid-push is what the incident logs record 64
		// times, every one of them at duration_ms 30000 against
		// timeout_ms 30000.
		trans.onPush = cancelPush

		err := act.Execute(pushCtx, pushDeps)
		Expect(err).To(MatchError(context.Canceled),
			"the tick must report that it ran out of time")

		Expect(len(provider.outbound)).To(BeNumerically("<", cap(provider.outbound)),
			"the outbound channel stayed full after a tick that hit its deadline; "+
				"every producer is now blocked or dropping, and the next tick starts equally stuck")
	})
})
