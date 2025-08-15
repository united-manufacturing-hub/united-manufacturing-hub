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

package streamprocessor

import (
	"context"

	"github.com/looplab/fsm"
)

// registerCallbacks registers common callbacks for state transitions
// These callbacks are executed synchronously and should not have any network calls or other operations that could fail.
func (i *Instance) registerCallbacks() {
	i.baseFSMInstance.AddCallback("enter_"+OperationalStateStopping, func(ctx context.Context, e *fsm.Event) {
		i.baseFSMInstance.GetLogger().Infof("Entering stopping state for %s", i.baseFSMInstance.GetID())
		// instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
		//	Record: storage.Record{
		//		State:       OperationalStateStopping,
		//		SourceEvent: e.Event,
		//	},
		//	Time: time.Now(),
		// })
	})

	i.baseFSMInstance.AddCallback("enter_"+OperationalStateStopped, func(ctx context.Context, e *fsm.Event) {
		i.baseFSMInstance.GetLogger().Infof("Entering stopped state for %s", i.baseFSMInstance.GetID())
		// instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
		//	Record: storage.Record{
		//		State:       OperationalStateStopped,
		//		SourceEvent: e.Event,
		//	},
		//	Time: time.Now(),
		// })
	})

	i.baseFSMInstance.AddCallback("enter_"+OperationalStateStartingRedpanda, func(ctx context.Context, e *fsm.Event) {
		i.baseFSMInstance.GetLogger().Infof("Entering starting redpanda state for %s", i.baseFSMInstance.GetID())
		// instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
		//	Record: storage.Record{
		//		State:       OperationalStateStartingRedpanda,
		//		SourceEvent: e.Event,
		//	},
		//	Time: time.Now(),
		// })
	})

	i.baseFSMInstance.AddCallback("enter_"+OperationalStateStartingDFC, func(ctx context.Context, e *fsm.Event) {
		i.baseFSMInstance.GetLogger().Infof("Entering starting dfc state for %s", i.baseFSMInstance.GetID())
		// instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
		//	Record: storage.Record{
		//		State:       OperationalStateStartingDFC,
		//		SourceEvent: e.Event,
		//	},
		//	Time: time.Now(),
		// })
	})

	i.baseFSMInstance.AddCallback("enter_"+OperationalStateStartingFailedDFC, func(ctx context.Context, e *fsm.Event) {
		i.baseFSMInstance.GetLogger().Errorf("Entering starting-failed-dfc state for %s", i.baseFSMInstance.GetID())
		// instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
		//	Record: storage.Record{
		//		State:       OperationalStateStartingFailedDFC,
		//		SourceEvent: e.Event,
		//	},
		//	Time: time.Now(),
		// })
	})

	// Running phase state callbacks
	i.baseFSMInstance.AddCallback("enter_"+OperationalStateIdle, func(ctx context.Context, e *fsm.Event) {
		i.baseFSMInstance.GetLogger().Infof("Entering idle state for %s", i.baseFSMInstance.GetID())
		// instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
		//	Record: storage.Record{
		//		State:       OperationalStateIdle,
		//		SourceEvent: e.Event,
		//	},
		//	Time: time.Now(),
		// })
	})

	i.baseFSMInstance.AddCallback("enter_"+OperationalStateActive, func(ctx context.Context, e *fsm.Event) {
		i.baseFSMInstance.GetLogger().Infof("Entering active state for %s", i.baseFSMInstance.GetID())
		// instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
		//	Record: storage.Record{
		//		State:       OperationalStateActive,
		//		SourceEvent: e.Event,
		//	},
		//	Time: time.Now(),
		// })
	})

	i.baseFSMInstance.AddCallback("enter_"+OperationalStateDegradedRedpanda, func(ctx context.Context, e *fsm.Event) {
		i.baseFSMInstance.GetLogger().Warnf("Entering degraded redpanda state for %s", i.baseFSMInstance.GetID())
		// instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
		//	Record: storage.Record{
		//		State:       OperationalStateDegradedRedpanda,
		//		SourceEvent: e.Event,
		//	},
		//	Time: time.Now(),
		// })
	})

	i.baseFSMInstance.AddCallback("enter_"+OperationalStateDegradedDFC, func(ctx context.Context, e *fsm.Event) {
		i.baseFSMInstance.GetLogger().Warnf("Entering degraded dfc state for %s", i.baseFSMInstance.GetID())
		// instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
		//	Record: storage.Record{
		//		State:       OperationalStateDegradedDFC,
		//		SourceEvent: e.Event,
		//	},
		//	Time: time.Now(),
		// })
	})

	i.baseFSMInstance.AddCallback("enter_"+OperationalStateDegradedOther, func(ctx context.Context, e *fsm.Event) {
		i.baseFSMInstance.GetLogger().Warnf("Entering degraded other state for %s", i.baseFSMInstance.GetID())
		// instance.archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
		//	Record: storage.Record{
		//		State:       OperationalStateDegradedOther,
		//		SourceEvent: e.Event,
		//	},
		//	Time: time.Now(),
		// })
	})
}
