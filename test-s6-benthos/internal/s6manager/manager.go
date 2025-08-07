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

package s6manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"test-s6-benthos/internal/testservice"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Manager manages multiple s6 services for testing
type Manager struct {
	s6Service *testservice.S6Service
	fsService *testservice.FilesystemService
	logger    *zap.SugaredLogger
	services  map[string]*ServiceInfo
}

// ServiceInfo tracks information about a managed service
type ServiceInfo struct {
	Name        string
	ServicePath string
	Config      testservice.S6ServiceConfig
	StartTime   time.Time
	Status      testservice.ServiceStatus
}

// NewManager creates a new service manager
func NewManager(s6Service *testservice.S6Service, fsService *testservice.FilesystemService, logger *zap.SugaredLogger) *Manager {
	return &Manager{
		s6Service: s6Service,
		fsService: fsService,
		logger:    logger,
		services:  make(map[string]*ServiceInfo),
	}
}

// RunBenthos creates and runs a Benthos instance via s6
func (m *Manager) RunBenthos(ctx context.Context, serviceName string, httpPort int) error {
	//m.logger.Infof("Starting Benthos service %s on port %d", serviceName, httpPort)

	// Create service path
	servicePath := filepath.Join(testservice.S6RepositoryBaseDir, serviceName)

	// Generate Benthos configuration
	benthosConfig := m.generateBenthosConfig(httpPort)
	configYAML, err := yaml.Marshal(benthosConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal benthos config: %w", err)
	}

	// Create s6 service configuration
	s6Config := testservice.S6ServiceConfig{
		Command: []string{
			"/usr/local/bin/benthos",
			"-c",
			fmt.Sprintf("%s/config/benthos.yaml", servicePath),
		},
		Env: map[string]string{
			"BENTHOS_LOG_LEVEL": "INFO",
		},
		ConfigFiles: map[string]string{
			"benthos.yaml": string(configYAML),
		},
		MemoryLimit: 0,       // No memory limit
		LogFilesize: 1048576, // 1MB log files
	}

	// Create the service with retry
	for {
		if err := m.s6Service.Create(ctx, servicePath, s6Config, m.fsService); err != nil {
			m.logger.Warnf("Failed to create s6 service %s (retrying): %v", serviceName, err)
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled while creating s6 service: %w", ctx.Err())
			case <-time.After(1 * time.Second):
				continue
			}
		}
		break
	}

	// Start the service with retry
	for {
		if err := m.s6Service.Start(ctx, servicePath, m.fsService); err != nil {
			m.logger.Warnf("Failed to start s6 service %s (retrying): %v", serviceName, err)
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled while starting s6 service: %w", ctx.Err())
			case <-time.After(1 * time.Second):
				continue
			}
		}
		break
	}

	// Track the service
	m.services[serviceName] = &ServiceInfo{
		Name:        serviceName,
		ServicePath: servicePath,
		Config:      s6Config,
		StartTime:   time.Now(),
		Status:      testservice.ServiceUp,
	}

	// Wait and monitor the service
	return nil
}

// monitorService monitors a service until context is cancelled
func (m *Manager) monitorService(ctx context.Context, serviceName string) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	serviceInfo, exists := m.services[serviceName]
	if !exists {
		return fmt.Errorf("service %s not found", serviceName)
	}

	for {
		select {
		case <-ctx.Done():
			m.logger.Infof("Context cancelled, stopping service %s", serviceName)
			return m.stopService(context.Background(), serviceName)
		case <-ticker.C:
			// Check service status
			status, err := m.s6Service.Status(ctx, serviceInfo.ServicePath, m.fsService)
			if err != nil {
				m.logger.Warnf("Failed to get status for service %s: %v", serviceName, err)
				continue
			}

			serviceInfo.Status = status.Status
			m.logger.Debugf("Service %s status: %s", serviceName, status.Status)

			// If service is down unexpectedly, try to restart it
			if status.Status == testservice.ServiceDown {
				m.logger.Warnf("Service %s is down, attempting restart", serviceName)
				if err := m.s6Service.Start(ctx, serviceInfo.ServicePath, m.fsService); err != nil {
					m.logger.Errorf("Failed to restart service %s: %v", serviceName, err)
				}
			}
		}
	}
}

