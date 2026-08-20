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
	"math"
	"strings"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// One scenario that shows, in the log, what happens to a reply to the Management
// Console when the outbound queue cannot keep up. Every knob is a flag, so the
// same scenario covers a link that is barely used and one that is oversubscribed.
//
// The Console accepts every request. It never errors and never stalls. The only
// limit is how fast bytes can leave, so what separates one run from the next is
// how much is offered against --bandwidth.
//
//	go run ./pkg/fsmv2/cmd/runner --scenario push-load --duration 30s \
//	    --bandwidth 65536 --subscribers 2 --payload-bytes 1024
//
//	go run ./pkg/fsmv2/cmd/runner --scenario push-load --duration 30s \
//	    --bandwidth 65536 --subscribers 5 --payload-bytes 65536
//
// Watch the `push_load` line once a second: the queue stays near empty in the
// first run and saturates in the second. The last line, `push_load_summary`,
// carries the numbers this scenario exists to produce -- degrade cycles per
// minute, budget expiries per minute, resets per minute, and how much of the run
// the queue spent full -- next to an echo of the settings that produced them, so
// two runs can be compared field by field.

const (
	// The log messages the summary counts, emitted by
	// supervisor/reconciliation.go,
	// supervisor/internal/execution/action_executor.go and
	// workers/transport/push/action/push.go.
	eventStateTransition  = "state_transition"
	eventActionFailed     = "action_failed"
	eventPushResetCleared = "push_reset_cleared"
)

const (
	// How --topic-count becomes a payload size: a fixed floor plus a per-topic
	// slope. Both numbers are measured on the wire, meaning after zstd
	// compression and base64, which is what actually crosses the link. Source:
	// payload-size-calibration.md, the internal calibration note of 2026-08-20.
	//
	// statusFloorBytes is what a status message costs carrying no topic-browser
	// bundle at all. It dominates the slope for an instance with few topics,
	// which is the case this scenario is usually pointed at, so the floor
	// matters more than the slope here.
	//
	// bytesPerTopic is a single-point estimate from one instance at 4,330
	// topics, obtained by dividing a byte rate by an assumed 10s resend period.
	// No slope was fitted, and other sources in that note range from 207 to 783
	// bytes per topic. So --topic-count gives an order of magnitude, not a
	// threshold, and a topic count must not be quoted as a limit.
	statusFloorBytes = 8900
	bytesPerTopic    = 345

	// Defaults for a run that names no settings. --bandwidth has no default: zero
	// means unlimited, which is a setting rather than a missing one.
	defaultSubscribers  = 2
	defaultPayloadBytes = 1024
	// Production's outbound queue capacity, from cmd/main.go.
	defaultQueueCapacity = 100
	// Production's per-request HTTP timeout.
	defaultHTTPTimeout = 10 * time.Second

	// How long each stream waits between messages. One second per watcher is
	// what pkg/communicator/pkg/subscriber/subscribers.go does.
	loadScenarioInterval = time.Second

	// When the tagged reply is offered. The CLI bounds a v1 scenario with a
	// context timeout rather than a duration (see routeDuration in
	// cmd/runner/main.go), so the run cannot read its own length and this has to
	// be a fixed point. A run shorter than this never offers the reply, which is
	// why the summary reports reply_offered next to reply_reached_console.
	loadScenarioReplyAt = 20 * time.Second

	// The tagged reply's content, so the run can report whether it got out.
	loadScenarioReplyTag = "TAGGED-ACTION-REPLY"

	// The state whose entries the summary counts as degrade cycles.
	degradedStateName = "Degraded"

	// The suffixes an FSMv2 worker path ends with, one per child of the
	// transport tree. The pull child emits byte-identical Degraded transitions to
	// the push child, so a count that does not filter on these is wrong.
	transportPathSuffix = "(transport)"
	pushPathSuffix      = "(push)"

	// The error text that marks a push whose per-action budget ran out. The
	// wrapper "context canceled during retry" must not be matched instead: an
	// ordinary shutdown produces it too, so counting it reports teardown as a
	// budget expiry.
	budgetExpiryError = "context deadline exceeded"

	// The action whose failures count towards budget_expiries_per_min.
	pushActionName = "push"
)

