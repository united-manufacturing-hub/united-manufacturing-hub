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

package dataflowcomponent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"

	benthosfsmmanager "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	benthosservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
)

// IDataFlowComponentService defines the interface for managing DataFlowComponent services.
// This service is responsible for managing the lifecycle of dataflow components and
// communicating with the underlying Benthos manager.
type IDataFlowComponentService interface {
	// GenerateBenthosConfigForDataFlowComponent generates a Benthos config for a given dataflow component
	GenerateBenthosConfigForDataFlowComponent(dataflowConfig *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, componentName string) (benthosserviceconfig.BenthosServiceConfig, error)

	// GetConfig returns the actual DataFlowComponent config from the Benthos service
	GetConfig(ctx context.Context, services serviceregistry.Provider, componentName string) (dataflowcomponentserviceconfig.DataflowComponentServiceConfig, error)

	// Status checks the status of a DataFlowComponent service
	Status(ctx context.Context, services serviceregistry.Provider, componentName string, tick uint64) (ServiceInfo, error)

	// AddDataFlowComponentToBenthosManager adds a DataFlowComponent to the Benthos manager
	AddDataFlowComponentToBenthosManager(ctx context.Context, services serviceregistry.Provider, cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, componentName string) error

	// UpdateDataFlowComponentInBenthosManager updates an existing DataFlowComponent in the Benthos manager
	UpdateDataFlowComponentInBenthosManager(ctx context.Context, services serviceregistry.Provider, cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, componentName string) error

	// RemoveDataFlowComponentFromBenthosManager removes a DataFlowComponent from the Benthos manager
	RemoveDataFlowComponentFromBenthosManager(ctx context.Context, services serviceregistry.Provider, componentName string) error

	// StartDataFlowComponent starts a DataFlowComponent
	StartDataFlowComponent(ctx context.Context, services serviceregistry.Provider, componentName string) error

	// StopDataFlowComponent stops a DataFlowComponent
	StopDataFlowComponent(ctx context.Context, services serviceregistry.Provider, componentName string) error

	// ForceRemoveDataFlowComponent removes a DataFlowComponent from the Benthos manager
	ForceRemoveDataFlowComponent(ctx context.Context, services serviceregistry.Provider, componentName string) error

	// ServiceExists checks if a DataFlowComponent service exists
	ServiceExists(ctx context.Context, services serviceregistry.Provider, componentName string) bool

	// ReconcileManager reconciles the DataFlowComponent manager with the actual state
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool)
}

// ServiceInfo contains information about a DataFlowComponent service
type ServiceInfo struct {
	// BenthosObservedState contains information about the Benthos service
	BenthosObservedState benthosfsmmanager.BenthosObservedState

	// BenthosFSMState contains the current state of the Benthos FSM
	BenthosFSMState string

	// StatusReason contains the reason for the current status
	StatusReason string
}

// DataFlowComponentService is the default implementation of the IDataFlowComponentService interface
type DataFlowComponentService struct {
	logger         *zap.SugaredLogger
	benthosManager *benthosfsmmanager.BenthosManager
	benthosService benthosservice.IBenthosService
	benthosConfigs []config.BenthosConfig
}

// DataFlowComponentServiceOption is a function that modifies a DataFlowComponentService
type DataFlowComponentServiceOption func(*DataFlowComponentService)

// WithBenthosService sets a custom Benthos service for the DataFlowComponentService
func WithBenthosService(benthosService benthosservice.IBenthosService) DataFlowComponentServiceOption {
	return func(s *DataFlowComponentService) {
		s.benthosService = benthosService
	}
}

// WithBenthosManager sets a custom Benthos manager for the DataFlowComponentService
func WithBenthosManager(benthosManager *benthosfsmmanager.BenthosManager) DataFlowComponentServiceOption {
	return func(s *DataFlowComponentService) {
		s.benthosManager = benthosManager
	}
}

