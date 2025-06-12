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

package topicbrowser

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	benthossvccfg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/monitor"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
	"go.uber.org/zap"
)

type ITopicBrowserService interface {
	// GenerateConfig generates a topic browser config
	GenerateConfig(tbName string) (s6serviceconfig.S6ServiceConfig, error)
	// Status returns information about the topic browser health
	Status(ctx context.Context, services serviceregistry.Provider, tick uint64) (ServiceInfo, error)
	// AddToManager registers a new topic browser
	AddToManager(ctx context.Context) error
	// UpdateInManager modifies an existing topic browser configuration.
	UpdateInManager(ctx context.Context) error
	// RemoveFromManager deletes a topic browser configuration.
	RemoveFromManager(ctx context.Context) error
	// Start begins the monitoring of a topic browser.
	Start(ctx context.Context) error
	// Stop stops the monitoring of a topic browser.
	Stop(ctx context.Context) error
	// ForceRemove removes a topic browser instance
	ForceRemove(ctx context.Context, services serviceregistry.Provider) error
	// ServiceExists checks if a topic browser with the given name exists.
	ServiceExists(ctx context.Context, services serviceregistry.Provider) bool
	// ReconcileManager synchronizes all connections on each tick.
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool)
}

type Status struct {
	Buffer []*Buffer
	Logs   []s6svc.LogEntry // contain the structured s6 logs entries
}

// CopyLogs is a go-deepcopy override for the Logs field.
//
// go-deepcopy looks for a method with the signature
//
//	func (dst *T) Copy<FieldName>(src <FieldType>) error
func (st *Status) CopyLogs(src []s6svc.LogEntry) error {
	st.Logs = src
	return nil
}

type ServiceInfo struct {
	// S6 state information
	S6ObservedState s6fsm.S6ObservedState
	S6FSMState      string

	// topic browser status
	Status Status
}

// Service implements ITopicBrowserService
type Service struct {
	logger          *zap.SugaredLogger
	s6Manager       *s6fsm.S6Manager
	s6Service       s6svc.Service
	s6ServiceConfig *config.S6FSMConfig // There can only be one monitor per instance

	benthosManager *benthosfsm.BenthosManager
	benthosService benthossvc.IBenthosService
	benthosConfigs []config.BenthosConfig

	tbName     string // normally a service can handle multiple instances, the service monitor here is different and can only handle one instance
	ringbuffer *Ringbuffer
}

// ServiceOption is a function that configures a Service.
type ServiceOption func(*Service)

func WithService(s6svc s6svc.Service, benthossvc benthossvc.IBenthosService) ServiceOption {
	return func(s *Service) {
		s.s6Service = s6svc
		s.benthosService = benthossvc
	}
}

func WithManager(s6mgr *s6fsm.S6Manager, benthosmgr *benthosfsm.BenthosManager) ServiceOption {
	return func(s *Service) {
		s.s6Manager = s6mgr
		s.benthosManager = benthosmgr
	}
}