// PushLoadConfig is one setting of the push-load scenario: how many people have
// the Console open, how big each of their status messages is, and what the link
// and the queue underneath them allow.
//
// Its zero value asks for the defaults. RunPushLoadScenario fills every
// non-positive field, which is what lets RunConfig carry it without changing any
// other scenario.
type PushLoadConfig struct {
	// HTTPTimeout caps each individual push request. Zero means the production
	// value, 10s.
	HTTPTimeout time.Duration
	// Subscribers is the number of watchers. Each offers one message per second.
	// Zero means 2.
	Subscribers int
	// PayloadBytes is the size of each status message. Zero means 1024.
	PayloadBytes int
	// TopicCount, when non-zero, replaces PayloadBytes with
	// statusFloorBytes + TopicCount*bytesPerTopic.
	TopicCount int
	// BandwidthBytesPerSecond is how fast the push endpoint accepts bytes. Zero
	// means unlimited: no bandwidth fault is applied at all.
	BandwidthBytesPerSecond int
	// QueueCapacity is the outbound queue's capacity. Zero means the production
	// value, 100.
	QueueCapacity int
	// StallEveryNth makes every Nth push request hang for StallFor, leaving the
	// rest untouched. This is the only dial that models a link whose median
	// request is fast and whose tail is not; BandwidthBytesPerSecond and a fixed
	// delay both act on every request equally. Zero disables it.
	StallEveryNth int
	// StallBurst is how many consecutive pushes stall at the start of each
	// period. This is a separate dial from the period because an isolated slow
	// push leaves no trace: the failure is absorbed and a child needs three
	// CONSECUTIVE errors to degrade. Defaults to 1.
	StallBurst int
	// StallFor is how long a stalled push is held. Ignored unless StallEveryNth
	// is set.
	StallFor time.Duration
}

// withDefaults returns the config with every unset field filled in.
func (c PushLoadConfig) withDefaults() PushLoadConfig {
	if c.Subscribers <= 0 {
		c.Subscribers = defaultSubscribers
	}

	if c.PayloadBytes <= 0 {
		c.PayloadBytes = defaultPayloadBytes
	}

	if c.TopicCount > 0 {
		c.PayloadBytes = statusFloorBytes + c.TopicCount*bytesPerTopic
	}

	if c.QueueCapacity <= 0 {
		c.QueueCapacity = defaultQueueCapacity
	}

	if c.HTTPTimeout <= 0 {
		c.HTTPTimeout = defaultHTTPTimeout
	}

	return c
}

// pushLoadCounters holds every number the summary reports that is not already
// counted somewhere else: the events the workers log, and once-a-second samples
// of the outbound queue.
//
// Events are counted where the FSMLogger method is called, not by matching lines
// in the log output. Line counting would be wrong twice over. The logger the CLI
// passes in samples repeated messages (deps.NewFSMLogger wraps the core in
// pkg/logger's level sampler), so at any real rate lines are missing from the
// output; and a line written twice by anything downstream would double a count
// taken from the output, while a count taken per call cannot move.
//
// state_transition is de-duplicated on top of that: a repeat of the same
// worker's last from/to pair is dropped, because a state machine cannot make the
// same transition twice without making a different one in between. action_failed
// and push_reset_cleared carry no id that could identify a repeat -- the push
// action's correlation_id is the literal string "push" -- so they are counted
// once per call and reported both as a rate and as a raw event count, so the
// count can be checked against the log by hand.
type pushLoadCounters struct {
	lastTransition map[string]string
	totals         pushLoadTotals
	// tearingDown latches when any worker first heads for a stopped state. Every
	// event after that is shutdown, and shutdown produces transitions whose
	// reason is indistinguishable from an overload one ("children unhealthy") --
	// a 30s run reported 1.94 transport degrade cycles per minute that were
	// entirely teardown. Counting stops at the latch rather than trying to tell
	// the two apart by reason.
	tearingDown bool
	mu          sync.Mutex
}

