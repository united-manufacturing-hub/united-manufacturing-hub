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

// Code generated from telemetry.yaml. DO NOT EDIT.

package telemetry

type actionsNode struct {
	UnknownType Identifier
}

// Actions holds the declared events of the actions domain.
var Actions = actionsNode{
	UnknownType: Identifier{Tag: "actions::unknown_type", Brief: "The Management Console sent an action type umh-core has no handler for, so it was ignored.", Severity: SeverityWarning},
}

type adapterChildNode struct {
	SpecBuildFailed Identifier
	UpsertFailed    Identifier
}

type adapterNode struct {
	Child adapterChildNode
}

// Adapter holds the declared events of the adapter domain.
var Adapter = adapterNode{
	Child: adapterChildNode{
		SpecBuildFailed: Identifier{Tag: "adapter::child::spec_build_failed", Brief: "Building a child worker's spec failed, so the child is absent until the next tick retries.", Severity: SeverityWarning},
		UpsertFailed:    Identifier{Tag: "adapter::child::upsert_failed", Brief: "Upserting a child worker failed, so the child is absent until the next tick retries.", Severity: SeverityWarning},
	},
}

type communicatorActionHandlerNode struct {
	DoublePanic                Identifier
	Panic                      Identifier
	PanicReplyGenerationFailed Identifier
}

type communicatorBridgeDeployNode struct {
	AddFailed                    Identifier
	CreateConfigFailed           Identifier
	StopOnFailureFailed          Identifier
	StopOnFailureGetConfigFailed Identifier
	Timeout                      Identifier
	WaitFailed                   Identifier
}

type communicatorBridgeEditNode struct {
	ApplyMutationFailed         Identifier
	ConfigErrorRollbackFailed   Identifier
	ConfigErrorRolledBack       Identifier
	PersistConfigFailed         Identifier
	RenderFailureRollbackFailed Identifier
	RenderFailureRolledBack     Identifier
	RollbackFailed              Identifier
	RollbackOnTimeout           Identifier
	RolloutFailed               Identifier
}

type communicatorBridgeNode struct {
	Deploy         communicatorBridgeDeployNode
	Edit           communicatorBridgeEditNode
	ExecuteFailed  Identifier
	ParseFailed    Identifier
	ValidateFailed Identifier
}

type communicatorNode struct {
	ActionHandler communicatorActionHandlerNode
	Bridge        communicatorBridgeNode
}

// Communicator holds the declared events of the communicator domain.
var Communicator = communicatorNode{
	ActionHandler: communicatorActionHandlerNode{
		DoublePanic:                Identifier{Tag: "communicator::action_handler::double_panic", Brief: "The action handler panicked while recovering from a panic, so the action is lost and the reply never sent.", Severity: SeverityError},
		Panic:                      Identifier{Tag: "communicator::action_handler::panic", Brief: "An action handler panicked; it was recovered, so umh-core keeps running but the action did not complete.", Severity: SeverityError},
		PanicReplyGenerationFailed: Identifier{Tag: "communicator::action_handler::panic_reply_generation_failed", Brief: "After recovering an action-handler panic, building the failure reply also failed, so the console hears nothing.", Severity: SeverityError},
	},
	Bridge: communicatorBridgeNode{
		Deploy: communicatorBridgeDeployNode{
			AddFailed:                    Identifier{Tag: "communicator::bridge::deploy::add_failed", Brief: "Adding the bridge to config.yaml failed, so the bridge was not deployed.", Severity: SeverityError},
			CreateConfigFailed:           Identifier{Tag: "communicator::bridge::deploy::create_config_failed", Brief: "Building the bridge's config failed, so nothing was written and the bridge was not deployed.", Severity: SeverityError},
			StopOnFailureFailed:          Identifier{Tag: "communicator::bridge::deploy::stop_on_failure_failed", Brief: "A bridge deployment failed and stopping the half-deployed bridge also failed, so it may still be running.", Severity: SeverityError},
			StopOnFailureGetConfigFailed: Identifier{Tag: "communicator::bridge::deploy::stop_on_failure_get_config_failed", Brief: "A bridge deployment failed and reading its config to stop it also failed, so it may still be running.", Severity: SeverityError},
			Timeout:                      Identifier{Tag: "communicator::bridge::deploy::timeout", Brief: "The bridge did not reach its desired state within the deployment timeout.", Severity: SeverityWarning},
			WaitFailed:                   Identifier{Tag: "communicator::bridge::deploy::wait_failed", Brief: "Waiting for the deployed bridge to come up failed, so its state is unknown.", Severity: SeverityError},
		},
		Edit: communicatorBridgeEditNode{
			ApplyMutationFailed:         Identifier{Tag: "communicator::bridge::edit::apply_mutation_failed", Brief: "Applying the edit to the bridge's config failed, so the bridge is unchanged.", Severity: SeverityError},
			ConfigErrorRollbackFailed:   Identifier{Tag: "communicator::bridge::edit::config_error_rollback_failed", Brief: "A bridge edit produced an invalid config and the rollback also failed, so config.yaml holds the bad edit.", Severity: SeverityError},
			ConfigErrorRolledBack:       Identifier{Tag: "communicator::bridge::edit::config_error_rolled_back", Brief: "A bridge edit produced an invalid config and was rolled back, so the bridge keeps its previous config.", Severity: SeverityWarning},
			PersistConfigFailed:         Identifier{Tag: "communicator::bridge::edit::persist_config_failed", Brief: "Writing the edited bridge config failed, so the edit did not take.", Severity: SeverityError},
			RenderFailureRollbackFailed: Identifier{Tag: "communicator::bridge::edit::render_failure_rollback_failed", Brief: "A bridge edit failed to render and the rollback also failed, so config.yaml holds a template that will not render.", Severity: SeverityError},
			RenderFailureRolledBack:     Identifier{Tag: "communicator::bridge::edit::render_failure_rolled_back", Brief: "A bridge edit failed to render and was rolled back, so the bridge keeps its previous config.", Severity: SeverityWarning},
			RollbackFailed:              Identifier{Tag: "communicator::bridge::edit::rollback_failed", Brief: "Rolling back a failed bridge edit failed, so config.yaml may hold a half-applied edit.", Severity: SeverityError},
			RollbackOnTimeout:           Identifier{Tag: "communicator::bridge::edit::rollback_on_timeout", Brief: "An edited bridge did not come up in time, so the edit was rolled back.", Severity: SeverityWarning},
			RolloutFailed:               Identifier{Tag: "communicator::bridge::edit::rollout_failed", Brief: "Rolling out the bridge edit failed, so the bridge may be running its old config.", Severity: SeverityError},
		},
		ExecuteFailed:  Identifier{Tag: "communicator::bridge::execute_failed", Brief: "Executing the bridge action failed, so the requested change did not happen.", Severity: SeverityError},
		ParseFailed:    Identifier{Tag: "communicator::bridge::parse_failed", Brief: "The bridge action's payload could not be parsed, so the action was rejected.", Severity: SeverityError},
		ValidateFailed: Identifier{Tag: "communicator::bridge::validate_failed", Brief: "The bridge action's payload failed validation, so the action was rejected.", Severity: SeverityError},
	},
}

