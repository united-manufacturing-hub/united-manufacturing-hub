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
	"sync"
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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
	"go.uber.org/zap"
)

type ITopicBrowserService interface {
	// GenerateConfig generates a topic browser config
	GenerateConfig(tbName string) (benthossvccfg.BenthosServiceConfig, error)
	// GetConfig returns the actual DataFlowComponent config from the Benthos service
	GetConfig(ctx context.Context, filesystemService filesystem.Service, tbName string) (benthossvccfg.BenthosServiceConfig, error)
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

type Status struct {
	Logs           []s6_shared.LogEntry // contain the structured s6 logs entries
	BufferSnapshot RingBufferSnapshot   // structured ring buffer snapshot with sequence tracking
}

// CopyLogs is a go-deepcopy override for the Logs field.
//
// go-deepcopy looks for a method with the signature
//
//	func (dst *T) Copy<FieldName>(src <FieldType>) error
func (st *Status) CopyLogs(src []s6_shared.LogEntry) error {
	st.Logs = src
	return nil
}

// CopyBufferSnapshot is a go-deepcopy override for the BufferSnapshot field.
//
// go-deepcopy looks for a method with the signature
//
//	func (dst *T) Copy<FieldName>(src <FieldType>) error
//
// and, if present, calls it instead of performing its generic deep-copy logic.
// By assigning the slice directly we make a **shallow copy**: the header is
// duplicated but the underlying backing array is shared.
//
// Why this is safe:
//
//  1. The ringbuffer service returns a fresh []*Buffer on every call, never reusing
//     or mutating a previously returned slice (see Ringbuffer.Get() method).
//  2. Buffer is treated as immutable after the snapshot is taken.
//
// If either assumption changes, delete this method to fall back to the default
// deep-copy (O(n) but safe for mutable slices).
//
// See also: https://github.com/tiendc/go-deepcopy?tab=readme-ov-file#copy-struct-fields-via-struct-methods
func (st *Status) CopyBufferSnapshot(src RingBufferSnapshot) error {
	st.BufferSnapshot = src
	return nil
}

type ServiceInfo struct {
	BenthosFSMState string

	RedpandaFSMState string

	StatusReason string

	// topic browser status
	Status Status

	// redpanda state information
	RedpandaObservedState rpfsm.RedpandaObservedState
	// benthos state information
	BenthosObservedState benthosfsm.BenthosObservedState

	// processing activities
	BenthosProcessing  bool // is benthos active
	RedpandaProcessing bool // is redpanda active
	InvalidMetrics     bool // if there is invalid metrics e.g. redpanda has no output but benthos has input
}

// CopyStatus is a go-deepcopy override for the Status field.
//
// go-deepcopy looks for a method with the signature
//
//	func (dst *T) Copy<FieldName>(src <FieldType>) error
//
// This method ensures that when ServiceInfo is deep copied, the nested Status
// struct uses its own custom copy methods (CopyLogs and CopyBuffer) to perform
// shallow copies instead of expensive deep copies.
func (si *ServiceInfo) CopyStatus(src Status) error {
	// Use the Status struct's own copy logic which handles Buffer and Logs efficiently
	si.Status.BufferSnapshot = src.BufferSnapshot // Shallow copy (handled by Status.CopyBuffer)
	si.Status.Logs = src.Logs                     // Shallow copy (handled by Status.CopyLogs)
	return nil
}

// Service implements ITopicBrowserService
type Service struct {

	// Block processing tracking
	lastProcessedTimestamp time.Time
	benthosService         benthossvc.IBenthosService
	logger                 *zap.SugaredLogger

	benthosManager *benthosfsm.BenthosManager
	ringbuffer     *Ringbuffer

	tbName         string // normally a service can handle multiple instances, the service monitor here is different and can only handle one instance
	benthosConfigs []config.BenthosConfig

	processingMutex sync.RWMutex
}

// ServiceOption is a function that configures a Service.
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

// NewDefaultService creates a new topic browser service with default options.
// It initializes the logger and sets default values.
func NewDefaultService(tbName string, opts ...ServiceOption) *Service {
	managerName := fmt.Sprintf("%s%s", logger.ComponentTopicBrowserService, tbName)
	service := &Service{
		logger:                 logger.For(managerName),
		benthosManager:         benthosfsm.NewBenthosManager(managerName),
		benthosService:         benthossvc.NewDefaultBenthosService(tbName),
		benthosConfigs:         []config.BenthosConfig{},
		tbName:                 tbName,
		ringbuffer:             NewRingbufferWithDefaultCapacity(),
		lastProcessedTimestamp: time.Time{}, // Start from beginning
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

// GetConfig returns the actual Benthos config from the Benthos service
// Expects benthosName (e.g. "topicbrowser-myservice") as defined in the UMH config
func (svc *Service) GetConfig(ctx context.Context, filesystemService filesystem.Service, tbName string) (benthossvccfg.BenthosServiceConfig, error) {
	if ctx.Err() != nil {
		return benthossvccfg.BenthosServiceConfig{}, ctx.Err()
	}

	benthosName := svc.getName(tbName)

	// Get the Benthos config
	benthosCfg, err := svc.benthosService.GetConfig(ctx, filesystemService, benthosName)
	if err != nil {
		return benthossvccfg.BenthosServiceConfig{}, fmt.Errorf("failed to get benthos config: %w", err)
	}

	// Convert Benthos config to Topic Browser config
	return benthosCfg, nil
}

// GenerateConfig provides a fixed benthosserviceconfig to ensure deploying
// a benthos instants that reads data from uns, processes it to get into a
// proper protobuf format and writes it to stdout using an additional timestamp.
func (svc *Service) GenerateConfig(tbName string) (benthossvccfg.BenthosServiceConfig, error) {
	return benthossvccfg.DefaultTopicBrowserBenthosServiceConfig, nil
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

	// Get logs
	logs := benthosObservedState.ServiceInfo.BenthosStatus.BenthosLogs

	// Parse the logs and hex-decode it, afterwards the data gets written
	// into the ringbuffer
	svc.logger.Debugf("parsing block from logs: %d", len(logs))
	err = svc.parseBlock(logs)
	if err != nil {
		svc.logger.Errorf("failed to parse block from logs: %v", err)
		return ServiceInfo{}, fmt.Errorf("failed to parse block from logs: %w", err)
	}
	svc.logger.Debugf("parsed block from logs")

	// check for invalidMetrics from benthos and redpanda
	statusReason, invalidMetrics := svc.checkMetrics(redpandaObservedState, benthosObservedState)

	// no direct reference
	return ServiceInfo{
		BenthosObservedState:  benthosObservedState,
		BenthosFSMState:       benthosFSMState,
		RedpandaObservedState: redpandaObservedState,
		RedpandaFSMState:      redpandaFSMState,
		BenthosProcessing:     svc.benthosProcessingActivity(benthosObservedState),
		RedpandaProcessing:    svc.redpandaProcessingActivity(redpandaObservedState),
		InvalidMetrics:        invalidMetrics,
		StatusReason:          statusReason,
		Status: Status{
			BufferSnapshot: svc.ringbuffer.GetSnapshot(),
			Logs:           logs,
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
// This stops monitoring the topic browser and removes all configuration.
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

// ForceRemove removes a topic browser from the s6 manager
func (svc *Service) ForceRemove(
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

// checks for invalid metric states e.g.:
// - redpanda has output but benthos no input
// - redpanda has no output but benthos has
// - redpanda has output but benthos not
func (svc *Service) checkMetrics(
	rpObsState rpfsm.RedpandaObservedState,
	benObsState benthosfsm.BenthosObservedState,
) (string, bool) {
	if rpObsState.ServiceInfo.RedpandaStatus.RedpandaMetrics.MetricsState == nil {
		return "no redpanda metrics available", true
	}

	if benObsState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState == nil {
		return "no benthos metrics available", true
	}

	// Redpanda output but no benthos output
	if rpObsState.ServiceInfo.RedpandaStatus.RedpandaMetrics.Metrics.Throughput.BytesOut > 0 &&
		benObsState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState.Output.MessagesPerTick == 0 {
		return "redpanda has output, but benthos no output", true
	}

	// Benthos output but not redpanda output
	if rpObsState.ServiceInfo.RedpandaStatus.RedpandaMetrics.Metrics.Throughput.BytesOut == 0 &&
		benObsState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState.Output.MessagesPerTick > 0 {
		return "redpanda has no output, but benthos has output", true
	}

	// Redpanda output but no benthos input
	if rpObsState.ServiceInfo.RedpandaStatus.RedpandaMetrics.Metrics.Throughput.BytesOut > 0 &&
		benObsState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState.Input.MessagesPerTick == 0 {
		return "redpanda has output, but benthos no input", true
	}

	return "", false
}

// check if benthos is active / has processing activity to provide information
// for starting phase of fsm
func (svc *Service) benthosProcessingActivity(observedState benthosfsm.BenthosObservedState) bool {
	if observedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState == nil {
		return false
	}
	return observedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState.IsActive
}

// check if redpanda is active / has processing activity to provide information
// for starting phase of fsm
func (svc *Service) redpandaProcessingActivity(observedState rpfsm.RedpandaObservedState) bool {
	if observedState.ServiceInfo.RedpandaStatus.RedpandaMetrics.MetricsState == nil {
		return false
	}
	if observedState.ServiceInfo.RedpandaStatus.RedpandaMetrics.MetricsState.IsActive {
		return true
	}
	return false
}

// ResetBlockProcessing resets the block processing state (useful for testing or restart scenarios)
func (svc *Service) ResetBlockProcessing() {
	svc.processingMutex.Lock()
	defer svc.processingMutex.Unlock()
	svc.lastProcessedTimestamp = time.Time{}
}
