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
	"time"

	"github.com/looplab/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/storage"
)

// registerCallbacks registers common callbacks for state transitions
// These callbacks are executed synchronously and should not have any network calls or other operations that could fail
func (instance *DataflowComponentInstance) registerCallbacks() {
	// Basic operational state callbacks
	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStarting, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering starting state for %s", instance.baseFSMInstance.GetID())
		instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
			Record: storage.Record{
				State:       OperationalStateStarting,
				SourceEvent: e.Event,
			},
			Time: time.Now(),
		})
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStartingFailed, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Errorf("Entering starting-failed state for %s", instance.baseFSMInstance.GetID())
		instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
			Record: storage.Record{
				State:       OperationalStateStartingFailed,
				SourceEvent: e.Event,
			},
			Time: time.Now(),
		})
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStopping, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering stopping state for %s", instance.baseFSMInstance.GetID())
		instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
			Record: storage.Record{
				State:       OperationalStateStopping,
				SourceEvent: e.Event,
			},
			Time: time.Now(),
		})
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStopped, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering stopped state for %s", instance.baseFSMInstance.GetID())
		instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
			Record: storage.Record{
				State:       OperationalStateStopped,
				SourceEvent: e.Event,
			},
			Time: time.Now(),
		})
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateActive, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering active state for %s", instance.baseFSMInstance.GetID())
		instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
			Record: storage.Record{
				State:       OperationalStateActive,
				SourceEvent: e.Event,
			},
			Time: time.Now(),
		})
	})

	// Running phase state callbacks
	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateIdle, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering idle state for %s", instance.baseFSMInstance.GetID())
		instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
			Record: storage.Record{
				State:       OperationalStateIdle,
				SourceEvent: e.Event,
			},
			Time: time.Now(),
		})
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateDegraded, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Warnf("Entering degraded state for %s", instance.baseFSMInstance.GetID())
		instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
			Record: storage.Record{
				State:       OperationalStateDegraded,
				SourceEvent: e.Event,
			},
			Time: time.Now(),
		})
	})
}