type configAgentNode struct {
	LocationMapMissing    Identifier
	ReleaseChannelInvalid Identifier
}

type configBackupCleanupNode struct {
	DirReadFailed Identifier
	RemoveFailed  Identifier
}

type configBackupNode struct {
	Cleanup           configBackupCleanupNode
	DirCreateFailed   Identifier
	DirReadFailed     Identifier
	ExistsCheckFailed Identifier
	LatestReadFailed  Identifier
	ReadFailed        Identifier
	WriteFailed       Identifier
}

type configFileNode struct {
	ExistsCheckFailed Identifier
}

type configManagerNode struct {
	PermanentlyFailed Identifier
}

type configNode struct {
	Agent   configAgentNode
	Backup  configBackupNode
	File    configFileNode
	Manager configManagerNode
}

// Config holds the declared events of the config domain.
var Config = configNode{
	Agent: configAgentNode{
		LocationMapMissing:    Identifier{Tag: "config::agent::location_map_missing", Brief: "config.yaml had no agent location map, so an empty one was substituted.", Severity: SeverityWarning},
		ReleaseChannelInvalid: Identifier{Tag: "config::agent::release_channel_invalid", Brief: "config.yaml named a release channel umh-core does not know, so it was set to n/a.", Severity: SeverityWarning},
	},
	Backup: configBackupNode{
		Cleanup: configBackupCleanupNode{
			DirReadFailed: Identifier{Tag: "config::backup::cleanup::dir_read_failed", Brief: "The config backup directory could not be listed, so old backups were not pruned.", Severity: SeverityWarning},
			RemoveFailed:  Identifier{Tag: "config::backup::cleanup::remove_failed", Brief: "An old config backup could not be deleted, so the backup directory keeps growing.", Severity: SeverityWarning},
		},
		DirCreateFailed:   Identifier{Tag: "config::backup::dir_create_failed", Brief: "The config backup directory could not be created, so this config was not backed up.", Severity: SeverityWarning},
		DirReadFailed:     Identifier{Tag: "config::backup::dir_read_failed", Brief: "The config backup directory could not be listed, so no backup could be chosen.", Severity: SeverityWarning},
		ExistsCheckFailed: Identifier{Tag: "config::backup::exists_check_failed", Brief: "Checking whether a config backup exists failed, so backup was skipped for this write.", Severity: SeverityWarning},
		LatestReadFailed:  Identifier{Tag: "config::backup::latest_read_failed", Brief: "The most recent config backup could not be read, so it cannot be restored from.", Severity: SeverityWarning},
		ReadFailed:        Identifier{Tag: "config::backup::read_failed", Brief: "A config backup could not be read, so this config was not backed up.", Severity: SeverityWarning},
		WriteFailed:       Identifier{Tag: "config::backup::write_failed", Brief: "Writing the config backup failed, so this config revision has no backup.", Severity: SeverityWarning},
	},
	File: configFileNode{
		ExistsCheckFailed: Identifier{Tag: "config::file::exists_check_failed", Brief: "Checking whether config.yaml exists failed, so the config could not be loaded.", Severity: SeverityWarning},
	},
	Manager: configManagerNode{
		PermanentlyFailed: Identifier{Tag: "config::manager::permanently_failed", Brief: "The config manager exhausted its backoff and stopped retrying, so config changes no longer apply.", Severity: SeverityError},
	},
}

type cpuStartupNode struct {
	SnapshotFailed Identifier
}

type cpuNode struct {
	Startup cpuStartupNode
}

// Cpu holds the declared events of the cpu domain.
var Cpu = cpuNode{
	Startup: cpuStartupNode{
		SnapshotFailed: Identifier{Tag: "cpu::startup::snapshot_failed", Brief: "The startup cgroup snapshot failed, so the CPU quota signals are missing for this run.", Severity: SeverityWarning},
	},
}

