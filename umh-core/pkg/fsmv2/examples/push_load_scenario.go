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

package examples

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// Two scenarios that show, in the log, what happens to a reply to the
// Management Console when the outbound queue cannot keep up.
//
// The Console in both runs accepts every request. It never errors and never
// stalls. The only limit is how fast bytes can leave, which is what makes the
// two runs differ: one offers far less than the link can carry, the other offers
// five times more.
//
//	go run ./pkg/fsmv2/cmd/runner --scenario push-healthy    --duration 40s
//	go run ./pkg/fsmv2/cmd/runner --scenario push-overloaded --duration 40s
//
// Watch the `push_load` line once a second. Under push-healthy the queue stays
// near empty and the reply arrives. Under push-overloaded the queue climbs to its
// limit of 100, status messages start being refused, and the reply never
// arrives — while the Console is answering every request it is given.
//
// The assertions for the same behaviour, and the intermediate load levels that
// locate the threshold between them, are in push_dose_response_test.go.

const (
	// The push endpoint's bandwidth in both runs.
	loadScenarioBytesPerSecond = 64 * 1024

	// The outbound queue's capacity, matching cmd/main.go.
	loadScenarioQueueCapacity = 100

	// How long each stream waits between messages. One second per watcher is
	// what pkg/communicator/pkg/subscriber/subscribers.go does.
	loadScenarioInterval = time.Second

	// When the tagged reply is offered, far enough in for the load to have
	// settled.
	loadScenarioReplyAt = 20 * time.Second

	// The tagged reply's content, so the run can report whether it got out.
	loadScenarioReplyTag = "TAGGED-ACTION-REPLY"
)

// PushLoadConfig describes one load: how many people have the Console open, and
// how big each of their status messages is.
type PushLoadConfig struct {
	// Streams is the number of watchers. Each offers one message per second.
	Streams int
	// MessageBytes is the size of each status message.
	MessageBytes int
}

// pushLoadProducer offers messages without blocking, the way the production
// subscriber does, and counts what the queue refused.
type pushLoadProducer struct {
	provider *TransportTestChannelProvider
	stop     chan struct{}
	done     chan struct{}
	mu       sync.Mutex
	accepted int
	refused  int
}

func (p *pushLoadProducer) start(cfg PushLoadConfig) {
	var wg sync.WaitGroup

	for s := range cfg.Streams {
		wg.Add(1)

		go func(stream int) {
			defer wg.Done()

			ticker := time.NewTicker(loadScenarioInterval)
			defer ticker.Stop()

			body := strings.Repeat("x", cfg.MessageBytes)

			for i := 0; ; i++ {
				select {
				case <-p.stop:
					return
				case <-ticker.C:
				}

				ok := p.provider.TryQueueOutbound(&types.UMHMessage{
					Content: fmt.Sprintf("s%d-n%d-%s", stream, i, body),
					Email:   fmt.Sprintf("watcher-%d@example.com", stream),
				})

				p.mu.Lock()
				if ok {
					p.accepted++
				} else {
					p.refused++
				}
				p.mu.Unlock()
			}
		}(s)
	}

	go func() {
		wg.Wait()
		close(p.done)
	}()
}

func (p *pushLoadProducer) counts() (accepted, refused int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.accepted, p.refused
}

func (p *pushLoadProducer) halt() {
	close(p.stop)
	<-p.done
}

