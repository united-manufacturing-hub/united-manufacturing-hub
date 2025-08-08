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
	"fmt"
	"log"
	"os"
	"s6-rc-poc/cmd/shared"
	"sync"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Service represents a service configuration.
type Service struct {
	Name         shared.ServiceName `yaml:"name"`
	DesiredState string             `yaml:"desired_state"` //nolint:tagliatelle // External API format
	Executable   string             `yaml:"executable"`
	Parameters   map[int]string     `yaml:"parameters"`
}

// Config represents the complete configuration.
type Config struct {
	Services []Service `yaml:"services"`
}

// FileWatcher monitors configuration file changes.
type FileWatcher struct {
	watcher      *fsnotify.Watcher
	events       chan ConfigEvent
	currentState map[shared.ServiceName]Service
	configPath   string
	mu           sync.RWMutex
	stopCh       chan struct{}
	logger       *zap.Logger
}

const eventBufferSize = 100

// NewFileWatcher creates a new file watcher instance.
func NewFileWatcher(logger *zap.Logger) *FileWatcher {
	return &FileWatcher{
		watcher:      nil,
		events:       make(chan ConfigEvent, eventBufferSize),
		currentState: make(map[shared.ServiceName]Service),
		configPath:   "",
		mu:           sync.RWMutex{},
		stopCh:       make(chan struct{}),
		logger:       logger,
	}
}

// Start begins watching the configuration file.
func (fw *FileWatcher) Start(configPath string) error {
	var err error

	fw.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	fw.configPath = configPath

	// Load initial config
	if err := fw.loadAndCompareConfig(); err != nil {
		return fmt.Errorf("failed to load initial config: %w", err)
	}

	// Watch the config file
	if err := fw.watcher.Add(configPath); err != nil {
		return fmt.Errorf("failed to watch config file: %w", err)
	}

	go fw.watchLoop()

	return nil
}

// Stop stops watching the configuration file.
func (fw *FileWatcher) Stop() error {
	close(fw.stopCh)

	if fw.watcher != nil {
		if err := fw.watcher.Close(); err != nil {
			return fmt.Errorf("failed to close watcher: %w", err)
		}
	}

	if fw.logger != nil {
		_ = fw.logger.Sync()
	}

	return nil
}

// Events returns the channel for configuration events.
func (fw *FileWatcher) Events() chan ConfigEvent {
	return fw.events
}

// emitEvent sends an event to the channel and logs it to stdout.
func (fw *FileWatcher) emitEvent(event ConfigEvent) {
	fw.events <- event

	if fw.logger == nil {
		return
	}

	switch eventTyped := event.(type) {
	case EventCreated:
		fw.logger.Info("Service created",
			zap.String("service", eventTyped.Name.String()),
			zap.String("event", "created"),
			zap.String("desired_state", eventTyped.DesiredState.String()),
			zap.String("executable", eventTyped.Executable),
		)
	case EventDeleted:
		fw.logger.Info("Service deleted",
			zap.String("service", eventTyped.Name.String()),
			zap.String("event", "deleted"),
		)
	case EventStateChanged:
		fw.logger.Info("Service state changed",
			zap.String("service", eventTyped.Name.String()),
			zap.String("event", "state_changed"),
			zap.String("desired_state", eventTyped.DesiredState.String()),
		)
	case EventConfigChanged:
		fw.logger.Info("Service config changed",
			zap.String("service", eventTyped.Name.String()),
			zap.String("event", "config_changed"),
			zap.String("executable", eventTyped.Executable),
		)
	}
}

func (fw *FileWatcher) watchLoop() {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			if event.Has(fsnotify.Write) {
				if err := fw.loadAndCompareConfig(); err != nil {
					log.Printf("Error processing config change: %v", err)
				}
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}

			log.Printf("Watcher error: %v", err)
		case <-fw.stopCh:
			return
		}
	}
}

func (fw *FileWatcher) loadAndCompareConfig() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	data, err := os.ReadFile(fw.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	newState := make(map[shared.ServiceName]Service)
	for _, service := range config.Services {
		newState[service.Name] = service
	}

	fw.compareAndEmitEvents(newState)
	fw.currentState = newState

	return nil
}

func (fw *FileWatcher) compareAndEmitEvents(newState map[shared.ServiceName]Service) {
	// Check for deleted services
	for name := range fw.currentState {
		if _, exists := newState[name]; !exists {
			fw.emitEvent(EventDeleted{Name: name})
		}
	}

	// Check for new and changed services
	for name, newService := range newState {
		oldService, exists := fw.currentState[name]
		if !exists {
			// New service
			fw.emitEvent(EventCreated{
				Name:         newService.Name,
				DesiredState: shared.FromStateString(newService.DesiredState),
				Executable:   newService.Executable,
				Parameters:   newService.Parameters,
			})
		} else {
			// Check for state changes
			if oldService.DesiredState != newService.DesiredState {
				fw.emitEvent(EventStateChanged{
					Name:         newService.Name,
					DesiredState: shared.FromStateString(newService.DesiredState),
				})
			}
			// Check for config changes (executable or parameters)
			if oldService.Executable != newService.Executable || !EqualMaps(oldService.Parameters, newService.Parameters) {
				fw.emitEvent(EventConfigChanged{
					Name:       newService.Name,
					Executable: newService.Executable,
					Parameters: newService.Parameters,
				})
			}
		}
	}
}

// EqualMaps compares two maps for equality.
func EqualMaps(first, second map[int]string) bool {
	if len(first) != len(second) {
		return false
	}

	for k, v := range first {
		if second[k] != v {
			return false
		}
	}

	return true
}