type cseDeltaNode struct {
	AppendFailed             Identifier
	QueryFallbackToBootstrap Identifier
}

type cseNode struct {
	Delta cseDeltaNode
}

// Cse holds the declared events of the cse domain.
var Cse = cseNode{
	Delta: cseDeltaNode{
		AppendFailed:             Identifier{Tag: "cse::delta::append_failed", Brief: "Appending a delta to the CSE store failed, so this state change is not persisted.", Severity: SeverityWarning},
		QueryFallbackToBootstrap: Identifier{Tag: "cse::delta::query_fallback_to_bootstrap", Brief: "A delta query failed, so the reader fell back to a full bootstrap read.", Severity: SeverityWarning},
	},
}

type examplesPersistenceNode struct {
	ObservedStateLoadFailed Identifier
}

type examplesScenarioNode struct {
	DumpFailed Identifier
}

type examplesStoreNode struct {
	DumpUnsupportedForV2 Identifier
}

type examplesNode struct {
	Persistence examplesPersistenceNode
	Scenario    examplesScenarioNode
	Store       examplesStoreNode
}

// Examples holds the declared events of the examples domain.
var Examples = examplesNode{
	Persistence: examplesPersistenceNode{
		ObservedStateLoadFailed: Identifier{Tag: "examples::persistence::observed_state_load_failed", Brief: "The persistence example could not load its observed state.", Severity: SeverityWarning},
	},
	Scenario: examplesScenarioNode{
		DumpFailed: Identifier{Tag: "examples::scenario::dump_failed", Brief: "Dumping a scenario's state failed, so the run has no artefact to inspect.", Severity: SeverityWarning},
	},
	Store: examplesStoreNode{
		DumpUnsupportedForV2: Identifier{Tag: "examples::store::dump_unsupported_for_v2", Brief: "A store dump was requested for an fsmv2 store, which does not support it.", Severity: SeverityWarning},
	},
}

type supervisorActionNode struct {
	EnqueueFailed     Identifier
	EnqueueRejected   Identifier
	Failed            Identifier
	Panic             Identifier
	QueueFull         Identifier
	StuckDetected     Identifier
	StuckForceRemoved Identifier
}

type supervisorChildNode struct {
	InitialDesiredStateSaveFailed Identifier
	ShutdownRequestFailed         Identifier
	SpecValidationFailed          Identifier
	StartSkippedContextCancelled  Identifier
	SupervisorAddWorkerFailed     Identifier
	SupervisorCreationFailed      Identifier
	TickFailed                    Identifier
	WorkerCreationFailed          Identifier
}

type supervisorCircuitBreakerNode struct {
	Opened         Identifier
	RetryScheduled Identifier
}

type supervisorCollectorNode struct {
	DoublePanic              Identifier
	FinalObservationFailed   Identifier
	FinalObservationSkipped  Identifier
	ObservationFailed        Identifier
	Panic                    Identifier
	RestartFailed            Identifier
	RestartNoParentContext   Identifier
	RestartSkippedNotRunning Identifier
	RestartStartFailed       Identifier
	RestartWorkerNotFound    Identifier
	Restarting               Identifier
	SaveFailed               Identifier
	StartFailed              Identifier
	Stopped                  Identifier
	TriggerNowFailed         Identifier
	TypeMismatch             Identifier
	UnresponsiveMaxAttempts  Identifier
}

type supervisorDesiredStateNode struct {
	DerivedSaveFailed   Identifier
	LoadFailed          Identifier
	LoadFailedOnCollect Identifier
}

type supervisorEscalationNode struct {
	OneRetryRemaining Identifier
	Required          Identifier
}

type supervisorFactoryNode struct {
	InvalidSupervisorType Identifier
}

type supervisorFreshnessNode struct {
	DataStale   Identifier
	DataTimeout Identifier
}

type supervisorObservedStateNode struct {
	InvalidTimestamp     Identifier
	LoadFailed           Identifier
	MissingTimestamp     Identifier
	Timeout              Identifier
	TypeUnknown          Identifier
	UnknownTimestampType Identifier
}

type supervisorPanicCircuitNode struct {
	AutoReset Identifier
	Open      Identifier
}

type supervisorReducerNode struct {
	Error Identifier
}

type supervisorRestartNode struct {
	ClearShutdownFailed   Identifier
	CollectorStartFailed  Identifier
	GracefulTimeout       Identifier
	ShutdownRequestFailed Identifier
}

type supervisorShutdownNode struct {
	BudgetExhausted        Identifier
	ForceExit              Identifier
	GracefulRequestFailed  Identifier
	RequestFailed          Identifier
	RequestFailedInCascade Identifier
	RequestedLoadFailed    Identifier
	Timeout                Identifier
}

type supervisorSignalNode struct {
	ProcessingFailed Identifier
	UnknownReceived  Identifier
}

type supervisorSnapshotNode struct {
	LoadFailed       Identifier
	MissingTimestamp Identifier
}

type supervisorSpecNode struct {
	HashCacheFailed             Identifier
	HashFailed                  Identifier
	OriginalUserMarshalFailed   Identifier
	OriginalUserUnmarshalFailed Identifier
	TemplateRenderingFailed     Identifier
}

type supervisorTickNode struct {
	DoublePanic Identifier
	Error       Identifier
	Panic       Identifier
}