// RunPushLoadScenario drives the transport worker tree against a Console that
// accepts everything at a fixed bandwidth, under the given load, and logs the
// queue's state once a second so the behaviour is visible while it happens.
func RunPushLoadScenario(ctx context.Context, cfg RunConfig, load PushLoadConfig) (*RunResult, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = deps.NewNopFSMLogger()
	}

	mockServer := testutil.NewMockRelayServer()
	mockServer.SetPathFault("/v2/instance/push", testutil.PathFault{
		BytesPerSecond: loadScenarioBytesPerSecond,
	})

	provider := NewTransportTestChannelProvider(loadScenarioQueueCapacity)
	producer := &pushLoadProducer{
		provider: provider,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}

	logger.Info("push_load_start",
		deps.Int("watchers", load.Streams),
		deps.Int("message_bytes", load.MessageBytes),
		deps.Int("offered_bytes_per_second", load.Streams*load.MessageBytes),
		deps.Int("push_limit_bytes_per_second", loadScenarioBytesPerSecond),
		deps.Int("queue_capacity", loadScenarioQueueCapacity))

	inner := RunTransportScenario(ctx, TransportRunConfig{
		Logger:          logger,
		MockServer:      mockServer,
		ChannelProvider: provider,
		HTTPTimeout:     20 * time.Second,
		Duration:        cfg.Duration,
		TickInterval:    cfg.TickInterval,
	})
	if inner.Error != nil {
		mockServer.Close()

		return nil, inner.Error
	}

	producer.start(load)

	// One reply, offered once. A failed offer is what the production subscriber
	// logs as a drop; either way the person waiting in the browser sees nothing.
	replyOffered := make(chan bool, 1)
	replyTimer := time.AfterFunc(loadScenarioReplyAt, func() {
		ok := provider.TryQueueOutbound(&types.UMHMessage{
			Content: loadScenarioReplyTag,
			Email:   "watcher-0@example.com",
		})
		replyOffered <- ok

		logger.Info("push_load_reply_offered",
			deps.Bool("accepted_onto_queue", ok),
			deps.Int("queue_depth", provider.OutboundLen()))
	})

	done := make(chan struct{})

	// The once-a-second progress line. This is the point of running the scenario
	// from the CLI rather than from a test: someone can watch the queue fill.
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-inner.Done:
				return
			case <-ticker.C:
			}

			accepted, refused := producer.counts()

			logger.Info("push_load",
				deps.Int("queue_depth", provider.OutboundLen()),
				deps.Int("queue_capacity", loadScenarioQueueCapacity),
				deps.Int("status_accepted", accepted),
				deps.Int("status_refused", refused),
				deps.Int("bytes_delivered", int(mockServer.PushedBytes())))
		}
	}()

	go func() {
		defer close(done)

		<-inner.Done

		replyTimer.Stop()
		producer.halt()

		delivered := false

		for _, m := range mockServer.GetPushedMessages() {
			if m != nil && strings.Contains(m.Content, loadScenarioReplyTag) {
				delivered = true

				break
			}
		}

		accepted, refused := producer.counts()

		logger.Info("push_load_complete",
			deps.Int("watchers", load.Streams),
			deps.Int("message_bytes", load.MessageBytes),
			deps.Bool("reply_reached_console", delivered),
			deps.Int("status_accepted", accepted),
			deps.Int("status_refused", refused),
			deps.Int("pushes_accepted", len(mockServer.GetPushedMessages())),
			deps.Int("bytes_delivered", int(mockServer.PushedBytes())))

		mockServer.Close()
	}()

	return &RunResult{Done: done, Shutdown: inner.Shutdown}, nil
}

// PushHealthyScenarioEntry offers far less than the link can carry.
var PushHealthyScenarioEntry = Scenario{
	Name:        "push-healthy",
	Description: "Two watchers on small status messages: the queue stays empty and a reply gets through",
	CustomRunner: func(ctx context.Context, cfg RunConfig) (*RunResult, error) {
		return RunPushLoadScenario(ctx, cfg, PushLoadConfig{Streams: 2, MessageBytes: 1024})
	},
}

// PushOverloadedScenarioEntry offers five times what the link can carry.
var PushOverloadedScenarioEntry = Scenario{
	Name:        "push-overloaded",
	Description: "Five watchers on large status messages: the queue saturates and the reply never arrives",
	CustomRunner: func(ctx context.Context, cfg RunConfig) (*RunResult, error) {
		return RunPushLoadScenario(ctx, cfg, PushLoadConfig{Streams: 5, MessageBytes: 64 * 1024})
	},
}
