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

// Package deps provides dependency injection for fsmv2 workers.
//
// # Logging
//
// [FSMLogger] provides structured logging with compile-time enforcement of
// Sentry fields. Use it for all logging in fsmv2 components.
//
// ## Choosing a Log Method
//
// Use [FSMLogger.Info] and [FSMLogger.Debug] for operational logs:
//
//	logger.Info("worker_started", WorkerID(id), WorkerType(wt))
//	logger.Debug("tick_completed", DurationMs(elapsed))
//
// Use [FSMLogger.SentryWarn] for warnings that should appear in Sentry:
//
//	logger.SentryWarn(FeatureReconciliation, "collector_unresponsive",
//	    Attempts(attempts),
//	    HierarchyPath(path))
//
// Use [FSMLogger.SentryError] for errors that should appear in Sentry:
//
//	logger.SentryError(FeatureLifecycle, err, "worker_add_failed",
//	    HierarchyPath(identity.HierarchyPath))
//
// ## Feature Enum
//
// The [Feature] type identifies which subsystem owns an error. Sentry uses
// this field to route alerts to the correct team. Define new features in
// feature.go when adding new subsystems.
//
// ## Field Constructors
//
// Use typed field constructors to prevent key typos:
//
//	HierarchyPath(path)  // Correct: uses "hierarchy_path"
//	String("hirarchy_path", path)  // Wrong: typo goes unnoticed
//
// Available constructors: [String], [Int], [Int64], [Bool], [Duration], [Any],
// [HierarchyPath], [WorkerID], [WorkerType], [ActionName], [CorrelationID],
// [DurationMs], [Attempts].
//
// ## Adding Context
//
// Use [FSMLogger.With] to add fields to all subsequent logs:
//
//	scopedLogger := logger.With(WorkerID(id), WorkerType(wt))
//	scopedLogger.Info("starting")  // Includes WorkerID and WorkerType
//	scopedLogger.Info("running")   // Also includes them
//
// ## Testing
//
// Use [NewNopFSMLogger] in unit tests:
//
//	logger := NewNopFSMLogger()
//	worker := NewWorker(logger, ...)
//
// # Dependencies Interface
//
// Workers access tools through the [Dependencies] interface. All workers
// receive a logger via [Dependencies.GetLogger]. Actions use
// [Dependencies.ActionLogger] for action-scoped logging.
//
// # Types
//
//   - [BaseDependencies]: Common tools for all workers (logger, state reader, metrics)
//   - [StateReader]: Read-only access to TriangularStore
//   - [MetricsRecorder]: Buffer for action-written metrics
//   - [FSMLogger]: Structured logging with Sentry enforcement
//   - [Feature]: Subsystem identifier for Sentry routing
//
// Workers embed [BaseDependencies] and extend with worker-specific tools.
// See pkg/fsmv2/DEPENDENCIES.md for patterns and examples.
package deps
