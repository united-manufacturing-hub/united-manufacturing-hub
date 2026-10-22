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
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// Config errors
//
// A config error (*config.ConfigError) is a mistake in the user's config,
// such as YAML that does not parse or an unknown desired state value. It is
// the user's mistake, not a fault in the code. The supervisor handles it as
// follows:
//
//   - A worker or child whose config has an error does not start. A running
//     worker does not take the broken config. It keeps ticking on the desired
//     state derived from its last good config, so shutdown, removal and its
//     children keep working. A shutdown or disable requested meanwhile is
//     read from the stored desired state (loadStoredLifecycleFlags).
//   - The error is logged once at Info as "config_error" and is never sent to
//     Sentry. The same message is logged again only after a recovery: a
//     successful derive, or a successful child create.
//   - Later ticks retry. The derive runs again on every tick, a failed child
//     create is retried on every reconcile, and a worker re-add a restart
//     left pending is retried on every tick (retryPendingReAdds). The first
//     attempt that succeeds applies the new config.
//
// A worker whose first derive fails has no last good config, so it is not
// ticked. When its supervisor shuts down, its children are dropped rather
// than recreated from cached specs.
//
// Sentry is for faults in the code that engineering can fix. A config error
// is fixed by the user, and the config_error log line tells them what to
// change.

// loadStoredLifecycleFlags returns a document holding only the lifecycle
// flags (ShutdownRequested, Disabled) of the desired state stored for
// workerID, for a tick that runs on the last good config. It returns nil when
// nothing is stored or the load fails. It never writes to the store.
func (s *Supervisor[TObserved, TDesired]) loadStoredLifecycleFlags(ctx context.Context, workerID string) persistence.Document {
	// Load under lifecycleFlagMu, the lock the flag setters and
	// saveDerivedDesiredState hold around their load and save, so a flag a
	// setter has committed is the flag read here.
	s.lifecycleFlagMu.Lock()

	var existingDesiredTyped TDesired

	loadErr := s.store.LoadDesiredTyped(ctx, s.workerType, workerID, &existingDesiredTyped)
	s.lifecycleFlagMu.Unlock()

	if loadErr != nil {
		if !errors.Is(loadErr, persistence.ErrNotFound) {
			// The caller then reads no flags, the same as for a worker that
			// never derived.
			s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "stored_desired_state_load_failed",
				deps.Err(loadErr),
				deps.String("worker_id", workerID))
		}

		return nil
	}

	ds, ok := any(existingDesiredTyped).(fsmv2.DesiredState)
	if !ok {
		return nil
	}

	desiredDoc := make(persistence.Document)

	if ds.IsShutdownRequested() {
		desiredDoc[FieldShutdownRequested] = true
	}

	if ds.IsDisabled() {
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