// pushLoadTotals is what pushLoadCounters has counted so far, copyable so the
// summary can read every number at one instant.
type pushLoadTotals struct {
	transportDegrades int
	pushDegrades      int
	budgetExpiries    int
	resets            int
	maxPendingDropped int
	queueSamples      int
	queueFullSamples  int
}

func newPushLoadCounters() *pushLoadCounters {
	return &pushLoadCounters{lastTransition: make(map[string]string)}
}

// observe counts one log call. err is the error SentryError was given, and nil
// for Info.
func (c *pushLoadCounters) observe(msg string, err error, fields []deps.Field) {
	if msg == eventStateTransition && headingForStop(fieldString(fields, "to_state")) {
		c.latchTeardown()
	}

	if c.tornDown() {
		return
	}

	switch msg {
	case eventStateTransition:
		c.countTransition(fields)
	case eventActionFailed:
		c.countActionFailure(err, fields)
	case eventPushResetCleared:
		c.countReset(fields)
	}
}

// headingForStop reports whether a destination state means the run is winding
// down rather than reacting to load.
func headingForStop(to string) bool {
	return strings.Contains(to, "Stop") || strings.Contains(to, "ShuttingDown")
}

func (c *pushLoadCounters) latchTeardown() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tearingDown = true
}

func (c *pushLoadCounters) tornDown() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.tearingDown
}

func (c *pushLoadCounters) countTransition(fields []deps.Field) {
	worker := fieldString(fields, "worker")
	from := fieldString(fields, "from_state")
	to := fieldString(fields, "to_state")
	transition := from + "->" + to

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.lastTransition[worker] == transition {
		return
	}

	c.lastTransition[worker] = transition

	if to != degradedStateName {
		return
	}

	switch {
	case strings.HasSuffix(worker, transportPathSuffix):
		c.totals.transportDegrades++
	case strings.HasSuffix(worker, pushPathSuffix):
		c.totals.pushDegrades++
	}
}