// NewDefaultService creates a new topic browser service with default options.
// It initializes the logger and sets default values.
func NewDefaultService(tbName string, opts ...ServiceOption) *Service {
	managerName := fmt.Sprintf("%s%s", logger.ComponentTopicBrowserService, tbName)
	service := &Service{
		logger:         logger.For(managerName),
		s6Manager:      s6fsm.NewS6Manager(logger.ComponentTopicBrowserService),
		s6Service:      s6svc.NewDefaultService(),
		benthosManager: benthosfsm.NewBenthosManager(managerName), // should reuse existing?
		benthosService: benthossvc.NewDefaultBenthosService(tbName),
		benthosConfigs: []config.BenthosConfig{},
		tbName:         tbName,
		ringbuffer:     NewRingbuffer(8),
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// getName converts the tbName (e.g. "myservice") that was given during NewService to its benthos service name (e.g. "topicbrowser-myservice")
func (svc *Service) getName() string {
	return fmt.Sprintf("topicbrowser-%s", svc.tbName)
}

func (svc *Service) GenerateConfig(tbName string) (benthossvccfg.BenthosServiceConfig, error) {
	return benthossvccfg.BenthosServiceConfig{
		Input: map[string]any{
			"input:": map[string]any{
				"uns:": map[string]any{
					"umh_topic:":      "umh.v1.*",
					"kafka_topic:":    "umh.messages",
					"broker_address:": "localhost:9092",
					"consumer_group:": "benthos_kafka_test",
				},
			},
		},
		Pipeline: map[string]any{
			"processors:": map[string]any{
				"topic-browser:": "{}",
			},
		},
		Output: map[string]any{
			"output:": map[string]any{
				"stdout:": "{}",
			},
		},
		LogLevel: constants.DefaultBenthosLogLevel,
	}, nil
}

// Status returns information about the connection health for the specified topic browser.
func (svc *Service) Status(
	ctx context.Context,
	services serviceregistry.Provider,
	tick uint64,
) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	s6SvcName := svc.getName()

	// First, check if the service exists in the S6 manager
	if _, exists := svc.s6Manager.GetInstance(s6SvcName); exists {
		return ServiceInfo{}, ErrServiceNotExist
	}
	// Get S6 state
	s6StateRaw, err := svc.s6Manager.GetLastObservedState(s6SvcName)
	if err != nil {
		if strings.Contains(err.Error(), "instance "+s6SvcName+" not found") ||
			strings.Contains(err.Error(), "not found") {
			return ServiceInfo{}, ErrServiceNotExist
		}
		return ServiceInfo{}, fmt.Errorf("failed to get S6 state: %w", err)
	}

	s6State, ok := s6StateRaw.(s6fsm.S6ObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("observed state is not a S6ObservedState: %v", s6StateRaw)
	}

	// Get FSM state
	fsmState, err := svc.s6Manager.GetCurrentFSMState(s6SvcName)
	if err != nil {
		if strings.Contains(err.Error(), "instance "+s6SvcName+" not found") ||
			strings.Contains(err.Error(), "not found") {
			return ServiceInfo{}, ErrServiceNotExist
		}
		return ServiceInfo{}, fmt.Errorf("failed to get current FSM state: %w", err)
	}

	// If the current state is stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if fsmState == s6fsm.OperationalStateStopped || fsmState == s6fsm.OperationalStateStopping {
		return ServiceInfo{}, monitor.ErrServiceStopped
	}

	// Get logs
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6SvcName)
	logs, err := svc.s6Service.GetLogs(ctx, s6ServicePath, services.GetFileSystem())
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get logs: %w", err)
	}

	if len(logs) == 0 {
		return ServiceInfo{}, ErrServiceNoLogFile
	}

	// Parse the logs
	err = svc.parseBlock(logs)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to parse block from logs: %w", err)
	}

	// no direct reference
	return ServiceInfo{
		S6ObservedState: s6State,
		S6FSMState:      fsmState,
		Status: Status{
			Buffer: svc.ringbuffer.Get(),
			Logs:   logs,
		},
	}, nil
}

// AddToManager registers a new topic browser to the S6 manager.
func (svc *Service) AddToManager(
	ctx context.Context,
	services serviceregistry.Provider,
	cfg *benthossvccfg.BenthosServiceConfig,
) error {
	if svc.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	svc.logger.Debugf("adding topic browser to benthos manager")

	benthosName := svc.getName()

	// Check whether benthosConfig already contains an entry for this instance
	for _, config := range svc.benthosConfigs {
		if config.Name == benthosName {
			return ErrServiceAlreadyExists
		}
	}

	// Generate the benthos config for this instance
	benthosCfg, err := svc.GenerateConfig(benthosName)
	if err != nil {
		return fmt.Errorf("failed to generate benthos config for topic browser service %s: %w", benthosName, err)
	}

	// Create the benthos FSM config for this instance
	benthosFSMConfig := config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            benthosName,
			DesiredFSMState: benthosfsm.OperationalStateStopped,
		},
		BenthosServiceConfig: benthosCfg,
	}

	// add the benthos FSM config
	svc.benthosConfigs = append(svc.benthosConfigs, benthosFSMConfig)

	return nil
}