type supervisorWorkerNode struct {
	AddCollectObservedFailed      Identifier
	AddDeriveDesiredFailed        Identifier
	AddDeriveDesiredFallbackToNil Identifier
	AddMarshalDesiredFailed       Identifier
	AddMarshalObservedFailed      Identifier
	AddRejected                   Identifier
	AddSaveDesiredFailed          Identifier
	AddSaveIdentityFailed         Identifier
	AddSaveObservedFailed         Identifier
	AddUnmarshalDesiredFailed     Identifier
	AddUnmarshalObservedFailed    Identifier
	AddZeroTimestampNotSettable   Identifier
	RemovalNotFound               Identifier
	RemoveNotFound                Identifier
	RestartNotFound               Identifier
}

type supervisorNode struct {
	Action         supervisorActionNode
	Child          supervisorChildNode
	CircuitBreaker supervisorCircuitBreakerNode
	Collector      supervisorCollectorNode
	DesiredState   supervisorDesiredStateNode
	Escalation     supervisorEscalationNode
	Factory        supervisorFactoryNode
	Freshness      supervisorFreshnessNode
	ObservedState  supervisorObservedStateNode
	PanicCircuit   supervisorPanicCircuitNode
	Reducer        supervisorReducerNode
	Restart        supervisorRestartNode
	Shutdown       supervisorShutdownNode
	Signal         supervisorSignalNode
	Snapshot       supervisorSnapshotNode
	Spec           supervisorSpecNode
	Tick           supervisorTickNode
	Worker         supervisorWorkerNode
}

