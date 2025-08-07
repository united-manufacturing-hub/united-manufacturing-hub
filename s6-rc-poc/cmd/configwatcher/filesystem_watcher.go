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

package configwatcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	// ErrWatcherAlreadyRunning indicates the watcher is already running
	ErrWatcherAlreadyRunning = errors.New("watcher is already running")
	// ErrConfigPathNotExist indicates the config path does not exist
	ErrConfigPathNotExist = errors.New("config path does not exist")
)

// FilesystemWatcher implements ConfigWatcher using filesystem polling
type FilesystemWatcher struct {
	mu        sync.RWMutex
	running   bool
	services  map[string]*ServiceDetails
	states    map[string]*DesiredState
	eventChan chan Event
	closeChan chan struct{}
}

const eventChannelBuffer = 100

// NewFilesystemWatcher creates a new filesystem-based config watcher.
func NewFilesystemWatcher() *FilesystemWatcher {
	return &FilesystemWatcher{
		mu:        sync.RWMutex{},
		running:   false,
		services:  make(map[string]*ServiceDetails),
		states:    make(map[string]*DesiredState),
		eventChan: make(chan Event, eventChannelBuffer),
		closeChan: make(chan struct{}),
	}
}

// Watch starts watching the configuration directory for changes
func (fw *FilesystemWatcher) Watch(ctx context.Context, configPath string) (<-chan Event, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.running {
		return nil, ErrWatcherAlreadyRunning
	}

	// Validate config path exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrConfigPathNotExist, configPath)
	}

	fw.running = true

	// Load initial state
	if err := fw.loadInitialState(configPath); err != nil {
		fw.running = false

		return nil, fmt.Errorf("failed to load initial state: %w", err)
	}

	// Start the watcher goroutine
	go fw.watchLoop(ctx, configPath)

	return fw.eventChan, nil
}

// Close stops the watcher and releases resources
func (fw *FilesystemWatcher) Close() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if !fw.running {
		return nil
	}

	fw.running = false
	close(fw.closeChan)
	close(fw.eventChan)

	return nil
}

// watchLoop is the main watching loop that polls for changes
func (fw *FilesystemWatcher) watchLoop(ctx context.Context, configPath string) {
	ticker := time.NewTicker(1 * time.Second) // Poll every second
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-fw.closeChan:
			return
		case <-ticker.C:
			fw.checkForChanges(configPath)
		}
	}
}

// loadInitialState loads the current state of services from the filesystem
func (fw *FilesystemWatcher) loadInitialState(configPath string) error {
	entries, err := os.ReadDir(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		serviceName := entry.Name()
		serviceDir := filepath.Join(configPath, serviceName)

		// Load service details
		details, err := fw.loadServiceDetails(serviceDir)
		if err != nil {
			continue // Skip invalid services
		}

		// Load desired state
		state, err := fw.loadDesiredState(serviceDir)
		if err != nil {
			continue // Skip invalid states
		}

		fw.services[serviceName] = details
		fw.states[serviceName] = state

		// Emit Create event for initial services
		fw.sendEvent(Event{
			Type:           Create,
			Name:           serviceName,
			ServiceDetails: details,
			DesiredState:   state,
		})
	}

	return nil
}

