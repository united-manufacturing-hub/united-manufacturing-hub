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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
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
	Start(ctx context.Context, tbName string) error
	// Stop stops the monitoring of a topic browser.
	Stop(ctx context.Context, tbName string) error
	// ForceRemove removes a topic browser instance
	ForceRemove(ctx context.Context, services serviceregistry.Provider, tbName string) error
	// ServiceExists checks if a topic browser with the given name exists.
	ServiceExists(ctx context.Context, services serviceregistry.Provider, tbName string) bool
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
	benthosConfig  config.BenthosConfig

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
		benthosConfig:  config.BenthosConfig{},
		tbName:         tbName,
		ringbuffer:     NewRingbuffer(8),
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// getName converts the tbName (e.g. "myservice") that was given during NewService to its S6 service name (e.g. "topicbrowser-myservice")
func (svc *Service) getName() string {
	return fmt.Sprintf("topicbrowser-%s", svc.tbName)
}

func (svc *Service) GenerateConfig(s6SvcName string) (s6serviceconfig.S6ServiceConfig, error) {
	// TODO: generate script

	// load s6svcConfig with script
	return s6serviceconfig.S6ServiceConfig{}, nil
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
) error {
	if svc.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	svc.logger.Debugf("Adding topic browser to S6 manager")

	s6ServiceName := svc.getName()

	// Check whether s6ServiceConfigs already contains an entry for this instance
	if svc.s6ServiceConfig != nil {
		return ErrServiceAlreadyExists
	}

	// Generate the S6 config for this instance
	s6Config, err := svc.GenerateConfig(s6ServiceName)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for Topic Browser service %s: %w", s6ServiceName, err)
	}

	// Create the S6 FSM config for this instance
	s6FSMConfig := config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: s6fsm.OperationalStateRunning,
		},
		S6ServiceConfig: s6Config,
	}

	// Add the S6 FSM config to the list of S6 FSM configs
	svc.s6ServiceConfig = &s6FSMConfig

	return nil
}

// UpdateInManager modifies an existing topic browser configuration.
func (svc *Service) UpdateInManager(
	ctx context.Context,
) error {
	if svc.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if svc.s6ServiceConfig == nil {
		return ErrServiceNotExist
	}

	// Generate the S6 config for this instance
	s6ServiceName := svc.getName()
	s6Config, err := svc.GenerateConfig(s6ServiceName)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for Topic Browser service %s: %w", s6ServiceName, err)
	}

	svc.s6ServiceConfig.S6ServiceConfig = s6Config

	return nil
}

// RemoveFromManager deletes a topic browser configuration.
// This stops monitoring the topic browser and removes all configuration.
func (svc *Service) RemoveFromManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	tbName string,
) error {
	if svc.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	svc.s6ServiceConfig = nil

	// Check that the instance was actually removed
	s6Name := svc.getName()
	if inst, ok := svc.s6Manager.GetInstance(s6Name); ok {
		return fmt.Errorf("%w: S6 instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	return nil
}

// Start starts a topic browser
func (svc *Service) Start(
	ctx context.Context,
	filesystemService filesystem.Service,
	tbName string,
) error {
	if svc.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if svc.s6ServiceConfig == nil {
		return ErrServiceNotExist
	}

	svc.s6ServiceConfig.DesiredFSMState = s6fsm.OperationalStateRunning

	return nil
}

// Stop stops a Topic Browser
func (svc *Service) Stop(
	ctx context.Context,
) error {
	if svc.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if svc.s6ServiceConfig == nil {
		return ErrServiceNotExist
	}

	svc.s6ServiceConfig.DesiredFSMState = s6fsm.OperationalStateStopped

	return nil
}

// ReconcileManager synchronizes the topic browser on each tick.
func (svc *Service) ReconcileManager(
	ctx context.Context,
	services serviceregistry.Provider,
	tick uint64,
) (error, bool) {
	if svc.s6Manager == nil {
		return errors.New("s6 manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	serviceMap := config.FullConfig{Internal: config.InternalConfig{Services: []config.S6FSMConfig{}}}

	if svc.s6ServiceConfig != nil {
		serviceMap.Internal.Services = []config.S6FSMConfig{*svc.s6ServiceConfig}
	}

	return svc.s6Manager.Reconcile(ctx, fsm.SystemSnapshot{CurrentConfig: serviceMap}, services)
}

// ServiceExists checks if a connection with the given name exists.
// Used by the FSM to determine appropriate transitions.
func (svc *Service) ServiceExists(
	ctx context.Context,
	services serviceregistry.Provider,
	tbName string,
) bool {
	if svc.s6Manager == nil {
		return false
	}

	if ctx.Err() != nil {
		return false
	}

	exists, err := svc.s6Service.ServiceExists(ctx, filepath.Join(constants.S6BaseDir, svc.getName()), services.GetFileSystem())
	if err != nil {
		return false
	}

	return exists
}

// ForceRemove removes a topic browser from the s6 manager
func (svc *Service) ForceRemove(
	ctx context.Context,
	services serviceregistry.Provider,
) error {
	s6ServiceName := svc.getName()
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)
	return services.GetFileSystem().RemoveAll(ctx, s6ServicePath)
}