// Supervisor holds the declared events of the supervisor domain.
var Supervisor = supervisorNode{
	Action: supervisorActionNode{
		EnqueueFailed:     Identifier{Tag: "supervisor::action::enqueue_failed", Brief: "Enqueueing an action failed, so the state it was driving toward will not be reached.", Severity: SeverityError},
		EnqueueRejected:   Identifier{Tag: "supervisor::action::enqueue_rejected", Brief: "An action was rejected rather than queued, so it will not run.", Severity: SeverityWarning},
		Failed:            Identifier{Tag: "supervisor::action::failed", Brief: "A worker action returned an error, so the state it was driving toward was not reached.", Severity: SeverityError},
		Panic:             Identifier{Tag: "supervisor::action::panic", Brief: "A worker action panicked; it was recovered, so the supervisor keeps ticking but the action did not complete.", Severity: SeverityError},
		QueueFull:         Identifier{Tag: "supervisor::action::queue_full", Brief: "The action queue is full, so new actions are being dropped.", Severity: SeverityError},
		StuckDetected:     Identifier{Tag: "supervisor::action::stuck_detected", Brief: "An action has been running past its deadline and is blocking its worker.", Severity: SeverityWarning},
		StuckForceRemoved: Identifier{Tag: "supervisor::action::stuck_force_removed", Brief: "A stuck action was force-removed so its worker could continue; whatever it was doing is unfinished.", Severity: SeverityError},
	},
	Child: supervisorChildNode{
		InitialDesiredStateSaveFailed: Identifier{Tag: "supervisor::child::initial_desired_state_save_failed", Brief: "A new child's first desired state could not be saved, so it starts with none.", Severity: SeverityWarning},
		ShutdownRequestFailed:         Identifier{Tag: "supervisor::child::shutdown_request_failed", Brief: "Requesting a child's shutdown failed, so it may still be running.", Severity: SeverityWarning},
		SpecValidationFailed:          Identifier{Tag: "supervisor::child::spec_validation_failed", Brief: "A child's spec failed validation, so the child was not created.", Severity: SeverityError},
		StartSkippedContextCancelled:  Identifier{Tag: "supervisor::child::start_skipped_context_cancelled", Brief: "A child's start was skipped because the context was already cancelled, which is normal during shutdown.", Severity: SeverityWarning},
		SupervisorAddWorkerFailed:     Identifier{Tag: "supervisor::child::supervisor_add_worker_failed", Brief: "Adding a worker to a child supervisor failed, so that worker does not exist.", Severity: SeverityError},
		SupervisorCreationFailed:      Identifier{Tag: "supervisor::child::supervisor_creation_failed", Brief: "Creating a child supervisor failed, so its whole subtree is absent.", Severity: SeverityError},
		TickFailed:                    Identifier{Tag: "supervisor::child::tick_failed", Brief: "A child supervisor's tick failed, so its subtree did not reconcile this cycle.", Severity: SeverityError},
		WorkerCreationFailed:          Identifier{Tag: "supervisor::child::worker_creation_failed", Brief: "Creating a child worker failed, so it does not exist.", Severity: SeverityError},
	},
	CircuitBreaker: supervisorCircuitBreakerNode{
		Opened:         Identifier{Tag: "supervisor::circuit_breaker::opened", Brief: "The circuit breaker opened after repeated failures, so this worker stops being reconciled.", Severity: SeverityError},
		RetryScheduled: Identifier{Tag: "supervisor::circuit_breaker::retry_scheduled", Brief: "The circuit breaker scheduled a retry, so reconciliation resumes after the delay.", Severity: SeverityWarning},
	},
	Collector: supervisorCollectorNode{
		DoublePanic:              Identifier{Tag: "supervisor::collector::double_panic", Brief: "A collector panicked while recovering from a panic, so it is dead and this worker stops observing.", Severity: SeverityError},
		FinalObservationFailed:   Identifier{Tag: "supervisor::collector::final_observation_failed", Brief: "The last observation before shutdown failed, so the worker's final state was not recorded.", Severity: SeverityWarning},
		FinalObservationSkipped:  Identifier{Tag: "supervisor::collector::final_observation_skipped", Brief: "The last observation before shutdown was skipped, so the worker's final state was not recorded.", Severity: SeverityWarning},
		ObservationFailed:        Identifier{Tag: "supervisor::collector::observation_failed", Brief: "Collecting observed state failed, so the supervisor is reconciling against stale data.", Severity: SeverityError},
		Panic:                    Identifier{Tag: "supervisor::collector::panic", Brief: "A collector panicked; it was recovered and restarted, so observations resume on the next tick.", Severity: SeverityError},
		RestartFailed:            Identifier{Tag: "supervisor::collector::restart_failed", Brief: "Restarting an unresponsive collector failed, so this worker stops observing until the next attempt.", Severity: SeverityError},
		RestartNoParentContext:   Identifier{Tag: "supervisor::collector::restart_no_parent_context", Brief: "A collector restart found no parent context to derive from, which is a wiring bug.", Severity: SeverityError},
		RestartSkippedNotRunning: Identifier{Tag: "supervisor::collector::restart_skipped_not_running", Brief: "A restart was requested for a collector that is not running, so there was nothing to restart.", Severity: SeverityWarning},
		RestartStartFailed:       Identifier{Tag: "supervisor::collector::restart_start_failed", Brief: "A restarted collector failed to start, so this worker stops observing.", Severity: SeverityError},
		RestartWorkerNotFound:    Identifier{Tag: "supervisor::collector::restart_worker_not_found", Brief: "A collector restart named a worker the supervisor does not hold, which is a wiring bug.", Severity: SeverityError},
		Restarting:               Identifier{Tag: "supervisor::collector::restarting", Brief: "An unresponsive collector is being restarted.", Severity: SeverityWarning},
		SaveFailed:               Identifier{Tag: "supervisor::collector::save_failed", Brief: "Saving a collector's observation failed, so the next tick reconciles against older data.", Severity: SeverityWarning},
		StartFailed:              Identifier{Tag: "supervisor::collector::start_failed", Brief: "A collector failed to start, so its worker is never observed.", Severity: SeverityError},
		Stopped:                  Identifier{Tag: "supervisor::collector::stopped", Brief: "A collector stopped, so its worker is no longer being observed.", Severity: SeverityWarning},
		TriggerNowFailed:         Identifier{Tag: "supervisor::collector::trigger_now_failed", Brief: "An immediate collection could not be triggered, so the next observation waits for the normal interval.", Severity: SeverityWarning},
		TypeMismatch:             Identifier{Tag: "supervisor::collector::type_mismatch", Brief: "A collector produced an observed state of the wrong type, which is a wiring bug.", Severity: SeverityError},
		UnresponsiveMaxAttempts:  Identifier{Tag: "supervisor::collector::unresponsive_max_attempts", Brief: "A collector stayed unresponsive through every restart attempt, so this worker stops being observed.", Severity: SeverityError},
	},
	DesiredState: supervisorDesiredStateNode{
		DerivedSaveFailed:   Identifier{Tag: "supervisor::desired_state::derived_save_failed", Brief: "Saving the derived desired state failed, so the next tick derives it again from older data.", Severity: SeverityWarning},
		LoadFailed:          Identifier{Tag: "supervisor::desired_state::load_failed", Brief: "Loading a worker's desired state failed, so the reconcile could not run.", Severity: SeverityError},
		LoadFailedOnCollect: Identifier{Tag: "supervisor::desired_state::load_failed_on_collect", Brief: "Loading desired state failed during collection, so this observation was skipped.", Severity: SeverityWarning},
	},
	Escalation: supervisorEscalationNode{
		OneRetryRemaining: Identifier{Tag: "supervisor::escalation::one_retry_remaining", Brief: "A worker has one retry left before it escalates.", Severity: SeverityWarning},
		Required:          Identifier{Tag: "supervisor::escalation::required", Brief: "A worker exhausted its retries and needs a human, so it stays in its failed state.", Severity: SeverityError},
	},
	Factory: supervisorFactoryNode{
		InvalidSupervisorType: Identifier{Tag: "supervisor::factory::invalid_supervisor_type", Brief: "A supervisor was requested for a type the factory does not know, so it was not created.", Severity: SeverityError},
	},
	Freshness: supervisorFreshnessNode{
		DataStale:   Identifier{Tag: "supervisor::freshness::data_stale", Brief: "A worker's observed state is older than the stale threshold, so it is being reconciled against old data.", Severity: SeverityWarning},
		DataTimeout: Identifier{Tag: "supervisor::freshness::data_timeout", Brief: "A worker's observed state is older than the collector timeout, so it is being reconciled against old data.", Severity: SeverityWarning},
	},
	ObservedState: supervisorObservedStateNode{
		InvalidTimestamp:     Identifier{Tag: "supervisor::observed_state::invalid_timestamp", Brief: "An observed state carried an unusable timestamp, so its freshness cannot be judged.", Severity: SeverityWarning},
		LoadFailed:           Identifier{Tag: "supervisor::observed_state::load_failed", Brief: "Loading a worker's observed state failed, so the reconcile could not run.", Severity: SeverityError},
		MissingTimestamp:     Identifier{Tag: "supervisor::observed_state::missing_timestamp", Brief: "An observed state carried no timestamp, so its freshness cannot be judged.", Severity: SeverityWarning},
		Timeout:              Identifier{Tag: "supervisor::observed_state::timeout", Brief: "No observed state arrived within the timeout, so the supervisor is reconciling against stale data.", Severity: SeverityWarning},
		TypeUnknown:          Identifier{Tag: "supervisor::observed_state::type_unknown", Brief: "An observed state's type is not one the supervisor knows, so it cannot be read.", Severity: SeverityWarning},
		UnknownTimestampType: Identifier{Tag: "supervisor::observed_state::unknown_timestamp_type", Brief: "An observed state's timestamp field is of an unexpected type, so its freshness cannot be judged.", Severity: SeverityWarning},
	},
	PanicCircuit: supervisorPanicCircuitNode{
		AutoReset: Identifier{Tag: "supervisor::panic_circuit::auto_reset", Brief: "The panic circuit reset itself after a quiet period, so ticking resumes.", Severity: SeverityWarning},
		Open:      Identifier{Tag: "supervisor::panic_circuit::open", Brief: "The panic circuit opened after repeated tick panics, so this subtree stops ticking.", Severity: SeverityWarning},
	},
	Reducer: supervisorReducerNode{
		Error: Identifier{Tag: "supervisor::reducer::error", Brief: "A state reducer returned an error, so the worker keeps its previous state.", Severity: SeverityWarning},
	},
	Restart: supervisorRestartNode{
		ClearShutdownFailed:   Identifier{Tag: "supervisor::restart::clear_shutdown_failed", Brief: "Clearing the shutdown flag during a restart failed, so the worker may refuse to start.", Severity: SeverityWarning},
		CollectorStartFailed:  Identifier{Tag: "supervisor::restart::collector_start_failed", Brief: "A restarted worker's collector failed to start, so it is not being observed.", Severity: SeverityError},
		GracefulTimeout:       Identifier{Tag: "supervisor::restart::graceful_timeout", Brief: "A worker did not stop within its restart timeout, so the restart continued without a clean stop.", Severity: SeverityWarning},
		ShutdownRequestFailed: Identifier{Tag: "supervisor::restart::shutdown_request_failed", Brief: "The stop half of a restart failed, so the worker may be running two instances.", Severity: SeverityWarning},
	},
	Shutdown: supervisorShutdownNode{
		BudgetExhausted:        Identifier{Tag: "supervisor::shutdown::budget_exhausted", Brief: "The graceful-shutdown budget was spent by child drains, so remaining workers were not drained.", Severity: SeverityWarning},
		ForceExit:              Identifier{Tag: "supervisor::shutdown::force_exit", Brief: "Graceful shutdown did not finish in time, so umh-core exited anyway; in-flight work is lost.", Severity: SeverityWarning},
		GracefulRequestFailed:  Identifier{Tag: "supervisor::shutdown::graceful_request_failed", Brief: "A graceful shutdown request failed, so that worker may not have drained.", Severity: SeverityWarning},
		RequestFailed:          Identifier{Tag: "supervisor::shutdown::request_failed", Brief: "Requesting a worker's shutdown failed, so the reconcile stopped.", Severity: SeverityError},
		RequestFailedInCascade: Identifier{Tag: "supervisor::shutdown::request_failed_in_cascade", Brief: "One worker's shutdown request failed while shutting every worker down; the cascade continued without it.", Severity: SeverityWarning},
		RequestedLoadFailed:    Identifier{Tag: "supervisor::shutdown::requested_load_failed", Brief: "Loading the shutdown-requested flag failed, so the supervisor cannot tell whether this worker is stopping.", Severity: SeverityWarning},
		Timeout:                Identifier{Tag: "supervisor::shutdown::timeout", Brief: "Graceful shutdown timed out with workers still running, so they were not drained.", Severity: SeverityWarning},
	},
	Signal: supervisorSignalNode{
		ProcessingFailed: Identifier{Tag: "supervisor::signal::processing_failed", Brief: "Processing an OS signal failed, so shutdown may not have been initiated.", Severity: SeverityError},
		UnknownReceived:  Identifier{Tag: "supervisor::signal::unknown_received", Brief: "An OS signal arrived that umh-core has no handler for, so it was ignored.", Severity: SeverityError},
	},
	Snapshot: supervisorSnapshotNode{
		LoadFailed:       Identifier{Tag: "supervisor::snapshot::load_failed", Brief: "Loading a worker's snapshot failed, so it reconciles from nothing.", Severity: SeverityError},
		MissingTimestamp: Identifier{Tag: "supervisor::snapshot::missing_timestamp", Brief: "A snapshot carried no timestamp, so its freshness cannot be judged.", Severity: SeverityWarning},
	},
	Spec: supervisorSpecNode{
		HashCacheFailed:             Identifier{Tag: "supervisor::spec::hash_cache_failed", Brief: "Caching a spec hash failed, so the next tick rehashes it.", Severity: SeverityWarning},
		HashFailed:                  Identifier{Tag: "supervisor::spec::hash_failed", Brief: "Hashing a spec failed, so the supervisor cannot tell whether it changed.", Severity: SeverityWarning},
		OriginalUserMarshalFailed:   Identifier{Tag: "supervisor::spec::original_user_marshal_failed", Brief: "Marshalling the user's original spec failed, so it is not stored for later comparison.", Severity: SeverityWarning},
		OriginalUserUnmarshalFailed: Identifier{Tag: "supervisor::spec::original_user_unmarshal_failed", Brief: "Reading back the user's original spec failed, so edits cannot be compared against it.", Severity: SeverityWarning},
		TemplateRenderingFailed:     Identifier{Tag: "supervisor::spec::template_rendering_failed", Brief: "Rendering a spec template failed, so the worker's config could not be produced.", Severity: SeverityError},
	},
	Tick: supervisorTickNode{
		DoublePanic: Identifier{Tag: "supervisor::tick::double_panic", Brief: "A tick panicked while recovering from a panic, so this supervisor stops ticking.", Severity: SeverityError},
		Error:       Identifier{Tag: "supervisor::tick::error", Brief: "A tick returned an error, so this cycle did not reconcile.", Severity: SeverityError},
		Panic:       Identifier{Tag: "supervisor::tick::panic", Brief: "A tick panicked; it was recovered, so ticking continues from the next cycle.", Severity: SeverityError},
	},
	Worker: supervisorWorkerNode{
		AddCollectObservedFailed:      Identifier{Tag: "supervisor::worker::add_collect_observed_failed", Brief: "A new worker's first observation failed, so it was not added.", Severity: SeverityError},
		AddDeriveDesiredFailed:        Identifier{Tag: "supervisor::worker::add_derive_desired_failed", Brief: "Deriving a new worker's desired state failed, so it was not added.", Severity: SeverityError},
		AddDeriveDesiredFallbackToNil: Identifier{Tag: "supervisor::worker::add_derive_desired_fallback_to_nil", Brief: "A new worker's desired state derived to nothing, so it starts with none.", Severity: SeverityWarning},
		AddMarshalDesiredFailed:       Identifier{Tag: "supervisor::worker::add_marshal_desired_failed", Brief: "Marshalling a new worker's desired state failed, so it was not added.", Severity: SeverityError},
		AddMarshalObservedFailed:      Identifier{Tag: "supervisor::worker::add_marshal_observed_failed", Brief: "Marshalling a new worker's observed state failed, so it was not added.", Severity: SeverityError},
		AddRejected:                   Identifier{Tag: "supervisor::worker::add_rejected", Brief: "A worker was rejected rather than added, so it does not exist.", Severity: SeverityWarning},
		AddSaveDesiredFailed:          Identifier{Tag: "supervisor::worker::add_save_desired_failed", Brief: "Saving a new worker's desired state failed, so it was not added.", Severity: SeverityError},
		AddSaveIdentityFailed:         Identifier{Tag: "supervisor::worker::add_save_identity_failed", Brief: "Saving a new worker's identity failed, so it was not added.", Severity: SeverityError},
		AddSaveObservedFailed:         Identifier{Tag: "supervisor::worker::add_save_observed_failed", Brief: "Saving a new worker's observed state failed, so it was not added.", Severity: SeverityError},
		AddUnmarshalDesiredFailed:     Identifier{Tag: "supervisor::worker::add_unmarshal_desired_failed", Brief: "Reading back a new worker's desired state failed, so it was not added.", Severity: SeverityError},
		AddUnmarshalObservedFailed:    Identifier{Tag: "supervisor::worker::add_unmarshal_observed_failed", Brief: "Reading back a new worker's observed state failed, so it was not added.", Severity: SeverityError},
		AddZeroTimestampNotSettable:   Identifier{Tag: "supervisor::worker::add_zero_timestamp_not_settable", Brief: "A new worker's observed state has a zero timestamp that cannot be set, so its freshness is unjudgeable.", Severity: SeverityWarning},
		RemovalNotFound:               Identifier{Tag: "supervisor::worker::removal_not_found", Brief: "A removal named a worker the supervisor does not hold, so nothing was removed.", Severity: SeverityWarning},
		RemoveNotFound:                Identifier{Tag: "supervisor::worker::remove_not_found", Brief: "A remove call named a worker the supervisor does not hold, so nothing was removed.", Severity: SeverityWarning},
		RestartNotFound:               Identifier{Tag: "supervisor::worker::restart_not_found", Brief: "A restart named a worker the supervisor does not hold, so nothing was restarted.", Severity: SeverityError},
	},
}

