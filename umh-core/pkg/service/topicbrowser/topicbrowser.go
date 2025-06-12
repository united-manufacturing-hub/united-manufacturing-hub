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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	benthossvccfg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	rpfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
	"go.uber.org/zap"
)

type ITopicBrowserService interface {
	// GenerateConfig generates a topic browser config
	GenerateConfig(tbName string) (benthossvccfg.BenthosServiceConfig, error)
	// Status returns information about the topic browser health
	Status(ctx context.Context, services serviceregistry.Provider, tbName string, snapshot fsm.SystemSnapshot) (ServiceInfo, error)
	// AddToManager registers a new topic browser
	AddToManager(ctx context.Context, filesystemService filesystem.Service, cfg *benthossvccfg.BenthosServiceConfig, tbName string) error
	// UpdateInManager modifies an existing topic browser configuration.
	UpdateInManager(ctx context.Context, filesystemService filesystem.Service, cfg *benthossvccfg.BenthosServiceConfig, tbName string) error
	// RemoveFromManager deletes a topic browser configuration.
	RemoveFromManager(ctx context.Context, filesystemService filesystem.Service, tbName string) error
	// Start begins the monitoring of a topic browser.
	Start(ctx context.Context, filesystemService filesystem.Service, tbName string) error
	// Stop stops the monitoring of a topic browser.
	Stop(ctx context.Context, filesystemService filesystem.Service, tbName string) error
	// ForceRemove removes a topic browser instance
	ForceRemove(ctx context.Context, services serviceregistry.Provider, tbName string) error
	// ServiceExists checks if a topic browser with the given name exists.
	ServiceExists(ctx context.Context, services serviceregistry.Provider, tbName string) bool
	// ReconcileManager synchronizes all connections on each tick.
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool)
}

// ServiceInfo holds information about the topic browsers health status.
type ServiceInfo struct {
	// benthos state information
	BenthosObservedState benthosfsm.BenthosObservedState
	BenthosFSMState      string

	// redpanda state information
	RedpandaObservedState rpfsm.RedpandaObservedState
	RedpandaFSMState      string

	// topic browser status
	Status                Status
	HasProcessingActivity bool
	StatusReason          string
}

// Service implements IConnectionService
type Service struct {
	logger    *zap.SugaredLogger
	s6Service s6svc.Service

	benthosManager *benthosfsm.BenthosManager
	benthosService benthossvc.IBenthosService
	benthosConfigs []config.BenthosConfig

	tbName     string // normally a service can handle multiple instances, the service monitor here is different and can only handle one instance
	ringbuffer *Ringbuffer
}

// ServiceOption is a function that configures a Service.
// This follows the functional options pattern for flexible configuration.
type ServiceOption func(*Service)

func WithService(svc benthossvc.IBenthosService) ServiceOption {
	return func(s *Service) {
		s.benthosService = svc
	}
}

func WithManager(mgr *benthosfsm.BenthosManager) ServiceOption {
	return func(s *Service) {
		s.benthosManager = mgr
	}
}

// NewDefaultService creates a new ConnectionService with default options.
// It initializes the logger and sets default values.
func NewDefaultService(tbName string, opts ...ServiceOption) *Service {
	managerName := fmt.Sprintf("%s%s", logger.ComponentTopicBrowserService, tbName)
	service := &Service{
		logger:         logger.For(managerName),
		s6Service:      s6svc.NewDefaultService(),
		benthosManager: benthosfsm.NewBenthosManager(managerName),
		benthosService: benthossvc.NewDefaultBenthosService(tbName),
		benthosConfigs: []config.BenthosConfig{},
		tbName:         tbName,
		ringbuffer:     NewRingbuffer(8), // NOTE: adjustable, no real reason for 8
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// getName converts the tbName (e.g. "myservice") that was given during NewService to its benthos service name (e.g. "topicbrowser-myservice")
func (svc *Service) getName(tbName string) string {
	return fmt.Sprintf("topicbrowser-%s", tbName)
}

func (svc *Service) getBenthosName() string {
	return fmt.Sprintf("benthos-topicbrowser-%s", svc.tbName)
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
	tbName string,
	snapshot fsm.SystemSnapshot,
) (ServiceInfo, error) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentTopicBrowserService, tbName+".Status", time.Since(start))
	}()

	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	benthosName := svc.getName(tbName)

	// First, check if the service exists in the Benthos manager
	// This is a crucial check that prevents "instance not found" errors
	// during reconciliation when a service is being created or removed
	if !svc.ServiceExists(ctx, services, tbName) {
		return ServiceInfo{}, ErrServiceNotExist
	}

	// Let's get the status of the underlying benthos service
	benthosObservedStateRaw, err := svc.benthosManager.GetLastObservedState(benthosName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get benthos observed state: %w", err)
	}

	benthosObservedState, ok := benthosObservedStateRaw.(benthosfsm.BenthosObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("observed state is not a BenthosObservedState: %v", benthosObservedStateRaw)
	}

	// Let's get the current FSM state of the underlying benthos FSM
	benthosFSMState, err := svc.benthosManager.GetCurrentFSMState(benthosName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get benthos FSM state: %w", err)
	}

	// get the redpanda instance from the redpanda manager
	rpInst, ok := fsm.FindInstance(snapshot, constants.RedpandaManagerName, constants.RedpandaInstanceName)
	if !ok || rpInst == nil {
		return ServiceInfo{}, fmt.Errorf("redpanda instance not found")
	}

	redpandaStatus := rpInst.LastObservedState

	// redpandaObservedStateSnapshot is slightly different from the others as it comes from the snapshot
	// and not from the fsm-instance
	redpandaObservedStateSnapshot, ok := redpandaStatus.(*rpfsm.RedpandaObservedStateSnapshot)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("redpanda status for redpanda %s is not a RedpandaObservedStateSnapshot", tbName)
	}

	// Now it is in the same format
	redpandaObservedState := rpfsm.RedpandaObservedState{
		ServiceInfo:                   redpandaObservedStateSnapshot.ServiceInfoSnapshot,
		ObservedRedpandaServiceConfig: redpandaObservedStateSnapshot.Config,
	}

	redpandaFSMState := rpInst.CurrentState

	// NOTE: access the correct s6Service here
	// Get logs
	s6ServicePath := filepath.Join(constants.S6BaseDir, svc.getBenthosName())
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

	statusReason, activity := svc.checkMetrics(redpandaObservedState, benthosObservedState)

	// no direct reference
	return ServiceInfo{
		BenthosObservedState:  benthosObservedState,
		BenthosFSMState:       benthosFSMState,
		RedpandaObservedState: redpandaObservedState,
		RedpandaFSMState:      redpandaFSMState,
		HasProcessingActivity: activity,
		StatusReason:          statusReason,
		Status: Status{
			Buffer: svc.ringbuffer.Get(),
			Logs:   logs,
		},
	}, nil
}

