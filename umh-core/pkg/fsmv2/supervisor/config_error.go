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

package supervisor

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// This file holds the supervisor's config-error helpers. The "Config errors"
// section of the package doc explains how the supervisor handles a config
// error.

// loadStoredLifecycleFlags returns a document holding only the lifecycle
// flags (ShutdownRequested, Disabled) of the desired state stored for
// workerID, for a tick that runs on the last good config. It returns nil when
// the load fails. It never writes to the store.
func (s *Supervisor[TObserved, TDesired]) loadStoredLifecycleFlags(ctx context.Context, workerID string) persistence.Document {
	// Load under lifecycleFlagMu, the lock the flag setters and
	// saveDerivedDesiredState hold around their load and save, so a flag a
	// setter has committed is the flag read here.
	s.lifecycleFlagMu.Lock()

	shutdown, disabled, flagErr := s.storedLifecycleFlags(ctx, workerID)

	s.lifecycleFlagMu.Unlock()

	if flagErr != nil {
		// The caller then reads no flags, the same as for a worker that
		// never derived.
		s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "stored_desired_state_load_failed",
			deps.Err(flagErr),
			deps.String("worker_id", workerID))

		return nil
	}

	desiredDoc := make(persistence.Document)

	if shutdown {
		desiredDoc[FieldShutdownRequested] = true
	}

	if disabled {
		desiredDoc[FieldDisabled] = true
	}

	return desiredDoc
}

// logConfigErrorOnce logs "config_error" at Info for this supervisor's own
// worker, unless the message equals the last one logged. A successful derive
// clears the remembered message, so the same error is logged again after a
// recovery.
func (s *Supervisor[TObserved, TDesired]) logConfigErrorOnce(cfgErr *config.ConfigError) {
	message := cfgErr.Error()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lastConfigErrorLogged == message {
		return
	}

	s.lastConfigErrorLogged = message
	s.logger.Info("config_error",
		deps.HierarchyPath(s.GetHierarchyPathUnlocked()),
		deps.String("field", cfgErr.Field),
		deps.Err(cfgErr.Err),
		deps.String("message", message))
}

// logChildConfigErrorOnce logs "config_error" at Info for a child the parent
// could not create, once per child name and message. The parent keeps the
// remembered message, because a child that failed to create does not exist
// to keep it. Creating the child clears the entry. The caller must hold s.mu,
// as reconcileChildren does.
func (s *Supervisor[TObserved, TDesired]) logChildConfigErrorOnce(childName string, cfgErr *config.ConfigError) {
	message := cfgErr.Error()

	if s.childConfigErrorLogged == nil {
		s.childConfigErrorLogged = make(map[string]string)
	}

	if last, logged := s.childConfigErrorLogged[childName]; logged && last == message {
		return
	}

	s.childConfigErrorLogged[childName] = message
	s.logger.Info("config_error",
		deps.HierarchyPath(s.GetHierarchyPathUnlocked()),
		deps.String("child_name", childName),
		deps.String("field", cfgErr.Field),
		deps.Err(cfgErr.Err),
		deps.String("message", message))
}

// markChildCreateConfigError records that the child's latest create attempt
// failed with a config error, which holds the child back. The caller must
// hold s.mu, as reconcileChildren does.
func (s *Supervisor[TObserved, TDesired]) markChildCreateConfigError(childName string) {
	if s.childCreateConfigError == nil {
		s.childCreateConfigError = make(map[string]struct{})
	}

	s.childCreateConfigError[childName] = struct{}{}
}

// clearChildCreateConfigError ends the config-error hold on the child, so a
// later absence warns again. The caller must hold s.mu, as reconcileChildren
// does.
func (s *Supervisor[TObserved, TDesired]) clearChildCreateConfigError(childName string) {
	delete(s.childCreateConfigError, childName)
}

// childHeldBackByConfigError reports whether the child's latest create
// attempt failed with a config error. The caller must hold s.mu, as
// reconcileChildren does.
func (s *Supervisor[TObserved, TDesired]) childHeldBackByConfigError(childName string) bool {
	_, held := s.childCreateConfigError[childName]

	return held
}