type telemetryNode struct {
	UnregisteredIdentifier Identifier
}

// Telemetry holds the declared events of the telemetry domain.
var Telemetry = telemetryNode{
	UnregisteredIdentifier: Identifier{Tag: "telemetry::unregistered_identifier", Brief: "A zero-value telemetry.Identifier reached the logger, so the call site is a bug.", Severity: SeverityError},
}

type workersAuthNode struct {
	Failed              Identifier
	InstanceUuidMissing Identifier
	PersistentFailure   Identifier
}

type workersConfigWatchNode struct {
	CpuUpsertFailed       Identifier
	HistorianUpsertFailed Identifier
	ReadFailed            Identifier
}

type workersConfigNode struct {
	Watch workersConfigWatchNode
}

type workersConnectNode struct {
	CancelledDuringDelay Identifier
	FailedSimulated      Identifier
	InitialFailed        Identifier
}

type workersPullNode struct {
	DependenciesCreationFailed       Identifier
	ParentTransportDepsMissing       Identifier
	PersistentFailure                Identifier
	SkippedNilInboundChan            Identifier
	SkippedNilInboundChanWithPending Identifier
}

type workersPushNode struct {
	DependenciesCreationFailed Identifier
	ParentTransportDepsMissing Identifier
	PersistentFailure          Identifier
}