// NewDefaultDataFlowComponentService creates a new default DataFlowComponent service
func NewDefaultDataFlowComponentService(componentName string, opts ...DataFlowComponentServiceOption) *DataFlowComponentService {

	managerName := fmt.Sprintf("%s%s", logger.ComponentDataFlowComponentService, componentName)
	service := &DataFlowComponentService{
		logger:         logger.For(managerName),
		benthosManager: benthosfsmmanager.NewBenthosManager(managerName),
		benthosService: benthosservice.NewDefaultBenthosService(componentName),
		benthosConfigs: []config.BenthosConfig{},
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// getBenthosName converts a componentName to its Benthos service name
func (s *DataFlowComponentService) getBenthosName(componentName string) string {
	return fmt.Sprintf("dataflow-%s", componentName)
}

// GenerateBenthosConfigForDataFlowComponent generates a Benthos config for a given dataflow component
func (s *DataFlowComponentService) GenerateBenthosConfigForDataFlowComponent(dataflowConfig *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, componentName string) (benthosserviceconfig.BenthosServiceConfig, error) {
	if dataflowConfig == nil {
		return benthosserviceconfig.BenthosServiceConfig{}, fmt.Errorf("dataflow config is nil")
	}

	// Convert DataFlowComponent config to Benthos service config
	return dataflowConfig.GetBenthosServiceConfig(), nil
}

// GetConfig returns the actual DataFlowComponent config from the Benthos service
// Expects benthosName (e.g. "dataflow-myservice") as defined in the UMH config
func (s *DataFlowComponentService) GetConfig(ctx context.Context, services serviceregistry.Provider, componentName string) (dataflowcomponentserviceconfig.DataflowComponentServiceConfig, error) {
	if ctx.Err() != nil {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, ctx.Err()
	}

	benthosName := s.getBenthosName(componentName)

	// Get the Benthos config
	benthosCfg, err := s.benthosService.GetConfig(ctx, services, benthosName)
	if err != nil {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("failed to get benthos config: %w", err)
	}

	// Convert Benthos config to DataFlowComponent config
	return dataflowcomponentserviceconfig.FromBenthosServiceConfig(benthosCfg), nil
}

// Status checks the status of a DataFlowComponent service
// Expects benthosName (e.g. "dataflow-myservice") as defined in the UMH config
func (s *DataFlowComponentService) Status(ctx context.Context, services serviceregistry.Provider, componentName string, tick uint64) (ServiceInfo, error) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentDataFlowComponentService, componentName+".Status", time.Since(start))
	}()

	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	benthosName := s.getBenthosName(componentName)

	// First, check if the service exists in the Benthos manager
	// This is a crucial check that prevents "instance not found" errors
	// during reconciliation when a service is being created or removed
	if !s.ServiceExists(ctx, services, componentName) {
		return ServiceInfo{}, ErrServiceNotExists
	}

	// Let's get the status of the underlying benthos service
	benthosObservedStateRaw, err := s.benthosManager.GetLastObservedState(benthosName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get benthos observed state: %w", err)
	}

	benthosObservedState, ok := benthosObservedStateRaw.(benthosfsmmanager.BenthosObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("observed state is not a BenthosObservedState: %v", benthosObservedStateRaw)
	}

	// Let's get the current FSM state of the underlying benthos FSM
	benthosFSMState, err := s.benthosManager.GetCurrentFSMState(benthosName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get benthos FSM state: %w", err)
	}

	// No additional healthchecks are done here for now
	// Maybe in the future there will be some for rollbacks or other things

	return ServiceInfo{
		BenthosObservedState: benthosObservedState,
		BenthosFSMState:      benthosFSMState,
		StatusReason:         benthosObservedState.ServiceInfo.BenthosStatus.StatusReason,
	}, nil
}

// AddDataFlowComponentToBenthosManager adds a DataFlowComponent to the Benthos manager
// Expects benthosName (e.g. "dataflow-myservice") as defined in the UMH config
func (s *DataFlowComponentService) AddDataFlowComponentToBenthosManager(ctx context.Context, services serviceregistry.Provider, cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, componentName string) error {
	if s.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	benthosName := s.getBenthosName(componentName)

	// Check whether benthosConfigs already contains an entry for this instance
	for _, config := range s.benthosConfigs {
		if config.Name == benthosName {
			return ErrServiceAlreadyExists
		}
	}

	// Generate Benthos config from DataFlowComponent config
	benthosCfg, err := s.GenerateBenthosConfigForDataFlowComponent(cfg, componentName)
	if err != nil {
		return fmt.Errorf("failed to generate benthos config: %w", err)
	}

	// Create the Benthos FSM config
	benthosFSMConfig := config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            benthosName,
			DesiredFSMState: benthosfsmmanager.OperationalStateStopped,
		},
		BenthosServiceConfig: benthosCfg,
	}

	// Add the benthos config to the list of benthos configs
	// So that the BenthosManager will start the service
	s.benthosConfigs = append(s.benthosConfigs, benthosFSMConfig)

	return nil
}