// checkForChanges polls the filesystem for changes
func (fw *FilesystemWatcher) checkForChanges(configPath string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if !fw.running {
		return
	}

	currentServices := make(map[string]bool)

	entries, err := os.ReadDir(configPath)
	if err != nil {
		return // Skip this check on error
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		serviceName := entry.Name()
		currentServices[serviceName] = true
		serviceDir := filepath.Join(configPath, serviceName)

		// Check if this is a new service
		if _, exists := fw.services[serviceName]; !exists {
			details, err := fw.loadServiceDetails(serviceDir)
			if err != nil {
				continue
			}

			state, err := fw.loadDesiredState(serviceDir)
			if err != nil {

				continue
			}

			fw.services[serviceName] = details
			fw.states[serviceName] = state

			fw.sendEvent(Event{
				Type:           Create,
				Name:           serviceName,
				ServiceDetails: details,
				DesiredState:   state,
			})
			continue
		}

		// Check for config changes
		details, err := fw.loadServiceDetails(serviceDir)
		if err != nil {
			continue
		}

		if !fw.serviceDetailsEqual(fw.services[serviceName], details) {
			fw.services[serviceName] = details
			fw.sendEvent(Event{
				Type:           ConfigChange,
				Name:           serviceName,
				ServiceDetails: details,
				DesiredState:   nil,
			})
		}

		// Check for state changes
		state, err := fw.loadDesiredState(serviceDir)
		if err != nil {
			continue
		}

		if !fw.desiredStateEqual(fw.states[serviceName], state) {
			fw.states[serviceName] = state
			fw.sendEvent(Event{
				Type:           StateChange,
				Name:           serviceName,
				ServiceDetails: nil,
				DesiredState:   state,
			})
		}
	}

	// Check for deleted services
	for serviceName := range fw.services {
		if !currentServices[serviceName] {
			delete(fw.services, serviceName)
			delete(fw.states, serviceName)

			fw.sendEvent(Event{
				Type:           Delete,
				Name:           serviceName,
				ServiceDetails: nil,
				DesiredState:   nil,
			})
		}
	}
}

// loadServiceDetails loads service configuration from filesystem
func (fw *FilesystemWatcher) loadServiceDetails(serviceDir string) (*ServiceDetails, error) {
	details := &ServiceDetails{
		Type:         "",
		Dependencies: []string{},
		Contents:     []string{},
		Environment:  make(map[string]string),
		RunScript:    "",
	}

	// Read service type
	if typeData, err := os.ReadFile(filepath.Join(serviceDir, "type")); err == nil {
		details.Type = strings.TrimSpace(string(typeData))
	}

	// Read dependencies
	if depDir := filepath.Join(serviceDir, "dependencies.d"); isDirExists(depDir) {
		if entries, err := os.ReadDir(depDir); err == nil {
			for _, entry := range entries {
				details.Dependencies = append(details.Dependencies, entry.Name())
			}
		}
	}

	// Read contents
	if contDir := filepath.Join(serviceDir, "contents.d"); isDirExists(contDir) {
		if entries, err := os.ReadDir(contDir); err == nil {
			for _, entry := range entries {
				details.Contents = append(details.Contents, entry.Name())
			}
		}
	}

	// Read run script
	if runData, err := os.ReadFile(filepath.Join(serviceDir, "run")); err == nil {
		details.RunScript = string(runData)
	}

	return details, nil
}

// loadDesiredState loads the desired state from filesystem (stub implementation)
func (fw *FilesystemWatcher) loadDesiredState(serviceDir string) (*DesiredState, error) {
	// This is a simplified implementation
	// In a real scenario, you'd read this from specific state files
	return &DesiredState{
		Enabled: true,
		Running: false, // Default to not running
	}, nil
}

// sendEvent safely sends an event to the channel
func (fw *FilesystemWatcher) sendEvent(event Event) {
	select {
	case fw.eventChan <- event:
	default:
		// Channel is full, drop the event
		// In production, you might want to log this
	}
}

// serviceDetailsEqual compares two ServiceDetails for equality
func (fw *FilesystemWatcher) serviceDetailsEqual(a, b *ServiceDetails) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if a.Type != b.Type || a.RunScript != b.RunScript {
		return false
	}

	if len(a.Dependencies) != len(b.Dependencies) || len(a.Contents) != len(b.Contents) {
		return false
	}

	// Compare slices
	for i, dep := range a.Dependencies {
		if dep != b.Dependencies[i] {
			return false
		}
	}

	for i, cont := range a.Contents {
		if cont != b.Contents[i] {
			return false
		}
	}

	// Compare environment maps
	if len(a.Environment) != len(b.Environment) {
		return false
	}

	for k, v := range a.Environment {
		if b.Environment[k] != v {
			return false
		}
	}

	return true
}

// desiredStateEqual compares two DesiredState for equality
func (fw *FilesystemWatcher) desiredStateEqual(a, b *DesiredState) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.Enabled == b.Enabled && a.Running == b.Running
}

// isDirExists checks if a directory exists
func isDirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