// AddToManager registers a new topic browser to the benthos manager.
func (svc *Service) AddToManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *benthossvccfg.BenthosServiceConfig,
	tbName string,
) error {
	if svc.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	svc.logger.Debugf("adding topic browser to benthos manager")

	benthosName := svc.getName(tbName)

	// Check whether benthosConfigs already contains an entry for this instance
	for _, config := range svc.benthosConfigs {
		if config.Name == benthosName {
			return ErrServiceAlreadyExists
		}
	}

	// Generate Benthos config
	benthosCfg, err := svc.GenerateConfig(tbName)
	if err != nil {
		return fmt.Errorf("failed to generate benthos config: %w", err)
	}

	// Create the Benthos FSM config
	benthosFSMConfig := config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            benthosName,
			DesiredFSMState: benthosfsm.OperationalStateStopped,
		},
		BenthosServiceConfig: benthosCfg,
	}

	// Add the benthos config to the list of benthos configs
	// So that the BenthosManager will start the service
	svc.benthosConfigs = append(svc.benthosConfigs, benthosFSMConfig)

	return nil
}

// UpdateInManager modifies an existing topic browser configuration.
func (svc *Service) UpdateInManager(
	ctx context.Context,
	filesystemServiec filesystem.Service,
	cfg *benthossvccfg.BenthosServiceConfig,
	tbName string,
) error {
	if svc.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	benthosName := svc.getName(tbName)

	// Check if the component exists
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

	// Generate Benthos config from DataFlowComponent config
	benthosCfg, err := svc.GenerateConfig(tbName)
	if err != nil {
		return fmt.Errorf("failed to generate benthos config: %w", err)
	}

	// Update our cached config while preserving the desired state
	currentDesiredState := svc.benthosConfigs[index].DesiredFSMState
	svc.benthosConfigs[index] = config.BenthosConfig{
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

// RemoveFromManager deletes a topic browser configuration.
// This stops monitoring the connection and removes all configuration.
func (svc *Service) RemoveFromManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	tbName string,
) error {
	if svc.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// ------------------------------------------------------------------
	// 1) Delete the *desired* config entry so the S6-manager will stop it
	// ------------------------------------------------------------------
	benthosName := svc.getName(tbName)

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

	svc.benthosConfigs = sliceRemoveByName(svc.benthosConfigs, benthosName)

	// ------------------------------------------------------------------
	// 2) is the child FSM still alive?
	// ------------------------------------------------------------------
	if inst, ok := svc.benthosManager.GetInstance(benthosName); ok {
		return fmt.Errorf("%w: Benthos instance state=%s", standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	return nil
}

// Start starts a topic browser
func (svc *Service) Start(
	ctx context.Context,
	filesystemServiec filesystem.Service,
	tbName string,
) error {
	if svc.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	benthosName := svc.getName(tbName)

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

// Stop stops a Connection
func (svc *Service) StopConnection(
	ctx context.Context,
	filesystemService filesystem.Service,
	tbName string,
) error {
	if svc.benthosManager == nil {
		return errors.New("benthos manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	benthosName := svc.getName(tbName)

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

// ReconcileManager synchronizes all connections on each tick.
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

// ServiceExists checks if a service with the given name exists.
// Used by the FSM to determine appropriate transitions.
func (svc *Service) ServiceExists(
	ctx context.Context,
	services serviceregistry.Provider,
	tbName string,
) bool {
	if ctx.Err() != nil {
		return false
	}

	benthosName := svc.getName(tbName)

	return svc.benthosService.ServiceExists(ctx, services.GetFileSystem(), benthosName)
}

// ForceRemoveConnection removes a Connection from the Nmap manager
func (svc *Service) ForceRemoveConnection(
	ctx context.Context,
	services serviceregistry.Provider,
	tbName string,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Then force remove from Benthos manager
	return svc.benthosService.ForceRemoveBenthos(ctx, services.GetFileSystem(), svc.getName(tbName))
}

func (svc *Service) checkMetrics(
	rpObsState rpfsm.RedpandaObservedState,
	benObsState benthosfsm.BenthosObservedState,
) (string, bool) {
	if rpObsState.ServiceInfo.RedpandaStatus.RedpandaMetrics.Metrics.Throughput.BytesOut > 0 {
		return "", true
	}

	// Redpanda output but no benthos output
	// Benthos output but not redpanda output
	// Redpanda output but no benthos input

	return "", false
}