// UpdateDataFlowComponentInBenthosManager updates an existing DataFlowComponent in the Benthos manager
// Expects benthosName (e.g. "dataflow-myservice") as defined in the UMH config
func (s *DataFlowComponentService) UpdateDataFlowComponentInBenthosManager(ctx context.Context, services serviceregistry.Provider, cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, componentName string) error {
	if s.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	benthosName := s.getBenthosName(componentName)

	// Check if the component exists
	found := false
	index := -1
	for i, config := range s.benthosConfigs {
		if config.Name == benthosName {
			found = true
			index = i
			break
		}
	}

	if !found {
		return ErrServiceNotExists
	}

	// Generate Benthos config from DataFlowComponent config
	benthosCfg, err := s.GenerateBenthosConfigForDataFlowComponent(cfg, componentName)
	if err != nil {
		return fmt.Errorf("failed to generate benthos config: %w", err)
	}

	// Update our cached config while preserving the desired state
	currentDesiredState := s.benthosConfigs[index].DesiredFSMState
	s.benthosConfigs[index] = config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            benthosName,
			DesiredFSMState: currentDesiredState,
		},
		BenthosServiceConfig: benthosCfg,
	}

	// The next reconciliation of the Benthos manager will detect the config change
	// and update the service
	return nil
}

// RemoveDataFlowComponentFromBenthosManager removes a DataFlowComponent from the Benthos manager
// Expects benthosName (e.g. "dataflow-myservice") as defined in the UMH config
func (s *DataFlowComponentService) RemoveDataFlowComponentFromBenthosManager(ctx context.Context, services serviceregistry.Provider, componentName string) error {
	if s.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// ------------------------------------------------------------------
	// 1) Delete the *desired* config entry so the S6-manager will stop it
	// ------------------------------------------------------------------
	benthosName := s.getBenthosName(componentName)

	// Helper that deletes exactly one element *without* reallocating when the
	// element is already missing – keeps the call idempotent and allocation-free.
	sliceRemoveByName := func(arr []config.BenthosConfig, name string) []config.BenthosConfig {
		for i, cfg := range arr {
			if cfg.Name == name {
				return append(arr[:i], arr[i+1:]...)
			}
		}
		return arr
	}

	s.benthosConfigs = sliceRemoveByName(s.benthosConfigs, benthosName)

	// ------------------------------------------------------------------
	// 2) is the child FSM still alive?
	// ------------------------------------------------------------------
	if inst, ok := s.benthosManager.GetInstance(benthosName); ok {
		return fmt.Errorf("%w: Benthos instance state=%s", standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	return nil
}

// StartDataFlowComponent starts a DataFlowComponent
// Expects benthosName (e.g. "dataflow-myservice") as defined in the UMH config
func (s *DataFlowComponentService) StartDataFlowComponent(ctx context.Context, services serviceregistry.Provider, componentName string) error {
	if s.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	benthosName := s.getBenthosName(componentName)

	// Find and update our cached config
	found := false
	for i, config := range s.benthosConfigs {
		if config.Name == benthosName {
			s.benthosConfigs[i].DesiredFSMState = benthosfsmmanager.OperationalStateActive
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExists
	}

	return nil
}

// StopDataFlowComponent stops a DataFlowComponent
// Expects benthosName (e.g. "dataflow-myservice") as defined in the UMH config
func (s *DataFlowComponentService) StopDataFlowComponent(ctx context.Context, services serviceregistry.Provider, componentName string) error {
	if s.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	benthosName := s.getBenthosName(componentName)

	// Find and update our cached config
	found := false
	for i, config := range s.benthosConfigs {
		if config.Name == benthosName {
			s.benthosConfigs[i].DesiredFSMState = benthosfsmmanager.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExists
	}

	return nil
}

// ReconcileManager reconciles the DataFlowComponent manager
func (s *DataFlowComponentService) ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentDataFlowComponentService, "ReconcileManager", time.Since(start))
	}()

	if s.benthosManager == nil {
		return errors.New("benthos manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Reconcile the Benthos manager with our configs
	// The Benthos manager will handle the reconciliation with the S6 manager
	return s.benthosManager.Reconcile(ctx, fsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{
			Internal: config.InternalConfig{
				Benthos: s.benthosConfigs,
			},
		},
		Tick: tick,
	}, services)
}

// ServiceExists checks if a DataFlowComponent service exists
func (s *DataFlowComponentService) ServiceExists(ctx context.Context, services serviceregistry.Provider, componentName string) bool {
	benthosName := s.getBenthosName(componentName)

	// Then check the actual service existence
	return s.benthosService.ServiceExists(ctx, services, benthosName)
}

// ForceRemoveDataFlowComponent removes a DataFlowComponent from the Benthos manager
// This should only be called if the DataFlowComponent is in a permanent failure state
// and the instance itself cannot be stopped or removed
// Expects benthosName (e.g. "dataflow-myservice") as defined in the UMH config
func (s *DataFlowComponentService) ForceRemoveDataFlowComponent(ctx context.Context, services serviceregistry.Provider, componentName string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Then force remove from Benthos manager
	return s.benthosService.ForceRemoveBenthos(ctx, services, s.getBenthosName(componentName))
}
