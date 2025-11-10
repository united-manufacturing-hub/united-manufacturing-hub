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

package collection

import (
	"context"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"go.uber.org/zap"
)

type collectorState int

const (
	collectorStateCreated collectorState = iota
	collectorStateRunning
	collectorStateStopped
)

// CollectorConfig provides configuration for observation data collection.
type CollectorConfig struct {
	Worker              fsmv2.Worker
	Identity            fsmv2.Identity
	Store               storage.TriangularStoreInterface
	Logger              *zap.SugaredLogger
	ObservationInterval time.Duration
	ObservationTimeout  time.Duration
	WorkerType          string
}

// Collector manages the observation loop lifecycle and data collection.
type Collector struct {
	config        CollectorConfig
	state         collectorState
	running       bool
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	goroutineDone chan struct{}
	parentCtx     context.Context
	restartChan   chan struct{}
}

// NewCollector creates a new collector with the given configuration.
func NewCollector(config CollectorConfig) *Collector {
	return &Collector{
		config:      config,
		state:       collectorStateCreated,
		restartChan: make(chan struct{}, 1),
	}
}

// Start launches the observation loop in a goroutine.
// The loop runs until the context is cancelled.
func (c *Collector) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == collectorStateRunning {
		panic("Invariant I8 violated: collector already started. Collector.Start() must not be called twice. Check lifecycle management in supervisor code.")
	}

	c.config.Logger.Infof("Starting collector, transitioning from state %d to running", c.state)

	c.state = collectorStateRunning
	c.parentCtx = ctx
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.goroutineDone = make(chan struct{})
	c.running = true

	go c.observationLoop()

	return nil
}

// IsRunning returns true if the observation loop is currently active.
func (c *Collector) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.running
}

// Restart signals the observation loop to collect immediately.
func (c *Collector) Restart() {
	c.mu.RLock()
	running := c.state == collectorStateRunning
	c.mu.RUnlock()

	if !running {
		c.config.Logger.Errorf("Cannot restart collector: not running (current state: %d)", c.state)

		return
	}

	c.config.Logger.Info("Collector restart requested, collecting immediately")

	select {
	case c.restartChan <- struct{}{}:
		c.config.Logger.Debug("Collector restart signal sent")
	default:
		c.config.Logger.Debug("Collector restart already pending")
	}
}

func (c *Collector) Stop(ctx context.Context) {
	c.mu.Lock()

	if c.state != collectorStateRunning {
		c.config.Logger.Warnf("Collector not running, cannot stop (current state: %d)", c.state)
		c.mu.Unlock()

		return
	}

	c.config.Logger.Info("Stopping collector")
	c.cancel()
	doneChan := c.goroutineDone
	c.mu.Unlock()

	select {
	case <-doneChan:
		c.config.Logger.Info("Collector stopped successfully")
	case <-ctx.Done():
		c.config.Logger.Warn("Context cancelled while waiting for collector to stop")
	case <-time.After(5 * time.Second):
		c.config.Logger.Error("Timeout waiting for collector to stop")
	}
}

func (c *Collector) observationLoop() {
	defer func() {
		c.mu.Lock()
		c.state = collectorStateStopped
		c.running = false
		close(c.goroutineDone)
		c.mu.Unlock()
		c.config.Logger.Info("Collector observation loop stopped, state set to stopped")
	}()

	c.mu.RLock()
	ctx := c.ctx
	interval := c.config.ObservationInterval
	timeout := c.config.ObservationTimeout
	c.mu.RUnlock()

	c.config.Logger.Infof("Starting observation loop for worker %s", c.config.Identity.ID)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.config.Logger.Infof("Observation loop stopped for worker %s", c.config.Identity.ID)

			return

		case <-c.restartChan:
			c.config.Logger.Info("Collector restart requested, collecting immediately")

			collectCtx, cancel := context.WithTimeout(ctx, timeout)
			if err := c.collectAndSaveObservedState(collectCtx); err != nil {
				c.config.Logger.Errorf("Failed to collect observed state after restart: %v", err)
			}

			cancel()

		case <-ticker.C:
			collectCtx, cancel := context.WithTimeout(ctx, timeout)
			if err := c.collectAndSaveObservedState(collectCtx); err != nil {
				c.config.Logger.Errorf("Failed to collect observed state: %v", err)
			}

			cancel()
		}
	}
}

func (c *Collector) collectAndSaveObservedState(ctx context.Context) error {
	collectionStartTime := time.Now()
	c.config.Logger.Debugf("[DataFreshness] Worker %s: Starting observation collection at %s",
		c.config.Identity.ID, collectionStartTime.Format(time.RFC3339Nano))

	observed, err := c.config.Worker.CollectObservedState(ctx)
	if err != nil {
		c.config.Logger.Debugf("[DataFreshness] Worker %s: Failed to collect observation: %v", c.config.Identity.ID, err)

		return err
	}

	// Extract and log observation timestamp
	var observationTimestamp time.Time
	if timestampProvider, ok := observed.(interface{ GetTimestamp() time.Time }); ok {
		observationTimestamp = timestampProvider.GetTimestamp()
		c.config.Logger.Debugf("[DataFreshness] Worker %s: Collected observation with timestamp=%s",
			c.config.Identity.ID, observationTimestamp.Format(time.RFC3339Nano))
	} else {
		c.config.Logger.Debugf("[DataFreshness] Worker %s: Collected observation does not implement GetTimestamp() (type: %T)",
			c.config.Identity.ID, observed)
	}

	saveStartTime := time.Now()

	if err := c.config.Store.SaveObserved(ctx, c.config.WorkerType, c.config.Identity.ID, observed); err != nil {
		c.config.Logger.Debugf("[DataFreshness] Worker %s: Failed to save observation: %v", c.config.Identity.ID, err)

		return err
	}

	saveDuration := time.Since(saveStartTime)
	c.config.Logger.Debugf("[DataFreshness] Worker %s: Successfully saved observation (save_duration=%v)",
		c.config.Identity.ID, saveDuration)

	return nil
}
