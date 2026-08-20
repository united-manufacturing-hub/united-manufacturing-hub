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
	"strings"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// Payload size of a status message carrying N topics: statusEnvelopeBytes for
// everything that is not a topic, plus statusBytesPerTopic per topic. Both are
// measured from real status messages and are orders of magnitude - about ten
// kilobytes of envelope, a couple of hundred bytes per topic - not limits.
const (
	statusEnvelopeBytes = 8900
	statusBytesPerTopic = 190
)

// defaultPayloadBytes applies when the caller sets neither TopicCount nor PayloadBytes.
const defaultPayloadBytes = 1024

// TransportLoadConfig describes the load offered to the transport worker and
// the uplink it runs against.
type TransportLoadConfig struct {
	Subscribers             int           // independent streams, each offering one message per second
	TopicCount              int           // when non-zero, derives the payload size
	PayloadBytes            int           // payload size when TopicCount is zero
	BurstBytes              int           // one oversized message on top of the streams
	BurstEvery              time.Duration // 0 means send the burst once
	BandwidthBytesPerSecond int           // 0 means unthrottled
}

// TransportLoadScenarioEntry registers the transport-load scenario for CLI access.
//
// The worker runs the configuration it ships with; only the offered load and the
// uplink rate vary. Set RunConfig.TransportLoad to choose them. A CLI run gets the
// zero value: one stream of 1 KiB messages against an unthrottled server.
var TransportLoadScenarioEntry = Scenario{
	Name:        "transport-load",
	Description: "Tests FSMv2 transport worker under offered load against a bandwidth-limited mock server",
	CustomRunner: func(ctx context.Context, cfg RunConfig) (*RunResult, error) {
		result := RunTransportLoadScenario(ctx, TransportRunConfig{
			Duration:     cfg.Duration,
			TickInterval: cfg.TickInterval,
			Logger:       cfg.Logger,
		}, cfg.TransportLoad)

		if result.Error != nil {
			return nil, result.Error
		}

		done := make(chan struct{})
		wrapped := &RunResult{Done: done, Shutdown: result.Shutdown}

		go func() {
			<-result.Done
			wrapped.ShutdownClean = result.ShutdownClean
			close(done)
		}()

		return wrapped, nil
	},
}

// RunTransportLoadScenario runs the FSMv2 transport worker under offered load.
//
// Messages are queued onto the live outbound channel while the run is in progress,
// never seeded before it starts, so the worker keeps the outbound capacity it ships
// with and the streams keep the queue near that capacity.
//
// A caller that supplies cfg.MockServer owns it and its uplink is left untouched,
// so BandwidthBytesPerSecond then has no effect.
func RunTransportLoadScenario(ctx context.Context, cfg TransportRunConfig, load TransportLoadConfig) *TransportRunResult {
	subscribers := load.Subscribers
	if subscribers == 0 {
		subscribers = 1
	}

	payloadBytes := load.PayloadBytes

	switch {
	case load.TopicCount > 0:
		payloadBytes = statusEnvelopeBytes + load.TopicCount*statusBytesPerTopic
	case payloadBytes == 0:
		payloadBytes = defaultPayloadBytes
	}

	cleanup := func() {}

	if cfg.MockServer == nil {
		server := testutil.NewMockRelayServer()
		server.SimulateBandwidthLimitation(load.BandwidthBytesPerSecond)
		cfg.MockServer = server
		cleanup = server.Close
	}

	logger := cfg.Logger
	if logger == nil {
		logger = deps.NewNopFSMLogger()
	}

	logger.Info("transport_load_params",
		deps.Int("subscribers", subscribers),
		deps.Int("payload_bytes", payloadBytes),
		deps.Int("topic_count", load.TopicCount),
		deps.Int("burst_bytes", load.BurstBytes),
		deps.Duration("burst_every", load.BurstEvery),
		deps.Int("bandwidth_bytes_per_second", load.BandwidthBytesPerSecond),
	)

	result := RunTransportScenario(ctx, cfg)

	if result.Error != nil {
		cleanup()

		return result
	}

	loadCtx, cancelLoad := context.WithCancel(ctx)

	var streams sync.WaitGroup

	for range subscribers {
		streams.Add(1)

		go func() {
			defer streams.Done()

			offerStream(loadCtx, result.ChannelProvider, time.Second, payloadBytes)
		}()
	}

	if load.BurstBytes > 0 {
		streams.Add(1)

		go func() {
			defer streams.Done()

			if load.BurstEvery == 0 {
				offerMessage(result.ChannelProvider, load.BurstBytes)
			} else {
				offerStream(loadCtx, result.ChannelProvider, load.BurstEvery, load.BurstBytes)
			}
		}()
	}

	go stopLoad(result, cancelLoad, &streams, cleanup)

	return result
}

// stopLoad ends the offered load once the run completes.
//
// After the run ends nothing reads the outbound channel any more, so a stream
// parked on a full queue would never return. Draining the channel until every
// stream has stopped releases them, which is what leaves no goroutine behind.
func stopLoad(result *TransportRunResult, cancelLoad context.CancelFunc, streams *sync.WaitGroup, cleanup func()) {
	<-result.Done

	cancelLoad()

	stopped := make(chan struct{})

	go func() {
		streams.Wait()
		close(stopped)
	}()

	_, outbound := result.ChannelProvider.GetChannels("")

	for {
		select {
		case <-outbound:
		case <-stopped:
			cleanup()

			return
		}
	}
}

// offerMessage queues one message of the given payload size. It blocks while the
// outbound queue is full, which is the backpressure under test.
func offerMessage(provider *TransportTestChannelProvider, payloadBytes int) {
	provider.QueueOutbound(&types.UMHMessage{
		InstanceUUID: "test-instance-uuid",
		Content:      strings.Repeat("x", payloadBytes),
	})
}

// offerStream queues one message of the given payload size every interval, until ctx ends.
func offerStream(ctx context.Context, provider *TransportTestChannelProvider, interval time.Duration, payloadBytes int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			offerMessage(provider, payloadBytes)
		}
	}
}
