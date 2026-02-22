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

package nmap

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"go.uber.org/zap"
)

// tcpCheckInstance holds the state for a single TCP check goroutine.
type tcpCheckInstance struct {
	config     nmapserviceconfig.NmapServiceConfig
	lastResult *NmapScanResult
	running    bool
	cancel     context.CancelFunc
	mu         sync.RWMutex
}

// globalInstances is a package-level shared registry of all TCP check instances.
// This replaces the shared filesystem that the old nmap binary implementation used.
// Multiple NmapService instances (e.g., from ConnectionService in different FSM layers)
// all read/write to this shared map, mimicking the old shared-filesystem behavior.
var (
	globalInstances   = make(map[string]*tcpCheckInstance)
	globalInstancesMu sync.Mutex
)

// NmapService is the implementation of the INmapService interface.
// It replaces nmap binary invocations with in-process net.DialTimeout goroutines.
// All instances share a package-level registry (globalInstances) so that
// configs and status are visible across different NmapService instances.
type NmapService struct {
	logger *zap.SugaredLogger
}

// NmapServiceOption is a function that modifies a NmapService.
type NmapServiceOption func(*NmapService)

// NewDefaultNmapService creates a new default Nmap service manager.
func NewDefaultNmapService(nmapName string, opts ...NmapServiceOption) *NmapService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentNmapService, nmapName)
	service := &NmapService{
		logger: logger.For(managerName),
	}

	for _, opt := range opts {
		opt(service)
	}

	return service
}

// tcpCheck performs a single TCP connectivity check using net.DialTimeout.
// It returns open (connect success), closed (connection refused), or filtered (timeout).
func tcpCheck(target string, port uint16, timeout time.Duration) *NmapScanResult {
	start := time.Now()
	addr := fmt.Sprintf("%s:%d", target, port)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	duration := time.Since(start)

	result := &NmapScanResult{
		Timestamp:  time.Now(),
		PortResult: PortResult{Port: port, LatencyMs: duration.Seconds() * 1000},
		Metrics:    ScanMetrics{ScanDuration: duration.Seconds()},
	}

	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			result.PortResult.State = "filtered"
		} else {
			result.PortResult.State = "closed"
		}
	} else {
		conn.Close()
		result.PortResult.State = "open"
	}

	return result
}

// run starts the background TCP check loop for an instance.
func (inst *tcpCheckInstance) run(ctx context.Context) {
	// Perform an immediate check before starting the ticker
	result := tcpCheck(inst.config.Target, inst.config.Port, 5*time.Second)
	inst.mu.Lock()
	inst.lastResult = result
	inst.mu.Unlock()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result := tcpCheck(inst.config.Target, inst.config.Port, 5*time.Second)
			inst.mu.Lock()
			inst.lastResult = result
			inst.mu.Unlock()
		}
	}
}

// getS6ServiceName converts a nmapName (e.g. "myscan") to its service name (e.g. "nmap-myscan").
func (s *NmapService) getS6ServiceName(nmapName string) string {
	return "nmap-" + nmapName
}

// GenerateS6ConfigForNmap returns an empty S6 config for interface compatibility.
// With the TCP check approach, no S6 service is created.
func (s *NmapService) GenerateS6ConfigForNmap(_ *nmapserviceconfig.NmapServiceConfig, _ string) (s6serviceconfig.S6ServiceConfig, error) {
	return s6serviceconfig.S6ServiceConfig{}, nil
}

// GetConfig returns the nmap config from the shared in-memory instances registry.
func (s *NmapService) GetConfig(_ context.Context, _ filesystem.Service, nmapName string) (nmapserviceconfig.NmapServiceConfig, error) {
	globalInstancesMu.Lock()
	defer globalInstancesMu.Unlock()

	inst, ok := globalInstances[nmapName]
	if !ok {
		return nmapserviceconfig.NmapServiceConfig{}, ErrServiceNotExist
	}

	return inst.config, nil
}