// UpdateInManager modifies an existing topic browser configuration.
func (svc *Service) UpdateInManager(
	ctx context.Context,
	cfg *benthossvccfg.BenthosServiceConfig,
) error {
	if svc.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	benthosName := svc.getName()

	found := false
	index := -1
	for i, config := range svc.benthosConfigs {
		if config.Name == benthosName {
			found = true
			index = i
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	// Generate the S6 config for this instance
	benthosCfg, err := svc.GenerateConfig(benthosName)
	if err != nil {
		return fmt.Errorf("failed to generate benthos config for topic browser service %s: %w", benthosName, err)
	}

	currentDesiredState := svc.benthosConfigs[index].DesiredFSMState
	svc.benthosConfigs[index] = config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            benthosName,
			DesiredFSMState: currentDesiredState,
		},
		BenthosServiceConfig: benthosCfg,
	}

	return nil
}

// RemoveFromManager deletes a topic browser configuration.
// This stops monitoring the topic browser and removes all configuration.
func (svc *Service) RemoveFromManager(
	ctx context.Context,
	filesystemService filesystem.Service,
) error {
	if svc.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check that the instance was actually removed
	benthosName := svc.getName()
	if inst, ok := svc.s6Manager.GetInstance(benthosName); ok {
		return fmt.Errorf("%w: S6 instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	sliceRemoveByName := func(arr []config.BenthosConfig, name string) []config.BenthosConfig {
		for i, cfg := range arr {
			if cfg.Name == name {
				return append(arr[:i], arr[i+1:]...)
			}
		}
		return arr
	}

	svc.benthosConfigs = sliceRemoveByName(svc.benthosConfigs, benthosName)

	if inst, ok := svc.benthosManager.GetInstance(benthosName); ok {
		return fmt.Errorf("%w: Benthos instance state=%s", standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	return nil
}

// Start starts a topic browser
func (svc *Service) Start(
	ctx context.Context,
	filesystemService filesystem.Service,
) error {
	if svc.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	benthosName := svc.getName()

	// Find and update our cached config
	found := false
	for i, config := range svc.benthosConfigs {
		if config.Name == benthosName {
			svc.benthosConfigs[i].DesiredFSMState = benthosfsm.OperationalStateActive
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return nil
}

// Stop stops a Topic Browser
func (svc *Service) Stop(
	ctx context.Context,
) error {
	if svc.s6Manager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	benthosName := svc.getName()

	// Find and update our cached config
	found := false
	for i, config := range svc.benthosConfigs {
		if config.Name == benthosName {
			svc.benthosConfigs[i].DesiredFSMState = benthosfsm.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return nil
}

// ReconcileManager synchronizes the topic browser on each tick.
func (svc *Service) ReconcileManager(
	ctx context.Context,
	services serviceregistry.Provider,
	tick uint64,
) (error, bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentTopicBrowserService, "ReconcileManager", time.Since(start))
	}()

	if svc.benthosManager == nil {
		return errors.New("benthos manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Reconcile the Benthos manager with our configs
	// The Benthos manager will handle the reconciliation with the S6 manager
	return svc.benthosManager.Reconcile(ctx, fsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{
			Internal: config.InternalConfig{
				Benthos: svc.benthosConfigs,
			},
		},
		Tick: tick,
	}, services)
}

// ServiceExists checks if a connection with the given name exists.
// Used by the FSM to determine appropriate transitions.
func (svc *Service) ServiceExists(
	ctx context.Context,
	services serviceregistry.Provider,
) bool {
	if svc.benthosManager == nil {
		return false
	}

	if ctx.Err() != nil {
		return false
	}

	benthosName := svc.getName()

	return svc.benthosService.ServiceExists(ctx, services.GetFileSystem(), benthosName)
}

// ForceRemove removes a topic browser from the s6 manager
func (svc *Service) ForceRemove(
	ctx context.Context,
	services serviceregistry.Provider,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Then force remove from Benthos manager
	return svc.benthosService.ForceRemoveBenthos(ctx, services.GetFileSystem(), svc.getName())
}
