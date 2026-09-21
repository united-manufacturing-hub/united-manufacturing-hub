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

// This file exposes unexported seams of the actions package to the external
// actions_test package. It is compiled into test binaries only.

package actions

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// SetTickInterval overrides the awaitRollout poll interval so specs do not
// wait on the 1s production ticker.
func (a *EditProtocolConverterAction) SetTickInterval(d time.Duration) {
	a.tickInterval = d
}

// SetAwaitTimeout overrides the awaitRollout overall timeout so timeout-path
// specs do not wait the full 30s.
func (a *EditProtocolConverterAction) SetAwaitTimeout(d time.Duration) {
	a.awaitTimeout = d
}

// SetFSMLogger replaces the action's FSMLogger so specs can record which
// Sentry events fire and what fields they carry.
func (a *EditProtocolConverterAction) SetFSMLogger(l deps.FSMLogger) {
	a.fsmLogger = l
}

// CompareProtocolConverterDFCConfig exposes compareProtocolConverterDFCConfig
// so specs can pin the lastRenderErr lifecycle without wall-clock ticks.
func (a *EditProtocolConverterAction) CompareProtocolConverterDFCConfig(pcSnapshot *protocolconverter.ProtocolConverterObservedStateSnapshot) (bool, error) {
	return a.compareProtocolConverterDFCConfig(pcSnapshot)
}

// LastRenderErr exposes the sticky render error captured for the timeout
// message.
func (a *EditProtocolConverterAction) LastRenderErr() error {
	return a.lastRenderErr
}