// Status checks the status of a nmap service.
func (s *NmapService) Status(_ context.Context, _ filesystem.Service, nmapName string, _ uint64) (ServiceInfo, error) {
	globalInstancesMu.Lock()
	defer globalInstancesMu.Unlock()

	inst, ok := globalInstances[nmapName]
	if !ok {
		return ServiceInfo{}, ErrServiceNotExist
	}

	inst.mu.RLock()
	defer inst.mu.RUnlock()

	fsmState := s6fsm.OperationalStateStopped
	if inst.running {
		fsmState = s6fsm.OperationalStateRunning
	}

	return ServiceInfo{
		S6FSMState: fsmState,
		NmapStatus: NmapServiceInfo{
			LastScan:  inst.lastResult,
			IsRunning: inst.running,
		},
	}, nil
}

// AddNmapToS6Manager adds a nmap instance and starts its TCP check goroutine.
func (s *NmapService) AddNmapToS6Manager(_ context.Context, cfg *nmapserviceconfig.NmapServiceConfig, nmapName string) error {
	globalInstancesMu.Lock()
	defer globalInstancesMu.Unlock()

	if _, ok := globalInstances[nmapName]; ok {
		return ErrServiceAlreadyExists
	}

	ctx, cancel := context.WithCancel(context.Background())
	inst := &tcpCheckInstance{
		config:  *cfg,
		running: true,
		cancel:  cancel,
	}

	globalInstances[nmapName] = inst
	go inst.run(ctx)

	return nil
}

// UpdateNmapInS6Manager updates an existing nmap instance by restarting its goroutine with the new config.
func (s *NmapService) UpdateNmapInS6Manager(_ context.Context, cfg *nmapserviceconfig.NmapServiceConfig, nmapName string) error {
	globalInstancesMu.Lock()
	defer globalInstancesMu.Unlock()

	inst, ok := globalInstances[nmapName]
	if !ok {
		return ErrServiceNotExist
	}

	// Stop old goroutine
	if inst.cancel != nil {
		inst.cancel()
	}

	// Start new goroutine with updated config
	ctx, cancel := context.WithCancel(context.Background())
	inst.config = *cfg
	inst.cancel = cancel
	inst.running = true

	go inst.run(ctx)

	return nil
}

// RemoveNmapFromS6Manager removes a nmap instance and stops its goroutine.
func (s *NmapService) RemoveNmapFromS6Manager(_ context.Context, nmapName string) error {
	globalInstancesMu.Lock()
	defer globalInstancesMu.Unlock()

	inst, ok := globalInstances[nmapName]
	if ok {
		if inst.cancel != nil {
			inst.cancel()
		}

		delete(globalInstances, nmapName)
	}

	return nil
}

// ForceRemoveNmap is the same as Remove for the TCP check implementation.
func (s *NmapService) ForceRemoveNmap(_ context.Context, _ filesystem.Service, nmapName string) error {
	return s.RemoveNmapFromS6Manager(context.Background(), nmapName)
}

// StartNmap starts the TCP check goroutine for a nmap instance.
func (s *NmapService) StartNmap(_ context.Context, nmapName string) error {
	globalInstancesMu.Lock()
	defer globalInstancesMu.Unlock()

	inst, ok := globalInstances[nmapName]
	if !ok {
		return ErrServiceNotExist
	}

	if inst.running {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	inst.cancel = cancel
	inst.running = true

	go inst.run(ctx)

	return nil
}

// StopNmap stops the TCP check goroutine for a nmap instance.
func (s *NmapService) StopNmap(_ context.Context, nmapName string) error {
	globalInstancesMu.Lock()
	defer globalInstancesMu.Unlock()

	inst, ok := globalInstances[nmapName]
	if !ok {
		return ErrServiceNotExist
	}

	if !inst.running {
		return nil
	}

	if inst.cancel != nil {
		inst.cancel()
	}

	inst.running = false

	return nil
}

// ReconcileManager is a no-op for the TCP check implementation.
// There is no S6 manager to reconcile.
func (s *NmapService) ReconcileManager(_ context.Context, _ serviceregistry.Provider, _ fsm.SystemSnapshot) (error, bool) {
	return nil, false
}

// ServiceExists checks if a nmap service exists in the shared instances registry.
func (s *NmapService) ServiceExists(_ context.Context, _ filesystem.Service, nmapName string) bool {
	globalInstancesMu.Lock()
	defer globalInstancesMu.Unlock()

	_, ok := globalInstances[nmapName]
	return ok
}