type workersTransportNode struct {
	BackpressureEntering          Identifier
	BackpressureExiting           Identifier
	ChannelProviderNotInitialized Identifier
	PendingBufferOverflow         Identifier
	PoisonMessageDropped          Identifier
	PreviousObservedLoadFailed    Identifier
	SimulatingPanic               Identifier
}

type workersNode struct {
	Auth      workersAuthNode
	Config    workersConfigNode
	Connect   workersConnectNode
	Pull      workersPullNode
	Push      workersPushNode
	Transport workersTransportNode
}

// Workers holds the declared events of the workers domain.
var Workers = workersNode{
	Auth: workersAuthNode{
		Failed:              Identifier{Tag: "workers::auth::failed", Brief: "Authenticating with the Management Console failed, so this instance stays unauthenticated.", Severity: SeverityWarning},
		InstanceUuidMissing: Identifier{Tag: "workers::auth::instance_uuid_missing", Brief: "The auth response carried no instance UUID, so the instance cannot identify itself.", Severity: SeverityWarning},
		PersistentFailure:   Identifier{Tag: "workers::auth::persistent_failure", Brief: "Authentication has failed repeatedly, so the instance may appear offline.", Severity: SeverityWarning},
	},
	Config: workersConfigNode{
		Watch: workersConfigWatchNode{
			CpuUpsertFailed:       Identifier{Tag: "workers::config::watch::cpu_upsert_failed", Brief: "Upserting the CPU monitor child failed, so CPU health is not being collected.", Severity: SeverityWarning},
			HistorianUpsertFailed: Identifier{Tag: "workers::config::watch::historian_upsert_failed", Brief: "Upserting the historian child failed, so historian data is not being collected.", Severity: SeverityWarning},
			ReadFailed:            Identifier{Tag: "workers::config::watch::read_failed", Brief: "The config worker could not read config.yaml on this tick, so children were not reconciled.", Severity: SeverityWarning},
		},
	},
	Connect: workersConnectNode{
		CancelledDuringDelay: Identifier{Tag: "workers::connect::cancelled_during_delay", Brief: "A reconnect was cancelled while waiting out its backoff delay, which is normal during shutdown.", Severity: SeverityWarning},
		FailedSimulated:      Identifier{Tag: "workers::connect::failed_simulated", Brief: "A simulated connection failure fired, which only happens in a test scenario.", Severity: SeverityWarning},
		InitialFailed:        Identifier{Tag: "workers::connect::initial_failed", Brief: "The first connection to the Management Console failed, so the instance starts offline.", Severity: SeverityWarning},
	},
	Pull: workersPullNode{
		DependenciesCreationFailed:       Identifier{Tag: "workers::pull::dependencies_creation_failed", Brief: "The pull worker's dependencies could not be created, so it cannot receive actions.", Severity: SeverityError},
		ParentTransportDepsMissing:       Identifier{Tag: "workers::pull::parent_transport_deps_missing", Brief: "The pull worker found no transport dependencies published by its parent, so it cannot start.", Severity: SeverityError},
		PersistentFailure:                Identifier{Tag: "workers::pull::persistent_failure", Brief: "Pulling from the Management Console has failed repeatedly, so actions are not arriving.", Severity: SeverityWarning},
		SkippedNilInboundChan:            Identifier{Tag: "workers::pull::skipped_nil_inbound_chan", Brief: "A pull was skipped because the inbound channel was nil, so the message was not delivered.", Severity: SeverityWarning},
		SkippedNilInboundChanWithPending: Identifier{Tag: "workers::pull::skipped_nil_inbound_chan_with_pending", Brief: "A pull was skipped on a nil inbound channel while deliveries were pending, so those are dropped.", Severity: SeverityWarning},
	},
	Push: workersPushNode{
		DependenciesCreationFailed: Identifier{Tag: "workers::push::dependencies_creation_failed", Brief: "The push worker's dependencies could not be created, so it cannot send status.", Severity: SeverityError},
		ParentTransportDepsMissing: Identifier{Tag: "workers::push::parent_transport_deps_missing", Brief: "The push worker found no transport dependencies published by its parent, so it cannot start.", Severity: SeverityError},
		PersistentFailure:          Identifier{Tag: "workers::push::persistent_failure", Brief: "Pushing to the Management Console has failed repeatedly, so the instance may appear offline.", Severity: SeverityWarning},
	},
	Transport: workersTransportNode{
		BackpressureEntering:          Identifier{Tag: "workers::transport::backpressure_entering", Brief: "The outbound queue filled, so the transport started shedding load.", Severity: SeverityWarning},
		BackpressureExiting:           Identifier{Tag: "workers::transport::backpressure_exiting", Brief: "The outbound queue drained, so the transport stopped shedding load.", Severity: SeverityWarning},
		ChannelProviderNotInitialized: Identifier{Tag: "workers::transport::channel_provider_not_initialized", Brief: "The channel provider was not initialised, so no messages can move in either direction.", Severity: SeverityWarning},
		PendingBufferOverflow:         Identifier{Tag: "workers::transport::pending_buffer_overflow", Brief: "The pending message buffer overflowed, so the oldest messages were dropped.", Severity: SeverityWarning},
		PoisonMessageDropped:          Identifier{Tag: "workers::transport::poison_message_dropped", Brief: "A message failed repeatedly and was dropped to stop it blocking the queue.", Severity: SeverityWarning},
		PreviousObservedLoadFailed:    Identifier{Tag: "workers::transport::previous_observed_load_failed", Brief: "The previous observed state could not be loaded, so this tick starts from nothing.", Severity: SeverityWarning},
		SimulatingPanic:               Identifier{Tag: "workers::transport::simulating_panic", Brief: "A deliberate panic was raised, which only happens in a test scenario.", Severity: SeverityWarning},
	},
}