// stopService stops a service
func (m *Manager) stopService(ctx context.Context, serviceName string) error {
	serviceInfo, exists := m.services[serviceName]
	if !exists {
		return fmt.Errorf("service %s not found", serviceName)
	}

	m.logger.Infof("Stopping service %s", serviceName)

	// Stop the service
	if err := m.s6Service.Stop(ctx, serviceInfo.ServicePath, m.fsService); err != nil {
		m.logger.Warnf("Failed to stop service %s: %v", serviceName, err)
	}

	// Remove the service
	if err := m.s6Service.Remove(ctx, serviceInfo.ServicePath, m.fsService); err != nil {
		m.logger.Warnf("Failed to remove service %s: %v", serviceName, err)
	}

	delete(m.services, serviceName)
	return nil
}

// LogServiceStatus logs the number of active benthos instances and memory usage
func (m *Manager) LogServiceStatus(ctx context.Context) {
	// Count active services
	activeServices := 0
	for _, info := range m.services {
		if info.Status == testservice.ServiceUp {
			activeServices++
		}
	}

	// Get container memory stats
	memUsedMB, memLimitMB, memPercent := m.getContainerMemoryStats()
	freeMemMB := memLimitMB - memUsedMB
	freeMemPercent := 100.0 - memPercent

	m.logger.Infof("Active benthos instances: %d | Free memory: %d MB (%.1f%%)",
		activeServices, freeMemMB, freeMemPercent)
}

// getContainerMemoryStats reads memory stats from cgroup files
func (m *Manager) getContainerMemoryStats() (usedMB, limitMB int64, usedPercent float64) {
	// Try cgroup v2 first, then v1
	memUsage := m.readMemoryUsage()
	memLimit := m.readMemoryLimit()

	if memUsage == 0 || memLimit == 0 {
		// Fallback: use reasonable defaults if we can't read cgroup info
		return 0, 1024, 0.0 // 1GB default
	}

	usedMB = memUsage / 1024 / 1024
	limitMB = memLimit / 1024 / 1024
	usedPercent = float64(memUsage) / float64(memLimit) * 100

	return usedMB, limitMB, usedPercent
}

// readMemoryUsage reads current memory usage from cgroup
func (m *Manager) readMemoryUsage() int64 {
	// Try cgroup v2
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.current"); err == nil {
		if val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
			return val
		}
	}

	// Try cgroup v1
	if data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.usage_in_bytes"); err == nil {
		if val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
			return val
		}
	}

	return 0
}

// readMemoryLimit reads memory limit from cgroup
func (m *Manager) readMemoryLimit() int64 {
	// Try cgroup v2
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		limitStr := strings.TrimSpace(string(data))
		if limitStr != "max" {
			if val, err := strconv.ParseInt(limitStr, 10, 64); err == nil {
				return val
			}
		}
	}

	// Try cgroup v1
	if data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		if val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
			// Check if it's a reasonable limit (not the default huge value)
			if val < (1 << 62) {
				return val
			}
		}
	}

	return 0
}

// generateBenthosConfig generates a Benthos configuration for testing
func (m *Manager) generateBenthosConfig(httpPort int) map[string]interface{} {
	return map[string]interface{}{
		"input": map[string]interface{}{
			"http_server": map[string]interface{}{
				"path":    "/",
				"address": fmt.Sprintf("0.0.0.0:%d", httpPort),
			},
		},
		"pipeline": map[string]interface{}{
			"processors": []map[string]interface{}{
				{
					"mapping": `root.message = "Hello from Benthos!"
root.timestamp = now()
root.service = "${HOSTNAME}"`,
				},
			},
		},
		"output": map[string]interface{}{
			"stdout": map[string]interface{}{},
		},
		"logger": map[string]interface{}{
			"level": "INFO",
		},
		"metrics": map[string]interface{}{
			"http_server": map[string]interface{}{
				"address": fmt.Sprintf("0.0.0.0:%d", httpPort+1000),
			},
		},
	}
}
