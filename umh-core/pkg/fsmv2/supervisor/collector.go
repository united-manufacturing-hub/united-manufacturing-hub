// Copyright 2025 UMH Systems GmbH
package supervisor

import (
	"context"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/persistence"
	"go.uber.org/zap"
)

// CollectorConfig provides configuration for observation data collection.
type CollectorConfig struct {
	Worker              fsmv2.Worker
	Identity            fsmv2.Identity
	Store               persistence.Store
	Logger              *zap.SugaredLogger
	ObservationInterval time.Duration
	ObservationTimeout  time.Duration
}

// Collector manages the observation loop lifecycle and data collection.
type Collector struct {
	config        CollectorConfig
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
		restartChan: make(chan struct{}, 1),
	}
}

// Start launches the observation loop in a goroutine.
// The loop runs until the context is cancelled.
func (c *Collector) Start(ctx context.Context) error {
	c.mu.Lock()
	c.parentCtx = ctx
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.goroutineDone = make(chan struct{})
	c.running = true
	c.mu.Unlock()

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
	c.config.Logger.Info("Collector restart requested, collecting immediately")

	select {
	case c.restartChan <- struct{}{}:
		c.config.Logger.Debug("Collector restart signal sent")
	default:
		c.config.Logger.Debug("Collector restart already pending")
	}
}

func (c *Collector) observationLoop() {
	defer func() {
		c.mu.Lock()
		c.running = false
		close(c.goroutineDone)
		c.mu.Unlock()
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
	observed, err := c.config.Worker.CollectObservedState(ctx)
	if err != nil {
		return err
	}

	if err := c.config.Store.SaveObserved(ctx, "container", c.config.Identity.ID, observed); err != nil {
		return err
	}

	return nil
}