func (c *pushLoadCounters) countActionFailure(err error, fields []deps.Field) {
	if fieldString(fields, "action_name") != pushActionName {
		return
	}

	if err == nil || !strings.Contains(err.Error(), budgetExpiryError) {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.totals.budgetExpiries++
}

func (c *pushLoadCounters) countReset(fields []deps.Field) {
	dropped := fieldInt(fields, "pending_dropped")

	c.mu.Lock()
	defer c.mu.Unlock()

	c.totals.resets++

	if dropped > c.totals.maxPendingDropped {
		c.totals.maxPendingDropped = dropped
	}
}

// sampleQueue records one once-a-second observation of the outbound queue.
func (c *pushLoadCounters) sampleQueue(depth, capacity int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.totals.queueSamples++

	if capacity > 0 && depth == capacity {
		c.totals.queueFullSamples++
	}
}

// snapshot returns every counter, read under one lock.
func (c *pushLoadCounters) snapshot() pushLoadTotals {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.totals
}

// fieldString returns the last string value logged under key, or "".
// Last wins because the same key can be added twice: the supervisor attaches
// the worker path with With, and a worker's own dependencies attach it again
// (supervisor/api.go and deps.NewBaseDependencies).
func fieldString(fields []deps.Field, key string) string {
	for i := len(fields) - 1; i >= 0; i-- {
		if fields[i].Key != key {
			continue
		}

		if s, ok := fields[i].Value.(string); ok {
			return s
		}
	}

	return ""
}

// fieldInt returns the last int value logged under key, or 0.
func fieldInt(fields []deps.Field, key string) int {
	for i := len(fields) - 1; i >= 0; i-- {
		if fields[i].Key != key {
			continue
		}

		if n, ok := fields[i].Value.(int); ok {
			return n
		}
	}

	return 0
}

// pushLoadCountingLogger counts the events the summary needs and forwards every
// call to the logger underneath, so a person watching the CLI still sees the
// whole log.
//
// It keeps the fields added by With. The supervisor attaches the worker path
// that way, and the state_transition call itself does not carry it, so a
// wrapper that only looked at the call's own fields could not tell the push
// child's Degraded transitions from the pull child's.
type pushLoadCountingLogger struct {
	base     []deps.Field
	inner    deps.FSMLogger
	counters *pushLoadCounters
}

func newPushLoadCountingLogger(inner deps.FSMLogger, counters *pushLoadCounters) *pushLoadCountingLogger {
	return &pushLoadCountingLogger{inner: inner, counters: counters}
}

func (l *pushLoadCountingLogger) Debug(msg string, fields ...deps.Field) {
	l.inner.Debug(msg, fields...)
}

func (l *pushLoadCountingLogger) Info(msg string, fields ...deps.Field) {
	l.counters.observe(msg, nil, l.allFields(fields))
	l.inner.Info(msg, fields...)
}

func (l *pushLoadCountingLogger) SentryWarn(feature deps.Feature, hierarchyPath string, msg string, fields ...deps.Field) {
	l.counters.observe(msg, nil, l.allFields(fields))
	l.inner.SentryWarn(feature, hierarchyPath, msg, fields...)
}

func (l *pushLoadCountingLogger) SentryError(feature deps.Feature, hierarchyPath string, err error, msg string, fields ...deps.Field) {
	l.counters.observe(msg, err, l.allFields(fields))
	l.inner.SentryError(feature, hierarchyPath, err, msg, fields...)
}

func (l *pushLoadCountingLogger) With(fields ...deps.Field) deps.FSMLogger {
	base := make([]deps.Field, 0, len(l.base)+len(fields))
	base = append(base, l.base...)
	base = append(base, fields...)

	return &pushLoadCountingLogger{
		inner:    l.inner.With(fields...),
		counters: l.counters,
		base:     base,
	}
}

func (l *pushLoadCountingLogger) allFields(fields []deps.Field) []deps.Field {
	if len(l.base) == 0 {
		return fields
	}

	all := make([]deps.Field, 0, len(l.base)+len(fields))
	all = append(all, l.base...)
	all = append(all, fields...)

	return all
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

	for s := range cfg.Subscribers {
		wg.Add(1)

		go func(stream int) {
			defer wg.Done()

			ticker := time.NewTicker(loadScenarioInterval)
			defer ticker.Stop()

			body := strings.Repeat("x", cfg.PayloadBytes)

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
// accepts everything at a fixed bandwidth, under the given load. It logs the
// queue's state once a second so the behaviour is visible while it happens, and
// one push_load_summary line at the end with the run's numbers next to the
// settings that produced them.
func RunPushLoadScenario(ctx context.Context, cfg RunConfig, load PushLoadConfig) (*RunResult, error) {
	load = load.withDefaults()

	logger := cfg.Logger
	if logger == nil {
		logger = deps.NewNopFSMLogger()
	}

	counters := newPushLoadCounters()

	mockServer := testutil.NewMockRelayServer()

	fault := testutil.PathFault{
		BytesPerSecond: load.BandwidthBytesPerSecond,
		StallEveryNth:  load.StallEveryNth,
		StallBurst:     load.StallBurst,
		StallFor:       load.StallFor,
	}
	if fault != (testutil.PathFault{}) {
		mockServer.SetPathFault("/v2/instance/push", fault)
	}

	provider := NewTransportTestChannelProvider(load.QueueCapacity)
	producer := &pushLoadProducer{
		provider: provider,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}

	logger.Info("push_load_start",
		deps.Int("subscribers", load.Subscribers),
		deps.Int("payload_bytes", load.PayloadBytes),
		deps.Int("topic_count", load.TopicCount),
		deps.Int("offered_bytes_per_second", load.Subscribers*load.PayloadBytes),
		deps.Int("bandwidth_bytes_per_second", load.BandwidthBytesPerSecond),
		deps.Int("queue_capacity", load.QueueCapacity),
		deps.Int("stall_every_nth", load.StallEveryNth),
		deps.String("stall_for", load.StallFor.String()),
		deps.Duration("http_timeout", load.HTTPTimeout))

	// The supervisor tree logs through the counting wrapper; this scenario's own
	// lines go straight to the logger underneath, so they cannot be counted as
	// worker events.
	inner := RunTransportScenario(ctx, TransportRunConfig{
		Logger:          newPushLoadCountingLogger(logger, counters),
		MockServer:      mockServer,
		ChannelProvider: provider,
		HTTPTimeout:     load.HTTPTimeout,
		Duration:        cfg.Duration,
		TickInterval:    cfg.TickInterval,
	})
	if inner.Error != nil {
		mockServer.Close()

		return nil, inner.Error
	}

	startedAt := time.Now()

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

	// Declared before the watcher goroutine because the watcher writes
	// ShutdownClean into it. The CLI reads that field only after done closes, and
	// the watcher writes it before closing done, so the write is visible.
	outer := &RunResult{Done: done, Shutdown: inner.Shutdown}

	// The once-a-second progress line. This is the point of running the scenario
	// from the CLI rather than from a test: someone can watch the queue fill.
	// The same tick samples the queue for queue_full_duty_pct.
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
			depth := provider.OutboundLen()

			counters.sampleQueue(depth, load.QueueCapacity)

			logger.Info("push_load",
				deps.Int("queue_depth", depth),
				deps.Int("queue_capacity", load.QueueCapacity),
				deps.Int("status_accepted", accepted),
				deps.Int("status_refused", refused),
				deps.Int("bytes_delivered", int(mockServer.PushedBytes())))
		}
	}()

	go func() {
		defer close(done)

		<-inner.Done

		// Carry the supervisor's drain outcome out to the CLI. A CustomRunner that
		// leaves this false makes shutdownExitCode return 1 on every run, however
		// well the run went.
		outer.ShutdownClean = inner.ShutdownClean

		elapsed := time.Since(startedAt)

		replyTimer.Stop()
		producer.halt()

		offered := false
		select {
		case offered = <-replyOffered:
		default:
		}

		delivered := false

		for _, m := range mockServer.GetPushedMessages() {
			if m != nil && strings.Contains(m.Content, loadScenarioReplyTag) {
				delivered = true

				break
			}
		}

		accepted, refused := producer.counts()
		counted := counters.snapshot()

		logger.Info("push_load_complete",
			deps.Int("subscribers", load.Subscribers),
			deps.Int("payload_bytes", load.PayloadBytes),
			deps.Bool("reply_reached_console", delivered),
			deps.Int("status_accepted", accepted),
			deps.Int("status_refused", refused),
			deps.Int("pushes_accepted", len(mockServer.GetPushedMessages())),
			deps.Int("bytes_delivered", int(mockServer.PushedBytes())))

		logger.Info("push_load_summary", pushLoadSummaryFields(load, counted, elapsed,
			pushLoadOutcome{
				accepted:       accepted,
				refused:        refused,
				replyOffered:   offered,
				replyReached:   delivered,
				pushCalls:      mockServer.PushCallCount(),
				bytesDelivered: mockServer.PushedBytes(),
			})...)

		mockServer.Close()
	}()

	return outer, nil
}

// pushLoadOutcome is what the run produced that the counters do not hold.
type pushLoadOutcome struct {
	bytesDelivered int64
	accepted       int
	refused        int
	pushCalls      int
	replyOffered   bool
	replyReached   bool
}

// pushLoadSummaryFields builds the push_load_summary line: the run's rates, then
// an echo of every setting that produced them.
func pushLoadSummaryFields(
	load PushLoadConfig,
	counted pushLoadTotals,
	elapsed time.Duration,
	outcome pushLoadOutcome,
) []deps.Field {
	minutes := elapsed.Minutes()

	return []deps.Field{
		deps.Float64("transport_degrade_cycles_per_min", perMinute(counted.transportDegrades, minutes)),
		deps.Float64("push_degrade_cycles_per_min", perMinute(counted.pushDegrades, minutes)),
		deps.Float64("budget_expiries_per_min", perMinute(counted.budgetExpiries, minutes)),
		deps.Float64("resets_per_min", perMinute(counted.resets, minutes)),
		deps.Int("max_pending_dropped", counted.maxPendingDropped),
		deps.Float64("queue_full_duty_pct", percent(counted.queueFullSamples, counted.queueSamples)),
		deps.Int("status_refused", outcome.refused),
		deps.Int("status_accepted", outcome.accepted),
		deps.Bool("reply_reached_console", outcome.replyReached),
		deps.Bool("reply_offered", outcome.replyOffered),

		// The raw event counts behind the four rates, so a rate can be checked
		// against the run's own length rather than taken on trust.
		deps.Int("transport_degrade_events", counted.transportDegrades),
		deps.Int("push_degrade_events", counted.pushDegrades),
		deps.Int("budget_expiry_events", counted.budgetExpiries),
		deps.Int("reset_events", counted.resets),
		deps.Int("queue_samples", counted.queueSamples),
		deps.Int("queue_full_samples", counted.queueFullSamples),
		deps.Float64("elapsed_seconds", round2(elapsed.Seconds())),
		deps.Int("push_calls", outcome.pushCalls),
		deps.Int64("bytes_delivered", outcome.bytesDelivered),

		// Every input, echoed, so one line carries both halves of a comparison.
		deps.Int("bandwidth_bytes_per_second", load.BandwidthBytesPerSecond),
		deps.Int("subscribers", load.Subscribers),
		deps.Int("payload_bytes", load.PayloadBytes),
		deps.Int("topic_count", load.TopicCount),
		deps.Int("queue_capacity", load.QueueCapacity),
		deps.Int("stall_every_nth", load.StallEveryNth),
		deps.Int("stall_burst", load.StallBurst),
		deps.String("stall_for", load.StallFor.String()),
		deps.Duration("http_timeout", load.HTTPTimeout),
		deps.Int("offered_bytes_per_second", load.Subscribers*load.PayloadBytes),
	}
}

// perMinute converts a count over a run into a rate per minute.
func perMinute(count int, minutes float64) float64 {
	if minutes <= 0 {
		return 0
	}

	return round2(float64(count) / minutes)
}

// percent converts a part of a total into a percentage.
func percent(part, total int) float64 {
	if total <= 0 {
		return 0
	}

	return round2(float64(part) / float64(total) * 100)
}

// round2 keeps two decimals, enough to compare two runs without printing noise.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// PushLoadScenarioEntry is the push-load scenario. It carries no load of its
// own: every setting arrives from the CLI through RunConfig.PushLoad, and an
// unset field takes its default from PushLoadConfig.withDefaults.
var PushLoadScenarioEntry = Scenario{
	Name:        "push-load",
	Description: "Watchers pushing status to a bandwidth-limited Console (--bandwidth, --subscribers, --payload-bytes, --topic-count, --queue-capacity, --http-timeout)",
	CustomRunner: func(ctx context.Context, cfg RunConfig) (*RunResult, error) {
		return RunPushLoadScenario(ctx, cfg, cfg.PushLoad)
	},
}
